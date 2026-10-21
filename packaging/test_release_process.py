import copy
import hashlib
import importlib.util
import io
import json
import pathlib
import shutil
import tempfile
import unittest
import zipfile
from unittest.mock import patch
from npm_fixtures import write_npm_fixture

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


tool = load('release_tool', ROOT / 'scripts/release.py')
helper = load('prepare_helper', ROOT / 'scripts/prepare-release.py')
VERSION, COMMIT, RUN, ARTIFACT = '0.5.0', 'a' * 40, 123, 456


def candidate(root, run_id=RUN, v=VERSION):
    packages = root / 'packages'
    manifest = {'adapterVersion': v, 'sourceRevision': COMMIT, 'sourceHasChanges': False, 'artifacts': []}
    for name in sorted(tool.package_paths(v)):
        path = packages / name
        path.parent.mkdir(parents=True, exist_ok=True)
        if name.startswith('npm/'):
            write_npm_fixture(path, v, COMMIT)
        else:
            path.write_bytes(name.encode())
        manifest['artifacts'].append({'path': name, 'sha256': tool.sha(path.read_bytes())})
    (packages / 'manifest.json').write_text(json.dumps(manifest))
    metadata = {'format': 1, 'repository': tool.REPO, 'version': v, 'sourceRevision': COMMIT,
                'sourceTree': 'c' * 40, 'grpcBridgeRevision': tool.BRIDGE, 'toolchains': {'go': 'go1.27.2'},
                'sourceHasChanges': False, 'workflowRunId': run_id, 'workflowAttempt': 1 if run_id else None,
                'workflowPath': tool.WORKFLOW}
    out = root / 'candidate'
    helper.prepare(packages, out, metadata, 'Reviewed release notes\n')
    return out


def rehash(root):
    (root / 'SHA256SUMS').write_text(''.join(f'{tool.sha(p.read_bytes())}  {p.name}\n' for p in sorted(root.iterdir()) if p.name != 'SHA256SUMS'))


class FakeGitHub:
    """Stateful API/CLI fake: exercises recovery using the real publisher logic."""
    def __init__(self):
        self.tag = None
        self.release = None
        self.data = {}
        self.writes = []
        self.fail_upload = None
        self.next_id = 1000
        self.go_failures = 0
        self.hide_draft_by_tag = False

    def api(self, path, method='GET', fields=None, optional=False, binary=False):
        if method != 'GET':
            self.writes.append((path, copy.deepcopy(fields)))
        if path == f'git/ref/tags/v{VERSION}':
            return {'object': {'type': 'commit', 'sha': self.tag}} if self.tag else None
        if path == 'git/tags' and method == 'POST':
            return {'sha': 'b' * 40}
        if path == 'git/refs' and method == 'POST':
            self.tag = COMMIT
            return {}
        if path == f'releases/tags/v{VERSION}':
            if self.release and self.release['draft'] and self.hide_draft_by_tag:
                return None
            return self.release
        if path.startswith('releases?per_page=100&page='):
            return [self.release] if self.release else []
        if path == 'releases/2000':
            return self.release
        if path == 'releases/latest':
            return {'tag_name': f'v{VERSION}' if self.release and not self.release['draft'] else 'v0.4.0'}
        if path.startswith('releases/assets/'):
            return self.data[int(path.split('/')[-1])]
        raise AssertionError((path, method))

    def run(self, *args, **kwargs):
        if args[0] == 'docker':
            if self.go_failures:
                self.go_failures -= 1
                raise RuntimeError('module proxy propagation delay')
            return json.dumps({'Origin': {'Hash': COMMIT}, 'Sum': 'h1:verified'})
        self.writes.append(args)
        operation = args[2]
        if operation == 'create':
            notes = pathlib.Path(args[args.index('--notes-file') + 1]).read_text(encoding='utf-8')
            self.release = {'draft': True, 'prerelease': False, 'body': notes, 'assets': [],
                            'id': 2000, 'tag_name': f'v{VERSION}',
                            'html_url': f'https://github.com/{tool.REPO}/releases/tag/v{VERSION}'}
        elif operation == 'upload':
            path = pathlib.Path(args[4])
            if path.name == self.fail_upload:
                self.fail_upload = None
                raise RuntimeError('interrupted upload')
            self.next_id += 1
            data = path.read_bytes()
            self.data[self.next_id] = data
            self.release['assets'].append({'name': path.name, 'id': self.next_id, 'size': len(data),
                                           'state': 'uploaded', 'digest': 'sha256:' + tool.sha(data)})
        elif operation == 'edit':
            self.release['draft'] = False
        else:
            raise AssertionError(args)
        return ''


class CandidateTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.out = candidate(self.root)

    def test_valid_candidate_checks_outer_and_nested_inventory(self):
        report = tool.verify_assets(self.out, VERSION, COMMIT, RUN)
        self.assertEqual(report['packageChecksums'], 12)
        self.assertEqual(report['assetChecksums'], 12)

    def test_npm_candidate_and_recomputed_outer_checksums(self):
        v = '0.6.0'
        out = candidate(self.root / 'npm', v=v)
        report = tool.verify_assets(out, v, COMMIT, RUN)
        self.assertEqual(report['packageChecksums'], 13)
        self.assertEqual(report['assetChecksums'], 13)
        def mutate(contents):
            package = json.loads(contents['package.json'])
            package['name'] = '@other/compiler'
            contents['package.json'] = json.dumps(package).encode()
        path = out / f'dunkymole-proto-contract-{v}.tgz'
        write_npm_fixture(path, v, COMMIT, mutate)
        manifest_path = out / 'manifest.json'
        manifest = json.loads(manifest_path.read_text())
        for entry in manifest['artifacts']:
            if entry['path'].startswith('npm/'):
                entry['sha256'] = tool.sha(path.read_bytes())
        manifest_path.write_text(json.dumps(manifest))
        rehash(out)
        with self.assertRaisesRegex(ValueError, 'npm package identity'):
            tool.verify_assets(out, v, COMMIT, RUN)

    def test_wrong_version_commit_run_are_rejected(self):
        for args in [('0.6.0', COMMIT, RUN), (VERSION, 'f' * 40, RUN), (VERSION, COMMIT, 999)]:
            with self.subTest(args=args), self.assertRaises(ValueError):
                tool.verify_assets(self.out, *args)

    def test_dirty_provenance_rejected_even_with_recomputed_checksums(self):
        path = self.out / 'manifest.json'
        data = json.loads(path.read_text())
        data['sourceHasChanges'] = True
        path.write_text(json.dumps(data))
        rehash(self.out)
        with self.assertRaisesRegex(ValueError, 'dirty'):
            tool.verify_assets(self.out, VERSION, COMMIT, RUN)

    def test_missing_extra_tampered_duplicate_assets(self):
        for failure in ['missing', 'extra', 'tampered', 'duplicate']:
            with self.subTest(failure=failure):
                out = self.root / failure
                shutil.copytree(self.out, out)
                if failure == 'missing':
                    (out / 'manifest.json').unlink()
                elif failure == 'extra':
                    (out / 'surprise').write_text('extra')
                elif failure == 'tampered':
                    (out / 'manifest.json').write_text('{}')
                else:
                    with (out / 'SHA256SUMS').open('a') as sums:
                        sums.write((out / 'SHA256SUMS').read_text().splitlines()[0] + '\n')
                with self.assertRaises(ValueError):
                    tool.verify_assets(out, VERSION, COMMIT, RUN)

    def test_original_manifest_cannot_drop_or_duplicate_packages(self):
        path = self.out / 'manifest.json'
        original = json.loads(path.read_text())
        for entries in [original['artifacts'][:-1], original['artifacts'] + [original['artifacts'][0]]]:
            data = dict(original, artifacts=entries)
            path.write_text(json.dumps(data))
            rehash(self.out)
            with self.assertRaisesRegex(ValueError, 'package inventory'):
                tool.verify_assets(self.out, VERSION, COMMIT, RUN)

    def test_nested_proxy_checksum_is_checked(self):
        proxy = self.out / f'proto-contract-go-proxy-{VERSION}.zip'
        with zipfile.ZipFile(proxy) as archive:
            contents = {n: archive.read(n) for n in archive.namelist()}
        first = next(iter(contents))
        contents[first] += b'tampered'
        with zipfile.ZipFile(proxy, 'w') as archive:
            for name, value in contents.items():
                archive.writestr(name, value)
        rehash(self.out)
        with self.assertRaisesRegex(ValueError, 'package checksum'):
            tool.verify_assets(self.out, VERSION, COMMIT, RUN)

    def test_artifact_run_and_archive_binding(self):
        buffer = io.BytesIO()
        with zipfile.ZipFile(buffer, 'w') as archive:
            for path in self.out.iterdir():
                archive.writestr(path.name, path.read_bytes())
        archive = buffer.getvalue()
        workflow = {'conclusion': 'success', 'status': 'completed', 'head_sha': COMMIT, 'head_branch': 'main',
                    'event': 'workflow_dispatch', 'path': tool.WORKFLOW, 'repository': {'full_name': tool.REPO}, 'run_attempt': 1}
        artifact = {'expired': False, 'name': f'proto-contract-release-{VERSION}',
                    'size_in_bytes': len(archive),
                    'workflow_run': {'id': RUN, 'head_sha': COMMIT}, 'digest': 'sha256:' + tool.sha(archive)}
        def api(path, **kwargs):
            return {f'actions/runs/{RUN}': workflow, f'actions/artifacts/{ARTIFACT}': artifact,
                    f'git/commits/{COMMIT}': {'tree': {'sha': 'c' * 40}},
                    f'actions/artifacts/{ARTIFACT}/zip': archive}[path]
        dest = self.root / 'downloaded'
        dest.mkdir()
        with patch.object(tool, 'api', side_effect=api):
            tool.download_artifact(RUN, ARTIFACT, VERSION, COMMIT, dest)
            for key, bad in [('conclusion', 'failure'), ('head_sha', 'f' * 40), ('head_branch', 'other'),
                             ('event', 'pull_request'), ('path', 'other.yml'), ('run_attempt', 2)]:
                saved = workflow[key]
                workflow[key] = bad
                with self.subTest(key=key), self.assertRaises(ValueError):
                    tool.download_artifact(RUN, ARTIFACT, VERSION, COMMIT, dest)
                workflow[key] = saved
            artifact['workflow_run']['id'] = 999
            with self.assertRaisesRegex(ValueError, 'artifact run'):
                tool.download_artifact(RUN, ARTIFACT, VERSION, COMMIT, dest)


class PublisherTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.out = candidate(self.root)
        self.github = FakeGitHub()
        def download(run_id, artifact_id, v, commit, dest):
            for path in self.out.iterdir():
                shutil.copyfile(path, dest / path.name)
        for name, replacement in [('api', self.github.api), ('run', self.github.run), ('download_artifact', download)]:
            patcher = patch.object(tool, name, side_effect=replacement)
            patcher.start()
            self.addCleanup(patcher.stop)

    def publish(self, **kwargs):
        return tool.publish(VERSION, COMMIT, RUN, ARTIFACT, **kwargs)

    def test_remote_dry_run_performs_no_writes(self):
        report = self.publish(dry_run=True)
        self.assertEqual(report['writes'], [])
        self.assertEqual(self.github.writes, [])

    def test_unicode_notes_survive_publication(self):
        (self.out / 'RELEASE_NOTES.md').write_text('Résumé — checked release notes\n', encoding='utf-8')
        rehash(self.out)
        self.publish()
        self.assertIn('Résumé — checked', self.github.release['body'])

    def test_local_candidate_can_only_be_dry_run(self):
        with self.assertRaisesRegex(ValueError, 'only for --dry-run'):
            tool.publish(VERSION, COMMIT, None, None, candidate_dir=self.out)

    def test_publish_then_retry_is_idempotent_and_verifies_downloads(self):
        report = self.publish()
        self.assertTrue(report['public'])
        self.assertEqual(report['goModuleSum'], 'h1:verified')
        writes = len(self.github.writes)
        self.publish()
        self.assertEqual(len(self.github.writes), writes)

    def test_interrupted_upload_resumes_without_overwriting(self):
        self.github.fail_upload = 'manifest.json'
        with self.assertRaisesRegex(RuntimeError, 'interrupted'):
            self.publish()
        uploaded = {a['name'] for a in self.github.release['assets']}
        self.assertTrue(uploaded)
        before = len(self.github.writes)
        self.publish()
        for command in self.github.writes[before:]:
            if command[:3] == ('gh', 'release', 'upload'):
                self.assertNotIn(pathlib.Path(command[4]).name, uploaded)

    def test_draft_hidden_by_tag_is_found_and_resumed_by_id(self):
        self.github.hide_draft_by_tag = True
        self.github.fail_upload = 'manifest.json'
        with self.assertRaisesRegex(RuntimeError, 'interrupted'):
            self.publish()
        self.assertTrue(self.github.release['draft'])
        self.publish()
        creates = [entry for entry in self.github.writes if entry[:3] == ('gh', 'release', 'create')]
        self.assertEqual(len(creates), 1)
        self.assertFalse(self.github.release['draft'])

    def test_conflicting_tag_fails_before_any_write(self):
        self.github.tag = 'f' * 40
        with self.assertRaisesRegex(ValueError, 'another commit'):
            self.publish()
        self.assertFalse(self.github.writes)

    def test_conflicting_draft_notes_and_assets_are_rejected(self):
        self.github.fail_upload = 'manifest.json'
        with self.assertRaises(RuntimeError):
            self.publish()
        body = self.github.release['body']
        self.github.release['body'] = 'a different candidate'
        with self.assertRaisesRegex(ValueError, 'different candidate'):
            self.publish()
        self.github.release['body'] = body
        self.github.release['assets'][0]['digest'] = 'sha256:' + '0' * 64
        with self.assertRaisesRegex(ValueError, 'conflicting uploaded asset'):
            self.publish()

    def test_go_failure_after_publication_can_be_retried_without_reupload(self):
        self.github.go_failures = 4
        with patch.object(tool.time, 'sleep'), self.assertRaises(RuntimeError):
            self.publish()
        self.assertFalse(self.github.release['draft'])
        before = len(self.github.writes)
        self.publish()
        self.assertEqual(len(self.github.writes), before)

    def test_post_publication_tampering_is_rejected(self):
        self.publish()
        asset = self.github.release['assets'][0]
        self.github.data[asset['id']] += b'tampered'
        destination = self.root / 'verify'
        destination.mkdir()
        with self.assertRaises(ValueError):
            tool.verify_published(VERSION, COMMIT, destination, RUN, go=False)


class ReleaseLookupTests(unittest.TestCase):
    def test_hidden_draft_lookup_paginates_and_reads_detail(self):
        draft = {'id': 42, 'tag_name': 'v0.6.0', 'draft': True}
        pages = {f'releases/tags/v0.6.0': None,
                 'releases?per_page=100&page=1': [{'tag_name': f'v0.0.{i}', 'id': i} for i in range(100)],
                 'releases?per_page=100&page=2': [draft], 'releases/42': draft}
        with patch.object(tool, 'api', side_effect=lambda path, **kwargs: pages[path]) as mocked:
            self.assertEqual(tool.find_release('0.6.0'), draft)
            self.assertEqual(mocked.call_args.args, ('releases/42',))

    def test_duplicate_exact_tags_fail_closed(self):
        with patch.object(tool, 'api', side_effect=[None, [{'tag_name': 'v0.6.0', 'id': 1}, {'tag_name': 'v0.6.0', 'id': 2}]]):
            with self.assertRaisesRegex(ValueError, 'multiple releases'):
                tool.find_release('0.6.0')


class VersionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        (self.root / 'metadata').write_text('version=0.4.0\nnegative-test=99.0.0\n')
        (self.root / 'CHANGELOG.md').write_text('# Changelog\n\n## 0.4.0 (2026-10-20)\n\n- Historical release.\n')
        (self.root / 'history').write_text('historical=0.4.0')
        (self.root / 'release.json').write_text(json.dumps({'version': '0.4.0', 'targets': {'metadata': 1}}))
        patcher = patch.object(tool, 'ROOT', self.root)
        patcher.start()
        self.addCleanup(patcher.stop)
        patcher = patch.object(tool, 'git', side_effect=lambda *args: '' if args[0] == 'status' else COMMIT)
        patcher.start()
        self.addCleanup(patcher.stop)
        patcher = patch.object(tool, 'absent_release')
        self.absent = patcher.start()
        self.addCleanup(patcher.stop)

    def bump(self, **kwargs):
        return tool.bump(VERSION, COMMIT, 'Next release.', '2026-10-20', **kwargs)

    def test_dry_run_no_edits_and_real_update_preserves_history(self):
        before = {p.name: p.read_bytes() for p in self.root.iterdir()}
        self.bump(dry_run=True)
        self.assertEqual(before, {p.name: p.read_bytes() for p in self.root.iterdir()})
        self.bump()
        self.assertEqual(tool.check_versions(), VERSION)
        self.assertEqual((self.root / 'history').read_text(), 'historical=0.4.0')
        self.assertIn('negative-test=99.0.0', (self.root / 'metadata').read_text())
        self.assertIn('## 0.4.0 (2026-10-20)', (self.root / 'CHANGELOG.md').read_text())

    def test_drift_is_rejected_before_any_edit(self):
        (self.root / 'metadata').write_text('version=0.3.0')
        before = (self.root / 'release.json').read_bytes()
        with self.assertRaisesRegex(ValueError, 'version drift'):
            self.bump()
        self.assertEqual(before, (self.root / 'release.json').read_bytes())

    def test_wrong_commit_dirty_checkout_and_existing_release_fail(self):
        for command in [('rev-parse', 'f' * 40), ('status', ' M metadata')]:
            with patch.object(tool, 'git', side_effect=lambda *args: command[1] if args[0] == command[0] else COMMIT):
                with self.assertRaises(ValueError):
                    self.bump()
        self.absent.side_effect = ValueError('tag already exists')
        with self.assertRaises(ValueError):
            self.bump()

    def test_invalid_versions(self):
        for value in ['01.2.3', '1.2', 'v0.5.0', '0.5.0-rc1', '0.5.0;echo', '2147483648.0.0']:
            with self.subTest(value=value), self.assertRaises(ValueError):
                tool.version(value)


if __name__ == '__main__':
    unittest.main()
