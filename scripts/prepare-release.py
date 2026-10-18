"""Prepare GitHub Release assets from verified local packages; no network writes."""
import argparse
import hashlib
import json
import pathlib
import re
import tempfile
import zipfile


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def prepare(packages, destination):
    manifest = json.loads((packages / 'manifest.json').read_text(encoding='utf-8-sig'))
    version = manifest['adapterVersion']
    if not re.fullmatch(r'\d+\.\d+\.\d+', version):
        raise ValueError('expected a numeric package version')
    expected = {item['path']: item['sha256'] for item in manifest['artifacts']}
    actual = {p.relative_to(packages).as_posix() for p in packages.rglob('*')
              if p.is_file() and p != packages / 'manifest.json'}
    if actual != set(expected):
        raise ValueError('package inventory differs from the build manifest')
    for name, checksum in expected.items():
        if digest(packages / name) != checksum:
            raise ValueError(f'package checksum mismatch: {name}')
    required = [
        f'python/proto_contract_runtime-{version}-py3-none-any.whl',
        f'python/proto_contract_runtime-{version}.tar.gz',
        f'dotnet/ProtoContract.Runtime.{version}.nupkg',
        'java/pom.xml',
        *[f'java/proto-contract-runtime-{version}{suffix}.jar'
          for suffix in ('', '-sources', '-javadoc')],
        *[f'go/github.com/dunkymole/proto-contract/@v/v{version}.{suffix}'
          for suffix in ('zip', 'mod', 'info')],
        'go/github.com/dunkymole/proto-contract/@v/list',
    ]
    if set(required) != actual:
        raise ValueError('unexpected package files or inconsistent package versions')
    # Refuse reuse rather than mixing old release files with a new build.
    if destination.exists():
        raise ValueError('release destination already exists; choose a fresh directory')
    destination.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(dir=destination.parent) as temporary:
        staging = pathlib.Path(temporary) / 'release'
        staging.mkdir()
        for name in required:
            if name.startswith('go/'):
                continue
            filename = pathlib.Path(name).name
            if filename == 'pom.xml':
                filename = f'proto-contract-runtime-{version}.pom'
            (staging / filename).write_bytes((packages / name).read_bytes())
        # Preserve the complete file-proxy layout so the Go asset remains usable.
        with zipfile.ZipFile(staging / f'proto-contract-go-proxy-{version}.zip', 'w') as archive:
            for name in sorted(n for n in required if n.startswith('go/')):
                entry = zipfile.ZipInfo(name.removeprefix('go/'), (2026, 10, 18, 0, 0, 0))
                entry.compress_type = zipfile.ZIP_DEFLATED
                entry.external_attr = 0o100644 << 16
                archive.writestr(entry, (packages / name).read_bytes())
        (staging / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
        sums = [f'{digest(file)}  {file.name}\n' for file in sorted(staging.iterdir())]
        (staging / 'SHA256SUMS').write_text(''.join(sums), encoding='utf-8')
        staging.rename(destination)
    return version


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--packages', type=pathlib.Path, default=pathlib.Path('artifacts/packages'))
    parser.add_argument('--out', type=pathlib.Path, default=pathlib.Path('artifacts/release'))
    args = parser.parse_args()
    print(prepare(args.packages, args.out))
