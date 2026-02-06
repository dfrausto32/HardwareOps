#!/usr/bin/env python3
import argparse, base64, hashlib, json, os, subprocess, tarfile, tempfile, time
from pathlib import Path

def sha256_file(path: Path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(1024*1024), b''):
            h.update(chunk)
    return h.hexdigest()

def compute_key_id(signing_key: Path):
    try:
        pub = subprocess.check_output(
            ["openssl", "pkey", "-in", str(signing_key), "-pubout", "-outform", "DER"],
            stderr=subprocess.DEVNULL,
        )
    except subprocess.CalledProcessError:
        return ""
    key_hash = hashlib.sha256(pub).hexdigest()
    return f"sha256:{key_hash}"

def sign_sha256(signing_key: Path, sha_hex: str):
    sha_bytes = bytes.fromhex(sha_hex)
    with tempfile.TemporaryDirectory() as tmpdir:
        sha_path = Path(tmpdir) / "sha.bin"
        sig_path = Path(tmpdir) / "sig.bin"
        sha_path.write_bytes(sha_bytes)
        try:
            subprocess.check_call(
                ["openssl", "pkeyutl", "-sign", "-inkey", str(signing_key), "-rawin", "-in", str(sha_path), "-out", str(sig_path)],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        except subprocess.CalledProcessError as exc:
            raise SystemExit(f"signing failed: {exc}")
        sig_b64 = base64.b64encode(sig_path.read_bytes()).decode("utf-8")
        return sig_b64

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--name', required=True)
    ap.add_argument('--version', required=True)
    ap.add_argument('--type', default='app_bundle')
    ap.add_argument('--input-dir', required=True)
    ap.add_argument('--out', required=True)
    ap.add_argument('--signing-key', default='')
    ap.add_argument('--signature-out', default='')
    ap.add_argument('--signing-key-id', default='')
    args = ap.parse_args()

    input_dir = Path(args.input_dir).resolve()
    if not input_dir.is_dir():
        raise SystemExit(f"input-dir not found: {input_dir}")

    files = []
    plan_path = input_dir / "plan.yaml"
    for path in input_dir.rglob('*'):
        if path.is_file():
            rel = path.relative_to(input_dir)
            if rel.as_posix() == "plan.yaml":
                continue
            files.append((path, rel))

    manifest = {
        "name": args.name,
        "version": args.version,
        "type": args.type,
        "createdAt": time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
        "files": [],
    }

    tmp_root = Path(args.out).with_suffix('').with_suffix('')
    staging = tmp_root.parent / (tmp_root.name + "-staging")
    if staging.exists():
        for p in staging.rglob('*'):
            if p.is_file():
                p.unlink()
        for p in sorted(staging.rglob('*'), reverse=True):
            if p.is_dir():
                p.rmdir()
    staging.mkdir(parents=True, exist_ok=True)

    files_dir = staging / 'files'
    files_dir.mkdir(parents=True, exist_ok=True)

    for src, rel in files:
        dest = files_dir / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_bytes(src.read_bytes())
        try:
            dest.chmod(src.stat().st_mode & 0o777)
        except OSError:
            pass
        manifest["files"].append({
            "path": f"files/{rel.as_posix()}",
            "sha256": sha256_file(dest),
            "size": dest.stat().st_size,
        })

    (staging / 'manifest.json').write_text(json.dumps(manifest, indent=2))
    if plan_path.is_file():
        (staging / 'plan.yaml').write_text(plan_path.read_text())

    out_path = Path(args.out).resolve()
    with tarfile.open(out_path, 'w:gz') as tf:
        tf.add(staging / 'manifest.json', arcname='manifest.json')
        if plan_path.is_file():
            tf.add(staging / 'plan.yaml', arcname='plan.yaml')
        tf.add(files_dir, arcname='files')

    size_bytes = out_path.stat().st_size
    sha = sha256_file(out_path)

    signature = ""
    signature_key_id = ""
    if args.signing_key:
        signing_key = Path(args.signing_key).resolve()
        if not signing_key.is_file():
            raise SystemExit(f"signing-key not found: {signing_key}")
        signature = sign_sha256(signing_key, sha)
        signature_key_id = args.signing_key_id or compute_key_id(signing_key)
        if args.signature_out:
            Path(args.signature_out).write_text(signature)

    print(json.dumps({
        "artifactPath": str(out_path),
        "sizeBytes": size_bytes,
        "sha256": sha,
        "signature": signature,
        "signatureAlg": "ed25519" if signature else "",
        "signaturePayload": "sha256" if signature else "",
        "signatureKeyId": signature_key_id,
    }, indent=2))

if __name__ == '__main__':
    main()
