import hashlib
import importlib.util
import json
import pathlib
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location('release', pathlib.Path(__file__).parents[1] / 'scripts/prepare-release.py')
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = pathlib.Path(self.temp.name)
        self.packages = self.root / 'packages'
        names = [
            'python/proto_contract_runtime-0.4.0-py3-none-any.whl',
            'python/proto_contract_runtime-0.4.0.tar.gz',
            'dotnet/ProtoContract.Runtime.0.4.0.nupkg', 'java/pom.xml',
            'dotnet/ProtoContract.AspNetCore.0.4.0.nupkg',
            *[f'java/proto-contract-runtime-0.4.0{suffix}.jar' for suffix in ('', '-sources', '-javadoc')],
            *[f'go/github.com/dunkymole/proto-contract/@v/v0.4.0.{suffix}' for suffix in ('zip', 'mod', 'info')],
            'go/github.com/dunkymole/proto-contract/@v/list',
        ]
        self.manifest = {'adapterVersion': '0.4.0', 'sourceRevision': 'a' * 40,
                         'sourceHasChanges': False, 'artifacts': []}
        for name in names:
            path = self.packages / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(name.encode())
            self.manifest['artifacts'].append({'path': name, 'sha256': release.digest(path)})
        self.write_manifest()

    def write_manifest(self):
        (self.packages / 'manifest.json').write_text(json.dumps(self.manifest), encoding='utf-8-sig')

    def test_release_layout_checksums_and_repeatability(self):
        first, second = self.root / 'first', self.root / 'second'
        self.assertEqual(release.prepare(self.packages, first), '0.4.0')
        release.prepare(self.packages, second)
        checksums = (first / 'SHA256SUMS').read_text().splitlines()
        self.assertEqual(len(checksums), 10)
        for line in checksums:
            checksum, name = line.split('  ')
            self.assertEqual(hashlib.sha256((first / name).read_bytes()).hexdigest(), checksum)
            self.assertEqual((first / name).read_bytes(), (second / name).read_bytes())
        with zipfile.ZipFile(first / 'proto-contract-go-proxy-0.4.0.zip') as archive:
            self.assertIn('github.com/dunkymole/proto-contract/@v/v0.4.0.mod', archive.namelist())
        self.assertTrue((first / 'proto-contract-runtime-0.4.0.pom').is_file())
        self.assertTrue((first / 'ProtoContract.AspNetCore.0.4.0.nupkg').is_file())

    def test_modified_package_is_rejected(self):
        (self.packages / 'java/pom.xml').write_text('tampered')
        with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
            release.prepare(self.packages, self.root / 'out')
        self.assertFalse((self.root / 'out').exists())

    def test_missing_and_extra_packages_are_rejected(self):
        extra = self.packages / 'java/manifest.json'
        extra.write_text('old')
        with self.assertRaisesRegex(ValueError, 'inventory'):
            release.prepare(self.packages, self.root / 'out')
        extra.unlink()
        (self.packages / 'java/pom.xml').unlink()
        with self.assertRaisesRegex(ValueError, 'inventory'):
            release.prepare(self.packages, self.root / 'out')

    def test_version_mismatch_and_reused_destination_are_rejected(self):
        self.manifest['adapterVersion'] = '0.4.0'
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, 'versions'):
            release.prepare(self.packages, self.root / 'out')
        self.manifest['adapterVersion'] = '0.4.0'
        self.write_manifest()
        release.prepare(self.packages, self.root / 'out')
        with self.assertRaisesRegex(ValueError, 'already exists'):
            release.prepare(self.packages, self.root / 'out')


if __name__ == '__main__':
    unittest.main()
