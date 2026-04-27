package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/parcel/agent/internal/plan"
	"github.com/parcel/agent/internal/planexec"
)

type Desired struct {
	ArtifactID      string
	SoftwareVersion string
	ConfigRev       string
	DownloadURL     string
}

type ArtifactMeta struct {
	ArtifactID          string
	SHA256              string
	SizeBytes           int64
	Version             string
	Type                string
	Signature           string
	SignatureType       string
	SignatureKeyID      string
	VerificationStatus  string
	VerificationError   string
}

type Manifest struct {
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Type      string         `json:"type"`
	CreatedAt string         `json:"createdAt"`
	Files     []ManifestFile `json:"files"`
}

type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type Logger interface {
	Infof(format string, args ...any)
	Errorf(format string, args ...any)
}

func Apply(root string, desired Desired, meta ArtifactMeta, httpClient *http.Client, logger Logger, opts ApplyOptions) (ApplyOutcome, error) {
	downloads := filepath.Join(root, "downloads")
	versions := filepath.Join(root, "versions")
	current := filepath.Join(root, "current")

	atype := normalizeArtifactType(meta.Type)
	outcome := ApplyOutcome{PreApplyStatus: "skipped"}

	if logger != nil {
		logger.Infof("artifact apply start id=%s version=%s", desired.ArtifactID, desired.SoftwareVersion)
	}
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		return outcome, err
	}
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return outcome, err
	}

	archivePath := filepath.Join(downloads, desired.ArtifactID+".tar.gz")
	if logger != nil {
		logger.Infof("download start url=%s", desired.DownloadURL)
	}
	sum, err := downloadAndVerify(httpClient, desired.DownloadURL, archivePath, meta.SHA256)
	if err != nil {
		return outcome, err
	}
	if err := verifySignature(meta, sum, opts); err != nil {
		return outcome, err
	}
	if logger != nil {
		logger.Infof("download verified sha=%s", meta.SHA256)
	}

	versionDir := filepath.Join(versions, desired.SoftwareVersion)
	tmpDir := versionDir + ".tmp-" + fmt.Sprintf("%d", time.Now().UnixNano())
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return outcome, err
	}

	if err := extractTarGz(archivePath, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return outcome, err
	}
	if logger != nil {
		logger.Infof("extract complete")
	}

	manifest, err := verifyManifest(tmpDir, desired.SoftwareVersion, atype)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return outcome, err
	}
	if logger != nil {
		logger.Infof("manifest verified")
	}

	planData, err := loadPlan(tmpDir)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return outcome, err
	}

	// Replace target directory
	if err := os.RemoveAll(versionDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return outcome, err
	}
	if err := os.Rename(tmpDir, versionDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return outcome, err
	}

	ctx := ApplyContext{
		Root:       root,
		VersionDir: versionDir,
		Desired:    desired,
		Meta:       meta,
		Manifest:   manifest,
		Logger:     logger,
	}

	if planData != nil {
		status, detail, err := runPreApplySteps(*planData, ctx)
		outcome.PreApplyStatus = status
		outcome.PreApplyError = detail
		if err != nil {
			return outcome, err
		}
	}

	switch atype {
	case "app_bundle", "config_bundle", "data_bundle", "agent_bundle":
		return outcome, applyBundle(ctx, planData, current)
	case "firmware":
		return outcome, applyFirmware(ctx, planData, current, opts)
	case "container_image":
		return outcome, applyContainerImage(ctx, planData, current, opts)
	default:
		return outcome, fmt.Errorf("unsupported artifact type: %s", atype)
	}
}

func normalizeArtifactType(val string) string {
	val = strings.TrimSpace(strings.ToLower(val))
	if val == "" {
		return "app_bundle"
	}
	return val
}

func loadPlan(root string) (*plan.Plan, error) {
	planPath := filepath.Join(root, "plan.yaml")
	data, err := os.ReadFile(planPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plan.yaml: %w", err)
	}

	p, err := plan.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse plan.yaml: %w", err)
	}
	if err := plan.Validate(*p); err != nil {
		return nil, fmt.Errorf("invalid plan.yaml: %w", err)
	}
	return p, nil
}

func downloadAndVerify(client *http.Client, url, dest, expectedSHA string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("missing download url")
	}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	tmpDest := dest + ".tmp"
	f, err := os.OpenFile(tmpDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	mw := io.MultiWriter(f, h)
	if _, err := io.Copy(mw, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmpDest)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpDest)
		return "", err
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if expectedSHA != "" && !strings.EqualFold(sum, expectedSHA) {
		_ = os.Remove(tmpDest)
		return "", fmt.Errorf("sha256 mismatch: expected %s got %s", expectedSHA, sum)
	}
	if err := os.Rename(tmpDest, dest); err != nil {
		_ = os.Remove(tmpDest)
		return "", fmt.Errorf("rename download: %w", err)
	}
	return sum, nil
}

func verifySignature(meta ArtifactMeta, shaHex string, opts ApplyOptions) error {
	mode := normalizeVerificationMode(opts)
	if meta.Signature == "" {
		if mode == "require_verified" || opts.RequireSignature {
			return fmt.Errorf("artifact signature required but missing")
		}
		return nil
	}
	if meta.SignatureKeyID != "" && len(opts.AllowedSigningKeyIDs) > 0 && !containsFold(opts.AllowedSigningKeyIDs, meta.SignatureKeyID) {
		if mode == "require_verified" {
			return fmt.Errorf("signature key id mismatch")
		}
		return nil
	}
	if meta.SignatureType != "" && len(opts.AllowedSignatureTypes) > 0 && !containsFold(opts.AllowedSignatureTypes, meta.SignatureType) {
		if mode == "require_verified" {
			return fmt.Errorf("signature type mismatch")
		}
		return nil
	}
	if mode == "require_verified" && len(opts.TrustKeys) > 0 && !strings.EqualFold(strings.TrimSpace(meta.VerificationStatus), "verified") {
		return fmt.Errorf("artifact verification status is %s", strings.TrimSpace(meta.VerificationStatus))
	}
	if len(opts.TrustKeys) > 0 {
		if err := verifyWithTrustBundle(meta, shaHex, opts); err != nil {
			if mode == "require_verified" {
				return err
			}
		}
		return nil
	}
	if opts.SigningPublicKeyPath == "" {
		if mode == "require_verified" || opts.RequireSignature {
			return fmt.Errorf("signature verification requires signing trust or SIGNING_PUB_KEY_PATH")
		}
		return nil
	}
	if opts.SigningKeyID != "" {
		if meta.SignatureKeyID == "" || !strings.EqualFold(meta.SignatureKeyID, opts.SigningKeyID) {
			if mode == "require_verified" || opts.RequireSignature {
				return fmt.Errorf("signature key id mismatch")
			}
			return nil
		}
	}
	if err := verifyEd25519(meta, shaHex, opts.SigningPublicKeyPath); err != nil {
		if mode == "require_verified" || opts.RequireSignature {
			return err
		}
	}
	return nil
}

func verifyWithTrustBundle(meta ArtifactMeta, shaHex string, opts ApplyOptions) error {
	key, ok := selectTrustKey(meta, opts.TrustKeys, opts)
	if !ok {
		return fmt.Errorf("trusted signing key not found")
	}
	if meta.SignatureType == "" || strings.EqualFold(meta.SignatureType, "ed25519") {
		return verifyEd25519PEM(meta, shaHex, key.PublicKeyPEM)
	}
	if strings.EqualFold(meta.SignatureType, "cosign") {
		return verifyCosignPEM(meta, shaHex, key.PublicKeyPEM)
	}
	return fmt.Errorf("unsupported signature type: %s", meta.SignatureType)
}

func verifyEd25519(meta ArtifactMeta, shaHex, path string) error {
	pubKey, err := loadEd25519PublicKey(path)
	if err != nil {
		return fmt.Errorf("load signing public key: %w", err)
	}
	return verifyEd25519Raw(meta.Signature, shaHex, pubKey)
}

func verifyEd25519PEM(meta ArtifactMeta, shaHex, publicKeyPEM string) error {
	pubKey, err := loadEd25519PublicKeyPEM(publicKeyPEM)
	if err != nil {
		return fmt.Errorf("load signing public key: %w", err)
	}
	return verifyEd25519Raw(meta.Signature, shaHex, pubKey)
}

func verifyEd25519Raw(signature, shaHex string, pubKey ed25519.PublicKey) error {
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	shaBytes, err := hex.DecodeString(shaHex)
	if err != nil {
		return fmt.Errorf("invalid sha256 for signature verification")
	}
	if !ed25519.Verify(pubKey, shaBytes, sigBytes) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

func verifyCosignPEM(meta ArtifactMeta, shaHex, publicKeyPEM string) error {
	pubAny, err := parsePublicKeyPEM(publicKeyPEM)
	if err != nil {
		return fmt.Errorf("load signing public key: %w", err)
	}
	sigBytes, err := base64.StdEncoding.DecodeString(meta.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	shaBytes, err := hex.DecodeString(shaHex)
	if err != nil {
		return fmt.Errorf("invalid sha256 for signature verification")
	}
	switch pubKey := pubAny.(type) {
	case *ecdsa.PublicKey:
		if !ecdsa.VerifyASN1(pubKey, shaBytes, sigBytes) {
			return fmt.Errorf("signature verification failed")
		}
		return nil
	case *rsa.PublicKey:
		if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, shaBytes, sigBytes); err == nil {
			return nil
		}
		if err := rsa.VerifyPSS(pubKey, crypto.SHA256, shaBytes, sigBytes, nil); err == nil {
			return nil
		}
		return fmt.Errorf("signature verification failed")
	case ed25519.PublicKey:
		if !ed25519.Verify(pubKey, shaBytes, sigBytes) {
			return fmt.Errorf("signature verification failed")
		}
		return nil
	default:
		return fmt.Errorf("unsupported public key type")
	}
}

func loadEd25519PublicKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadEd25519PublicKeyPEM(string(data))
}

func loadEd25519PublicKeyPEM(publicKeyPEM string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("invalid public key pem")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	pubKey, ok := pubAny.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ed25519")
	}
	return pubKey, nil
}

func parsePublicKeyPEM(publicKeyPEM string) (any, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("invalid public key pem")
	}
	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return pubAny, nil
}

func normalizeVerificationMode(opts ApplyOptions) string {
	mode := strings.TrimSpace(strings.ToLower(opts.VerificationMode))
	switch mode {
	case "allow_unsigned":
		return "allow_unsigned"
	case "warn_unsigned":
		return "warn_unsigned"
	case "require_verified":
		return "require_verified"
	default:
		if opts.RequireSignature {
			return "require_verified"
		}
		return "require_verified"
	}
}

func containsFold(items []string, needle string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func selectTrustKey(meta ArtifactMeta, keys []TrustKey, opts ApplyOptions) (TrustKey, bool) {
	candidates := keys
	if meta.SignatureKeyID != "" {
		for _, key := range candidates {
			if strings.EqualFold(strings.TrimSpace(key.KeyID), strings.TrimSpace(meta.SignatureKeyID)) {
				return key, true
			}
		}
	}
	if opts.SigningKeyID != "" {
		for _, key := range candidates {
			if strings.EqualFold(strings.TrimSpace(key.KeyID), strings.TrimSpace(opts.SigningKeyID)) {
				return key, true
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return TrustKey{}, false
}

func extractTarGz(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Name == "" {
			continue
		}
		if strings.Contains(hdr.Name, "..") || strings.HasPrefix(hdr.Name, "/") {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}
		path := filepath.Join(dest, hdr.Name)
		// Post-join bounds check: ensure the resolved path is still inside dest.
		if !strings.HasPrefix(path, dest+string(filepath.Separator)) && path != dest {
			return fmt.Errorf("invalid path in archive (traversal): %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			return fmt.Errorf("symlinks are not permitted in artifact archives: %s -> %s", hdr.Name, hdr.Linkname)
		}
	}
	return nil
}

func verifyManifest(root, expectedVersion, expectedType string) (Manifest, error) {
	manifestPath := filepath.Join(root, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest.json missing: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}
	if expectedVersion != "" && m.Version != "" && m.Version != expectedVersion {
		return Manifest{}, fmt.Errorf("manifest version mismatch: %s != %s", m.Version, expectedVersion)
	}
	if m.Type != "" && expectedType != "" && m.Type != expectedType {
		return Manifest{}, fmt.Errorf("manifest type mismatch: %s != %s", m.Type, expectedType)
	}

	for _, f := range m.Files {
		filePath := filepath.Join(root, f.Path)
		info, err := os.Stat(filePath)
		if err != nil {
			return Manifest{}, fmt.Errorf("missing file %s", f.Path)
		}
		if f.Size > 0 && info.Size() != f.Size {
			return Manifest{}, fmt.Errorf("size mismatch for %s", f.Path)
		}
		if f.SHA256 != "" {
			sum, err := fileSHA256(filePath)
			if err != nil {
				return Manifest{}, err
			}
			if !strings.EqualFold(sum, f.SHA256) {
				return Manifest{}, fmt.Errorf("sha256 mismatch for %s", f.Path)
			}
		}
	}
	return m, nil
}

func applyBundle(ctx ApplyContext, planData *plan.Plan, current string) error {
	if planData != nil {
		_, steps := splitPlanSteps(planData.Steps)
		if len(steps) > 0 {
			if ctx.Logger != nil {
				ctx.Logger.Infof("plan start steps=%d", len(steps))
			}
			results, err := planexec.Execute(plan.Plan{Steps: steps}, planexec.Context{
				WorkingDir: ctx.VersionDir,
				RenderRoot: ctx.VersionDir,
				Env:        buildPlanEnv(ctx),
			})
			if err != nil {
				if ctx.Logger != nil {
					if len(results) > 0 {
						last := results[len(results)-1]
						ctx.Logger.Errorf("plan failed step=%s type=%s error=%s", last.ID, last.Type, last.Error)
					} else {
						ctx.Logger.Errorf("plan failed error=%v", err)
					}
				}
				return fmt.Errorf("execute plan: %w", err)
			}
			if ctx.Logger != nil {
				ctx.Logger.Infof("plan complete steps=%d", len(results))
			}
		}
		if planData.Health != nil {
			if ctx.Logger != nil {
				ctx.Logger.Infof("health check start type=%s url=%s", planData.Health.Type, planData.Health.URL)
			}
			if err := planexec.ExecuteHealth(*planData.Health, planexec.Context{
				WorkingDir: ctx.VersionDir,
				RenderRoot: ctx.VersionDir,
				Env:        buildPlanEnv(ctx),
			}); err != nil {
				if ctx.Logger != nil {
					ctx.Logger.Errorf("health check failed error=%v", err)
				}
				return fmt.Errorf("health check: %w", err)
			}
			if ctx.Logger != nil {
				ctx.Logger.Infof("health check passed")
			}
		}
	}

	if _, err := switchSymlink(current, ctx.VersionDir); err != nil {
		return err
	}
	if ctx.Logger != nil {
		ctx.Logger.Infof("symlink switched target=%s", ctx.VersionDir)
	}
	return nil
}

func applyFirmware(ctx ApplyContext, planData *plan.Plan, current string, opts ApplyOptions) error {
	if opts.AllowUnsupported {
		if ctx.Logger != nil {
			ctx.Logger.Infof("firmware apply demo mode: using bundle apply")
		}
		return applyBundle(ctx, planData, current)
	}
	return defaultFirmwareApplier.ApplyFirmware(ctx)
}

func applyContainerImage(ctx ApplyContext, planData *plan.Plan, current string, opts ApplyOptions) error {
	if opts.AllowUnsupported {
		if ctx.Logger != nil {
			ctx.Logger.Infof("container image apply demo mode: using bundle apply")
		}
		return applyBundle(ctx, planData, current)
	}
	return defaultContainerImageApplier.ApplyContainerImage(ctx)
}

func runPreApplySteps(p plan.Plan, ctx ApplyContext) (string, string, error) {
	preSteps, _ := splitPlanSteps(p.Steps)
	if len(preSteps) == 0 {
		return "skipped", "", nil
	}
	env := buildPlanEnv(ctx)
	results, err := planexec.Execute(plan.Plan{Steps: preSteps}, planexec.Context{
		WorkingDir: ctx.VersionDir,
		RenderRoot: ctx.VersionDir,
		Env:        env,
	})
	if err != nil {
		detail := ""
		if ctx.Logger != nil {
			if len(results) > 0 {
				last := results[len(results)-1]
				ctx.Logger.Errorf("preApply failed step=%s error=%s", last.ID, last.Error)
				detail = last.Error
			} else {
				ctx.Logger.Errorf("preApply failed error=%v", err)
			}
		}
		if detail == "" {
			detail = err.Error()
		}
		return "error", detail, fmt.Errorf("preApply failed: %w", err)
	}
	return "success", "", nil
}

func splitPlanSteps(steps []plan.Step) (preApply []plan.Step, rest []plan.Step) {
	for _, step := range steps {
		if step.Type == "script.preApply" {
			preApply = append(preApply, step)
		} else {
			rest = append(rest, step)
		}
	}
	return preApply, rest
}

func buildPlanEnv(ctx ApplyContext) []string {
	env := []string{
		"HWOPS_ARTIFACT_ID=" + ctx.Desired.ArtifactID,
		"HWOPS_ARTIFACT_VERSION=" + ctx.Desired.SoftwareVersion,
		"HWOPS_ARTIFACT_TYPE=" + ctx.Meta.Type,
		"HWOPS_CONFIG_REV=" + ctx.Desired.ConfigRev,
		"HWOPS_ARTIFACT_ROOT=" + ctx.Root,
		"HWOPS_ARTIFACT_DIR=" + ctx.VersionDir,
	}
	if ctx.Desired.DownloadURL != "" {
		env = append(env, "HWOPS_DOWNLOAD_URL="+ctx.Desired.DownloadURL)
	}
	return env
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func switchSymlink(linkPath, target string) (string, error) {
	prev := ""
	if info, err := os.Lstat(linkPath); err == nil && (info.Mode()&os.ModeSymlink) != 0 {
		if p, err := os.Readlink(linkPath); err == nil {
			prev = p
		}
	}

	tmpLink := linkPath + ".tmp"
	_ = os.Remove(tmpLink)
	if err := os.Symlink(target, tmpLink); err != nil {
		return prev, err
	}
	if err := os.Rename(tmpLink, linkPath); err != nil {
		return prev, err
	}
	return prev, nil
}
