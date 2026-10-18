"""Build an unpublished Go module proxy artifact from the release source tree."""
import json
import pathlib
import zipfile

root = pathlib.Path('/source')
module = 'github.com/dunkymole/proto-contract'
version = 'v0.2.1'
out = pathlib.Path('/packages/go') / module / '@v'
out.mkdir(parents=True, exist_ok=True)
files = [root / 'go.mod', root / 'go.sum', root / 'LICENSE', root / 'README.md',
         root / 'runtimes/contract-vectors.json', root / 'contracts/demo.echo.json']
for directory in ('cmd', 'internal', 'runtimes/go'):
    files.extend(sorted((root / directory).rglob('*.go')))
files.extend(sorted((root / 'gen/go').rglob('*.go')))
with zipfile.ZipFile(out / f'{version}.zip', 'w', zipfile.ZIP_DEFLATED) as archive:
    for file in sorted(files):
        info = zipfile.ZipInfo(f'{module}@{version}/{file.relative_to(root).as_posix()}', (2026, 10, 18, 0, 0, 0))
        info.external_attr = 0o100644 << 16
        info.compress_type = zipfile.ZIP_DEFLATED
        archive.writestr(info, file.read_bytes())
(out / f'{version}.mod').write_bytes((root / 'go.mod').read_bytes())
(out / f'{version}.info').write_text(json.dumps({'Version': version, 'Time': '2026-10-18T00:00:00Z'}))
(out / 'list').write_text(version + '\n')
