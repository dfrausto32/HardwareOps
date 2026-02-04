package artifacts

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hardwareops/agent/internal/plan"
	"github.com/hardwareops/agent/internal/planexec"
)

type Desired struct {
	ArtifactID      string
	SoftwareVersion string
	ConfigRev       string
	DownloadURL     string
}

type ArtifactMeta struct {
	ArtifactID string
	SHA256     string
	SizeBytes  int64
	Version    string
	Type       string
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
	if err := downloadAndVerify(httpClient, desired.DownloadURL, archivePath, meta.SHA256); err != nil {
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
	case "app_bundle", "config_bundle", "data_bundle":
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

func downloadAndVerify(client *http.Client, url, dest, expectedSHA string) error {
	if url == "" {
		return fmt.Errorf("missing download url")
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	mw := io.MultiWriter(f, h)
	if _, err := io.Copy(mw, resp.Body); err != nil {
		return err
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if expectedSHA != "" && !strings.EqualFold(sum, expectedSHA) {
		return fmt.Errorf("sha256 mismatch: expected %s got %s", expectedSHA, sum)
	}
	return nil
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
