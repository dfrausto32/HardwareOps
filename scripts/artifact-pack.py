#!/usr/bin/env python3
import argparse, hashlib, json, os, tarfile, time
from pathlib import Path

def sha256_file(path: Path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda: f.read(1024*1024), b''):
            h.update(chunk)
    return h.hexdigest()

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--name', required=True)
    ap.add_argument('--version', required=True)
    ap.add_argument('--type', default='app_bundle')
    ap.add_argument('--input-dir', required=True)
    ap.add_argument('--out', required=True)
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

    print(json.dumps({
        "artifactPath": str(out_path),
        "sizeBytes": size_bytes,
        "sha256": sha,
    }, indent=2))

if __name__ == '__main__':
    main()
