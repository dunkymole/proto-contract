"""Checked package release operations. Only `publish` without --dry-run writes GitHub."""
import argparse
import datetime
import hashlib
import importlib.util
import io
import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile
import time
import tomllib
import xml.etree.ElementTree as ET
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
REPO = 'dunkymole/proto-contract'
WORKFLOW = '.github/workflows/package-release.yml'
BRIDGE = '87f0e1c86dcd53b64cd24948962ced5b3fc38f99'


def require(condition, message):
    if not condition:
        raise ValueError(message)


def version(value):
    require(re.fullmatch(r'(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)', value), 'version must be canonical MAJOR.MINOR.PATCH')
    require(all(int(x) <= 2147483647 for x in value.split('.')), 'version component too large')
    return value


def run(*args, cwd=ROOT, binary=False, env=None):
    result = subprocess.run([str(a) for a in args], cwd=cwd, capture_output=True, env=env)
    if result.returncode:
        raise RuntimeError(f'{args[0]} failed: {result.stderr.decode(errors="replace")[-4000:]}')
    return result.stdout if binary else result.stdout.decode('utf-8').strip()


def git(*args):
    return run('git', *args)


def api(path, method='GET', fields=None, optional=False, binary=False):
    args = ['gh', 'api', f'repos/{REPO}/{path}', '--method', method]
    if binary and path.startswith('releases/assets/'):
        args += ['-H', 'Accept: application/octet-stream']
    for key, value in (fields or {}).items():
        args += ['-f', f'{key}={value}']
    try:
        data = run(*args, binary=binary)
    except RuntimeError as error:
        if optional and method == 'GET' and '(HTTP 404)' in str(error):
            return None
        raise
    return data if binary else json.loads(data)


def config():
    data = json.loads((ROOT / 'release.json').read_text())
    version(data['version'])
    for name, count in data['targets'].items():
        require((ROOT / name).resolve().is_relative_to(ROOT.resolve()) and name != 'release.json', 'version target outside the source tree or self-referencing')
        require(type(count) is int and count > 0, 'version target counts must be positive integers')
    return data


def check_versions():
    data = config()
    for name, count in data['targets'].items():
        text = (ROOT / name).read_text(encoding='utf-8')
        require(text.count(data['version']) == count, f'version drift in {name}; expected {count} occurrences of {data["version"]}')
    # Also inspect real package metadata, so a matching comment/example cannot
    # disguise a stale package version when maintaining the allowlist.
    for name in data['targets']:
        if name == 'runtimes/python/pyproject.toml':
            actual = tomllib.loads((ROOT / name).read_text())['project']['version']
        elif name.endswith('/pom.xml'):
            actual = ET.parse(ROOT / name).getroot().find('{*}version').text
        elif name.startswith('runtimes/') and name.endswith('.csproj'):
            actual = ET.parse(ROOT / name).getroot().find('.//Version').text
        else:
            continue
        require(actual == data['version'], f'package metadata version drift in {name}')
    return data['version']


def clean_source(commit):
    require(re.fullmatch('[0-9a-f]{40}', commit), 'expected a full source commit SHA')
    require(git('rev-parse', 'HEAD') == commit, 'checkout differs from expected commit')
    require(not git('status', '--porcelain'), 'source checkout is dirty; commit reviewed changes first')


def tag_target(tag):
    ref = api(f'git/ref/tags/{tag}', optional=True)
    if ref is None:
        return None
    obj = ref['object']
    for _ in range(5):
        if obj['type'] == 'commit':
            return obj['sha']
        require(obj['type'] == 'tag', 'tag does not reference a commit')
        obj = api(f'git/tags/{obj["sha"]}')['object']
    raise ValueError('excessively nested tag')


def absent_release(v):
    require(tag_target(f'v{v}') is None, f'v{v} tag already exists')
    require(api(f'releases/tags/v{v}', optional=True) is None, f'v{v} release already exists')


def bump(v, commit, summary, date, dry_run=False):
    version(v)
    current = check_versions()
    clean_source(commit)
    require(tuple(map(int, v.split('.'))) > tuple(map(int, current.split('.'))), 'new version must increase')
    datetime.date.fromisoformat(date)
    require(summary.strip() and '\n' not in summary, 'provide a one-line release summary')
    absent_release(v)
    data = config()
    edits = {}
    for name in data['targets']:
        # An explicit, counted allowlist, never a repository-wide replacement.
        edits[ROOT / name] = (ROOT / name).read_text(encoding='utf-8').replace(current, v)
    changelog = ROOT / 'CHANGELOG.md'
    text = changelog.read_text(encoding='utf-8')
    require(not re.search(rf'^## {re.escape(v)}(?: |$)', text, re.M), 'changelog already contains the proposed version')
    unreleased = re.search(r'^## Unreleased\n(.*?)(?=^## |\Z)', text, re.M | re.S)
    heading = f'## {v} ({date})\n\n- {summary.strip()}\n'
    if unreleased:
        edits[changelog] = text[:unreleased.start()] + heading + unreleased[1] + text[unreleased.end():]
    else:
        pos = text.index('\n## ')
        edits[changelog] = text[:pos] + '\n' + heading + text[pos:]
    data['version'] = v
    edits[ROOT / 'release.json'] = json.dumps(data, indent=2) + '\n'
    if not dry_run:
        for path, text in edits.items():
            path.write_text(text, encoding='utf-8', newline='\n')
        check_versions()
    return {'operation': 'bump', 'dryRun': dry_run, 'from': current, 'to': v,
            'files': [str(p.relative_to(ROOT)) for p in edits]}


def package_paths(v):
    return {
        f'python/proto_contract_runtime-{v}-py3-none-any.whl', f'python/proto_contract_runtime-{v}.tar.gz',
        f'dotnet/ProtoContract.Runtime.{v}.nupkg', f'dotnet/ProtoContract.AspNetCore.{v}.nupkg',
        'java/pom.xml', *[f'java/proto-contract-runtime-{v}{suffix}.jar' for suffix in ('', '-sources', '-javadoc')],
        *[f'go/github.com/dunkymole/proto-contract/@v/v{v}.{suffix}' for suffix in ('zip', 'mod', 'info')],
        'go/github.com/dunkymole/proto-contract/@v/list',
    }


def asset_names(v):
    return {'manifest.json', f'proto-contract-go-proxy-{v}.zip',
            f'proto-contract-runtime-{v}.pom',
            *[pathlib.PurePosixPath(p).name for p in package_paths(v) if not p.startswith('go/') and p != 'java/pom.xml']}


def sha(data):
    return hashlib.sha256(data).hexdigest()


def verify_assets(root, v, commit, run_id=None, candidate=True):
    version(v)
    sums = {}
    for line in (root / 'SHA256SUMS').read_text().splitlines():
        require(re.fullmatch(r'[0-9a-f]{64}  [A-Za-z0-9_.-]+', line), 'invalid checksum entry')
        digest, name = line.split('  ')
        require(name not in sums and name != 'SHA256SUMS', 'duplicate or recursive checksum entry')
        sums[name] = digest
    expected = asset_names(v) | ({'candidate.json', 'RELEASE_NOTES.md'} if candidate else set())
    require(set(sums) == expected, 'unexpected or missing release assets')
    require({p.name for p in root.iterdir()} == expected | {'SHA256SUMS'}, 'release directory inventory differs')
    for name, digest in sums.items():
        require((root / name).is_file() and sha((root / name).read_bytes()) == digest, f'asset checksum mismatch: {name}')
    manifest = json.loads((root / 'manifest.json').read_text(encoding='utf-8-sig'))
    require(manifest['adapterVersion'] == v and manifest['sourceRevision'] == commit, 'wrong version or source commit')
    require(manifest['sourceHasChanges'] is False, 'dirty source provenance')
    entries = manifest['artifacts']
    require(len(entries) == len(package_paths(v)) and {p['path'] for p in entries} == package_paths(v), 'invalid original package inventory')
    with zipfile.ZipFile(root / f'proto-contract-go-proxy-{v}.zip') as proxy:
        require(len(proxy.namelist()) == 4 and set(proxy.namelist()) == {p[3:] for p in package_paths(v) if p.startswith('go/')}, 'invalid Go proxy inventory')
        require(sum(p.file_size for p in proxy.infolist()) < 128 << 20, 'Go proxy archive too large')
        for item in entries:
            name = pathlib.PurePosixPath(item['path']).name
            if item['path'].startswith('go/'):
                data = proxy.read(item['path'][3:])
            else:
                if name == 'pom.xml':
                    name = f'proto-contract-runtime-{v}.pom'
                data = (root / name).read_bytes()
            require(sha(data) == item['sha256'], f'package checksum mismatch: {item["path"]}')
    if candidate:
        metadata = json.loads((root / 'candidate.json').read_text())
        require(metadata['format'] == 1 and metadata['repository'] == REPO, 'unknown candidate format/repository')
        require(metadata['version'] == v and metadata['sourceRevision'] == commit and metadata['sourceHasChanges'] is False, 'candidate provenance mismatch')
        require(metadata['workflowRunId'] == run_id and type(metadata['workflowRunId']) is type(run_id), 'candidate workflow run mismatch')
        require(metadata['workflowPath'] == WORKFLOW, 'candidate workflow mismatch')
        require(re.fullmatch('[0-9a-f]{40}', metadata['sourceTree']), 'invalid source tree identity')
        require(re.fullmatch('[0-9a-f]{40}', metadata['grpcBridgeRevision']) and metadata['toolchains'], 'missing tested dependency/toolchain details')
        attempt = metadata['workflowAttempt']
        valid_attempt = attempt is None if run_id is None else type(attempt) is int and attempt > 0
        require(valid_attempt, 'invalid workflow attempt')
    return {'version': v, 'sourceRevision': commit, 'assetChecksums': len(sums), 'packageChecksums': len(entries)}


def release_notes(v, commit):
    text = (ROOT / 'CHANGELOG.md').read_text(encoding='utf-8')
    section = re.search(rf'^## {re.escape(v)}(?: .*?)?\n(.*?)(?=^## |\Z)', text, re.M | re.S)
    require(section and section[1].strip(), 'add release notes to CHANGELOG.md before preparation')
    return f'''# Proto Contract v{v}

{section[1].strip()}

Source: `{commit}`. Download all attached release assets, including `manifest.json`,
`candidate.json`, `RELEASE_NOTES.md` and `SHA256SUMS`, into one directory. Run
`sha256sum --check SHA256SUMS` there before installing the packages you need.
Package versions are independent of application API contract versions.
See [installation](https://github.com/{REPO}/blob/v{v}/docs/PACKAGES.md) and
[compatibility limits](https://github.com/{REPO}/blob/v{v}/docs/COMPATIBILITY.md).

```sh
python -m pip install ./proto_contract_runtime-{v}-py3-none-any.whl
mvn install:install-file -Dfile=proto-contract-runtime-{v}.jar -DpomFile=proto-contract-runtime-{v}.pom
dotnet nuget add source /absolute/path/to/downloads --name proto-contract-release
dotnet add package ProtoContract.Runtime --version {v}
dotnet add package ProtoContract.AspNetCore --version {v}
go get github.com/dunkymole/proto-contract@v{v}
go install github.com/dunkymole/proto-contract/cmd/proto-contract@v{v}
go install github.com/dunkymole/proto-contract/cmd/protoc-gen-proto-contract@v{v}
```

Keep NuGet.org enabled for transitive dependencies. GitHub hosts these assets;
there are no uploads to PyPI, Maven Central or NuGet.org. Go uses the public tag
and normal checksum verification. The Go test-proxy ZIP can differ from the
canonical public module ZIP; do not bypass public module checksum verification.
Cross-machine byte-for-byte reproducibility is not guaranteed for every ecosystem.
'''


def bundle(commit, out, packages, run_id=None, attempt=None):
    v = check_versions()
    clean_source(commit)
    require((run_id is None) == (attempt is None), 'workflow run and attempt must be provided together')
    if run_id is not None:
        require(str(run_id) == os.environ.get('GITHUB_RUN_ID') and str(attempt) == os.environ.get('GITHUB_RUN_ATTEMPT'), 'run identity must come from the workflow environment')
        require(os.environ.get('GITHUB_SHA') == commit, 'workflow source mismatch')
    spec = importlib.util.spec_from_file_location('prepare_release', ROOT / 'scripts/prepare-release.py')
    helper = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(helper)
    bridge = os.environ.get('GRPC_BRIDGE_REF') or BRIDGE
    require(re.fullmatch('[0-9a-f]{40}', bridge), 'pin the tested grpc-bridge revision to a full commit SHA')
    metadata = {'format': 1, 'repository': REPO, 'version': v, 'sourceRevision': commit,
                'sourceTree': git('rev-parse', 'HEAD^{tree}'), 'sourceHasChanges': False,
                'workflowPath': WORKFLOW, 'workflowRunId': run_id, 'workflowAttempt': attempt,
                'grpcBridgeRevision': bridge,
                'toolchains': {'go': run('go', 'version'), 'protoc': run('protoc', '--version'),
                               'python': sys.version,
                               'containerBases': [line for line in (ROOT / 'packaging/Dockerfile').read_text().splitlines() if line.startswith('FROM ')],
                               'buildDefinitions': ['Dockerfile', 'packaging/Dockerfile', WORKFLOW]}}
    helper.prepare(packages, out, metadata, release_notes(v, commit))
    return verify_assets(out, v, commit, run_id)


def prepare(commit, out):
    v = check_versions()
    clean_source(commit)
    absent_release(v)
    # Reuse the complete validation suite, including the same source-retention
    # compiler selected by the maintainer/CI PATH. Output is streamed for progress.
    commands = [['go', 'test', './...'], ['go', 'vet', './...'],
                [sys.executable, '-m', 'unittest', 'discover', '-s', 'packaging', '-p', 'test_release*.py'],
                ['go', 'run', 'golang.org/x/vuln/cmd/govulncheck@v1.9.0', './...'],
                ['bash', 'scripts/test-custom-options.sh'],
                ['bash', 'scripts/test-editions.sh'],
                ['pwsh', '-NoProfile', '-File', 'scripts/test-packages.ps1'],
                ['pwsh', '-NoProfile', '-File', 'scripts/test-naming.ps1'],
                ['pwsh', '-NoProfile', '-File', 'scripts/test-rollout.ps1']]
    for command in commands:
        subprocess.run(command, cwd=ROOT, check=True)
    return bundle(commit, out, ROOT / 'artifacts/packages')


def download_artifact(run_id, artifact_id, v, commit, destination):
    result = api(f'actions/runs/{run_id}')
    require(result['conclusion'] == 'success' and result['status'] == 'completed', 'candidate workflow did not succeed')
    require(result['head_sha'] == commit and result['head_branch'] == 'main' and result['event'] == 'workflow_dispatch', 'candidate must be built from the selected main commit by workflow_dispatch')
    require(result['path'] == WORKFLOW and result['repository']['full_name'] == REPO, 'wrong candidate workflow/repository')
    artifact = api(f'actions/artifacts/{artifact_id}')
    require(not artifact['expired'] and artifact['name'] == f'proto-contract-release-{v}', 'wrong or expired artifact')
    require(artifact['workflow_run']['id'] == run_id and artifact['workflow_run']['head_sha'] == commit, 'wrong artifact run/source')
    require(artifact['size_in_bytes'] < 128 << 20, 'artifact too large')
    archive_bytes = api(f'actions/artifacts/{artifact_id}/zip', binary=True)
    if artifact.get('digest'):
        require(artifact['digest'] == 'sha256:' + sha(archive_bytes), 'artifact archive digest mismatch')
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as archive:
        names = archive.namelist()
        require(len(names) == len(set(names)), 'duplicate archive entries')
        require(set(names) == asset_names(v) | {'candidate.json', 'RELEASE_NOTES.md', 'SHA256SUMS'}, 'unexpected artifact archive inventory')
        require(sum(p.file_size for p in archive.infolist()) < 128 << 20, 'artifact too large')
        for name in names:
            (destination / name).write_bytes(archive.read(name))
    verify_assets(destination, v, commit, run_id)
    metadata = json.loads((destination / 'candidate.json').read_text())
    require(metadata['workflowAttempt'] == result['run_attempt'], 'artifact is from a different workflow attempt')
    require(metadata['sourceTree'] == api(f'git/commits/{commit}')['tree']['sha'], 'artifact source tree differs from selected commit')
    return result


def release_state(v, commit, marker, notes, files):
    target = tag_target(f'v{v}')
    require(target is None or target == commit, 'existing tag references another commit; never move it')
    release = api(f'releases/tags/v{v}', optional=True)
    if release is not None:
        require(target == commit and release['body'] == marker + notes and not release['prerelease'], 'existing release belongs to a different candidate')
        assets = release['assets']
        require(len({a['name'] for a in assets}) == len(assets), 'duplicate uploaded assets')
        for asset in assets:
            require(asset['name'] in files, 'existing release has unexpected assets')
            require(asset['state'] == 'uploaded' and asset['size'] == len(files[asset['name']]), 'incomplete/conflicting asset; inspect and remove only the failed draft upload manually')
            digest = asset.get('digest')
            if digest:
                require(digest == 'sha256:' + sha(files[asset['name']]), 'conflicting uploaded asset; never overwrite it')
            else:
                require(api(f'releases/assets/{asset["id"]}', binary=True) == files[asset['name']], 'conflicting uploaded asset')
        if not release['draft']:
            require({a['name'] for a in assets} == set(files), 'published release inventory differs')
    return target, release


def verify_published(v, commit, destination, run_id=None, candidate=True, expected_files=None, go=True):
    version(v)
    require(re.fullmatch('[0-9a-f]{40}', commit), 'expected full commit SHA')
    require(tag_target(f'v{v}') == commit, 'published tag source mismatch')
    release = api(f'releases/tags/v{v}')
    require(not release['draft'] and not release['prerelease'], 'release is not public/stable')
    require(api('releases/latest')['tag_name'] == f'v{v}', 'release is not latest; do not move latest backwards')
    names = [a['name'] for a in release['assets']]
    expected_names = asset_names(v) | {'SHA256SUMS'} | ({'candidate.json', 'RELEASE_NOTES.md'} if candidate else set())
    require(len(names) == len(set(names)) and set(names) == expected_names, 'published release inventory differs')
    require(all(a['state'] == 'uploaded' for a in release['assets']) and sum(a['size'] for a in release['assets']) < 128 << 20, 'published assets incomplete or too large')
    for asset in release['assets']:
        data = api(f'releases/assets/{asset["id"]}', binary=True)
        if expected_files is not None:
            require(data == expected_files[asset['name']], 'published bytes differ from approved candidate')
        (destination / asset['name']).write_bytes(data)
    report = verify_assets(destination, v, commit, run_id, candidate)
    if candidate:
        marker = re.match(rf'<!-- proto-contract-candidate:{re.escape(REPO)}:{run_id}:(\d+):{commit} -->\n', release['body'])
        require(marker is not None, 'public release candidate identity mismatch')
        report['artifactId'] = int(marker[1])
    if go:
        # Fresh module cache; never use the local test proxy or a checksum bypass.
        for attempt in range(4):
            try:
                output = run('docker', 'run', '--rm', '-e', 'GOPROXY=https://proxy.golang.org',
                             '-e', 'GOSUMDB=sum.golang.org', 'golang:1.27.2-bookworm', 'sh', '-c',
                             f'go mod download -json github.com/{REPO}@v{v} && '
                             f'go install github.com/{REPO}/cmd/proto-contract@v{v} && '
                             f'go install github.com/{REPO}/cmd/protoc-gen-proto-contract@v{v}')
                module = json.loads(output)
                require(module['Origin']['Hash'] == commit and module.get('Sum'), 'Go module source/checksum mismatch')
                report['goModuleSum'] = module['Sum']
                break
            except (RuntimeError, ValueError):
                if attempt == 3:
                    raise
                time.sleep(10)
    report.update({'releaseUrl': release['html_url'], 'workflowRunId': run_id, 'public': True})
    return report


def publish(v, commit, run_id, artifact_id, dry_run=False, candidate_dir=None):
    version(v)
    require(re.fullmatch('[0-9a-f]{40}', commit), 'expected full commit SHA')
    with tempfile.TemporaryDirectory() as temporary:
        root = pathlib.Path(temporary)
        if candidate_dir is not None:
            require(dry_run and run_id is None and artifact_id is None, 'local candidates are accepted only for --dry-run')
            verify_assets(candidate_dir, v, commit)
            root = candidate_dir
        else:
            require(run_id and artifact_id, 'provide the exact workflow run ID and artifact ID')
            download_artifact(run_id, artifact_id, v, commit, root)
        marker = f'<!-- proto-contract-candidate:{REPO}:{run_id}:{artifact_id}:{commit} -->\n'
        notes = (root / 'RELEASE_NOTES.md').read_text(encoding='utf-8')
        if run_id:
            notes += f'\nValidation: https://github.com/{REPO}/actions/runs/{run_id}\nArtifact: {artifact_id}\n'
        files = {p.name: p.read_bytes() for p in root.iterdir()}
        target, release = release_state(v, commit, marker, notes, files)
        latest = api('releases/latest', optional=True)
        if latest and latest['tag_name'] != f'v{v}':
            require(tuple(map(int, v.split('.'))) > tuple(map(int, version(latest['tag_name'].removeprefix('v')).split('.'))), 'refusing to replace a newer latest release')
        if dry_run:
            return {'dryRun': True, 'version': v, 'sourceRevision': commit, 'workflowRunId': run_id,
                    'artifactId': artifact_id, 'assetCount': len(files), 'writes': [],
                    'next': 'Explicit publish would create/verify the tag, resume a matching draft, publish and verify public downloads/Go.'}
        if target is None:
            # The first public side effect occurs only after all candidate checks.
            tag = api('git/tags', 'POST', {'tag': f'v{v}', 'message': f'Proto Contract v{v}', 'object': commit, 'type': 'commit'})
            api('git/refs', 'POST', {'ref': f'refs/tags/v{v}', 'sha': tag['sha']})
        if release is None:
            notes_file = pathlib.Path(temporary) / 'notes.md'
            notes_file.write_text(marker + notes, encoding='utf-8', newline='\n')
            run('gh', 'release', 'create', f'v{v}', '--repo', REPO, '--verify-tag', '--draft', '--latest=false',
                '--target', commit, '--title', f'Proto Contract v{v}', '--notes-file', notes_file)
        # Re-read state on retries/races. Upload only absent assets, never clobber.
        _, release = release_state(v, commit, marker, notes, files)
        uploaded = {a['name'] for a in release['assets']}
        for name in sorted(set(files) - uploaded):
            run('gh', 'release', 'upload', f'v{v}', root / name, '--repo', REPO)
        _, release = release_state(v, commit, marker, notes, files)
        require({a['name'] for a in release['assets']} == set(files), 'draft upload inventory is incomplete')
        if release['draft']:
            # Another version may have published during a long upload.
            newest = api('releases/latest', optional=True)
            if newest and newest['tag_name'] != f'v{v}':
                require(tuple(map(int, v.split('.'))) > tuple(map(int, version(newest['tag_name'].removeprefix('v')).split('.'))), 'newer latest release appeared during upload; leaving this draft untouched')
            run('gh', 'release', 'edit', f'v{v}', '--repo', REPO, '--draft=false', '--latest')
        with tempfile.TemporaryDirectory() as downloaded:
            return verify_published(v, commit, pathlib.Path(downloaded), run_id, expected_files=files)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    sub.add_parser('check')
    p = sub.add_parser('preflight')
    p.add_argument('--commit', required=True)
    p = sub.add_parser('bump')
    p.add_argument('--version', required=True)
    p.add_argument('--commit', required=True)
    p.add_argument('--summary', required=True)
    p.add_argument('--date', required=True)
    p.add_argument('--dry-run', action='store_true')
    for name in ('prepare', 'bundle'):
        p = sub.add_parser(name)
        p.add_argument('--commit', required=True)
        p.add_argument('--out', type=pathlib.Path, required=True)
        if name == 'bundle':
            p.add_argument('--run-id', type=int)
            p.add_argument('--attempt', type=int)
    p = sub.add_parser('publish')
    p.add_argument('--version', required=True)
    p.add_argument('--commit', required=True)
    p.add_argument('--run-id', type=int)
    p.add_argument('--artifact-id', type=int)
    p.add_argument('--candidate', type=pathlib.Path)
    p.add_argument('--dry-run', action='store_true')
    p = sub.add_parser('verify')
    p.add_argument('--version', required=True)
    p.add_argument('--commit', required=True)
    p.add_argument('--run-id', type=int)
    p.add_argument('--legacy', action='store_true', help='verify releases predating candidate metadata')
    p.add_argument('--out', type=pathlib.Path, required=True)
    args = parser.parse_args()
    if args.command == 'check':
        result = {'version': check_versions(), 'consistent': True}
    elif args.command == 'preflight':
        v = check_versions()
        clean_source(args.commit)
        absent_release(v)
        result = {'version': v, 'sourceRevision': args.commit, 'unpublished': True}
    elif args.command == 'bump':
        result = bump(args.version, args.commit, args.summary, args.date, args.dry_run)
    elif args.command == 'prepare':
        result = prepare(args.commit, args.out)
    elif args.command == 'bundle':
        result = bundle(args.commit, args.out, ROOT / 'artifacts/packages', args.run_id, args.attempt)
    elif args.command == 'publish':
        result = publish(args.version, args.commit, args.run_id, args.artifact_id, args.dry_run, args.candidate)
    else:
        require(not args.out.exists(), 'verification output already exists; choose a fresh directory')
        args.out.mkdir(parents=True)
        result = verify_published(args.version, args.commit, args.out, args.run_id, not args.legacy)
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, RuntimeError, KeyError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit(f'release: {error}')
