"""Inspect the npm compiler tarball without extraction or executing package code."""
import hashlib
import json
import re
import tarfile

COMMANDS = ('proto-contract', 'protoc-gen-proto-contract')
TARGETS = tuple((platform, arch) for platform in ('linux', 'darwin', 'win32') for arch in ('x64', 'arm64'))


def required_files():
    binaries = {f'native/{platform}-{arch}/{command}' + ('.exe' if platform == 'win32' else '')
                for platform, arch in TARGETS for command in COMMANDS}
    return binaries, binaries | {'package.json', 'README.md', 'LICENSE', 'THIRD_PARTY_NOTICES.md',
                                 'native-manifest.json', 'bin/launcher.cjs',
                                 *[f'bin/{command}.cjs' for command in COMMANDS]}


def require(value, message):
    if not value:
        raise ValueError(message)


def binary_target(data, platform, arch):
    if platform == 'linux':
        return len(data) >= 20 and data[:6] == b'\x7fELF\x02\x01' and int.from_bytes(data[18:20], 'little') == {'x64': 62, 'arm64': 183}[arch]
    if platform == 'darwin':
        return len(data) >= 8 and data[:4] == b'\xcf\xfa\xed\xfe' and int.from_bytes(data[4:8], 'little') == {'x64': 0x01000007, 'arm64': 0x0100000C}[arch]
    if len(data) < 64 or data[:2] != b'MZ':
        return False
    offset = int.from_bytes(data[60:64], 'little')
    return data[offset:offset+4] == b'PE\0\0' and int.from_bytes(data[offset+4:offset+6], 'little') == {'x64': 0x8664, 'arm64': 0xAA64}[arch]


def verify_npm_package(path, version, source_revision):
    binaries, expected = required_files()
    with tarfile.open(path, 'r:gz') as archive:
        members = archive.getmembers()
        names = [member.name for member in members]
        require(len(names) == len(set(names)) and set(names) == {'package/' + name for name in expected}, 'invalid npm archive inventory')
        require(all(member.isfile() for member in members), 'npm archive must contain regular files only')
        require(sum(member.size for member in members) < 512 << 20, 'npm archive too large')
        contents = {member.name[8:]: archive.extractfile(member).read() for member in members}
        for member in members:
            name = member.name[8:]
            if name in {f'bin/{command}.cjs' for command in COMMANDS} or (name in binaries and not name.endswith('.exe')):
                require(member.mode & 0o111, f'npm command is not executable: {name}')
    package = json.loads(contents['package.json'])
    require(package.get('name') == '@dunkymole/proto-contract' and package.get('version') == version, 'npm package identity/version mismatch')
    require(package.get('bin') == {command: f'bin/{command}.cjs' for command in COMMANDS}, 'invalid npm command bindings')
    require(not any(package.get(key) for key in ('scripts', 'dependencies', 'optionalDependencies', 'peerDependencies')), 'npm compiler must not have install scripts or runtime dependencies')
    require(package.get('license') == 'MIT', 'npm package license mismatch')
    require(package.get('engines', {}).get('node') in ('>=22', '>=22.0.0'), 'npm package must declare Node.js 22 or newer')
    manifest = json.loads(contents['native-manifest.json'])
    require(manifest.get('version') == version and manifest.get('sourceRevision') == source_revision and manifest.get('goVersion'), 'npm native provenance mismatch')
    require(set(manifest.get('files', {})) == binaries, 'invalid npm native checksum inventory')
    for name in binaries:
        digest = manifest['files'][name]
        require(isinstance(digest, str) and re.fullmatch('[0-9a-f]{64}', digest) and hashlib.sha256(contents[name]).hexdigest() == digest, f'npm native checksum mismatch: {name}')
    for platform, arch in TARGETS:
        for command in COMMANDS:
            name = f'native/{platform}-{arch}/{command}' + ('.exe' if platform == 'win32' else '')
            require(binary_target(contents[name], platform, arch), f'npm native target mismatch: {name}')
    require(contents['README.md'].strip() and contents['LICENSE'].strip() and contents['THIRD_PARTY_NOTICES.md'].strip(), 'npm package lacks documentation/license notices')
    return {'name': package['name'], 'version': version, 'nativeBinaries': len(binaries)}
