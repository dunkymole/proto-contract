import json
import hashlib
import pathlib
import tempfile
import unittest

from npm_fixtures import write_npm_fixture
from npm_package import verify_npm_package


class NpmPackageTests(unittest.TestCase):
    def test_valid_native_inventory_and_identity(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = pathlib.Path(temporary) / 'package.tgz'
            write_npm_fixture(path, '0.6.0', 'a' * 40)
            self.assertEqual(verify_npm_package(path, '0.6.0', 'a' * 40)['nativeBinaries'], 12)

    def test_tampered_identity_provenance_inventory_and_binaries(self):
        def metadata(contents, name, key, value):
            data = json.loads(contents[name])
            data[key] = value
            contents[name] = json.dumps(data).encode()
        cases = {
            'identity': lambda c: metadata(c, 'package.json', 'name', '@other/compiler'),
            'version': lambda c: metadata(c, 'package.json', 'version', '0.5.0'),
            'source': lambda c: metadata(c, 'native-manifest.json', 'sourceRevision', 'f' * 40),
            'install script': lambda c: metadata(c, 'package.json', 'scripts', {'postinstall': 'curl example.com'}),
            'runtime dependency': lambda c: metadata(c, 'package.json', 'dependencies', {'go': '*'}),
            'Node baseline': lambda c: metadata(c, 'package.json', 'engines', {'node': '>=18'}),
            'missing target': lambda c: c.pop('native/darwin-arm64/proto-contract'),
            'extra file': lambda c: c.update({'unexpected': b'bad'}),
            'binary checksum': lambda c: c.update({'native/linux-x64/proto-contract': b'tampered'}),
        }
        for name, change in cases.items():
            with self.subTest(name=name), tempfile.TemporaryDirectory() as temporary:
                path = pathlib.Path(temporary) / 'package.tgz'
                write_npm_fixture(path, '0.6.0', 'a' * 40, change)
                with self.assertRaises(ValueError):
                    verify_npm_package(path, '0.6.0', 'a' * 40)

    def test_wrong_native_architecture_is_rejected_after_checksum_update(self):
        def mutate(contents):
            name = 'native/linux-x64/proto-contract'
            contents[name] = contents['native/linux-arm64/proto-contract']
            manifest = json.loads(contents['native-manifest.json'])
            manifest['files'][name] = hashlib.sha256(contents[name]).hexdigest()
            contents['native-manifest.json'] = json.dumps(manifest).encode()
        with tempfile.TemporaryDirectory() as temporary:
            path = pathlib.Path(temporary) / 'package.tgz'
            write_npm_fixture(path, '0.6.0', 'a' * 40, mutate)
            with self.assertRaisesRegex(ValueError, 'target mismatch'):
                verify_npm_package(path, '0.6.0', 'a' * 40)


if __name__ == '__main__':
    unittest.main()
