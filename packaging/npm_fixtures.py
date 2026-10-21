"""Synthetic archive fixtures; never published or executed as native programs."""
import hashlib
import io
import json
import tarfile

from npm_package import COMMANDS, TARGETS


def npm_contents(version, commit):
    package = {'name': '@dunkymole/proto-contract', 'version': version, 'license': 'MIT',
               'engines': {'node': '>=22'}, 'bin': {name: f'bin/{name}.cjs' for name in COMMANDS}}
    contents = {'package.json': json.dumps(package).encode(), 'README.md': b'Compiler fixture',
                'LICENSE': b'MIT license fixture', 'THIRD_PARTY_NOTICES.md': b'Go licenses fixture',
                'bin/launcher.cjs': b'// Fixture'}
    for command in COMMANDS:
        contents[f'bin/{command}.cjs'] = b'#!/usr/bin/env node\n// Fixture'
    files = {}
    for platform, arch in TARGETS:
        data = bytearray(128)
        if platform == 'linux':
            data[:6] = b'\x7fELF\x02\x01'
            data[18:20] = {'x64': 62, 'arm64': 183}[arch].to_bytes(2, 'little')
        elif platform == 'darwin':
            data[:4] = b'\xcf\xfa\xed\xfe'
            data[4:8] = {'x64': 0x01000007, 'arm64': 0x0100000C}[arch].to_bytes(4, 'little')
        else:
            data[:2] = b'MZ'
            data[60:64] = (64).to_bytes(4, 'little')
            data[64:68] = b'PE\0\0'
            data[68:70] = {'x64': 0x8664, 'arm64': 0xAA64}[arch].to_bytes(2, 'little')
        for command in COMMANDS:
            name = f'native/{platform}-{arch}/{command}' + ('.exe' if platform == 'win32' else '')
            contents[name] = bytes(data)
            files[name] = hashlib.sha256(data).hexdigest()
    contents['native-manifest.json'] = json.dumps({'version': version, 'sourceRevision': commit, 'goVersion': 'go1.27.2', 'files': files}).encode()
    return contents


def write_npm_fixture(path, version, commit, mutate=None):
    contents = npm_contents(version, commit)
    if mutate:
        mutate(contents)
    with tarfile.open(path, 'w:gz') as archive:
        for name, data in sorted(contents.items()):
            info = tarfile.TarInfo('package/' + name)
            info.size = len(data)
            info.mode = 0o755 if name.startswith(('bin/', 'native/')) else 0o644
            archive.addfile(info, io.BytesIO(data))
