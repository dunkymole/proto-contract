"""Verify distribution contents and license notices before consumer tests."""
import pathlib
import json
import tarfile
import zipfile
import xml.etree.ElementTree as ET

root = pathlib.Path('/artifacts/packages')
version = json.loads((root / 'manifest.json').read_text(encoding='utf-8-sig'))['adapterVersion']
license_text = pathlib.Path('/license').read_text(encoding='utf-8-sig').strip()


def verify_zip(path, required, license_path):
    with zipfile.ZipFile(path) as archive:
        assert set(required).issubset(archive.namelist()), (path, archive.namelist())
        assert archive.read(license_path).decode('utf-8-sig').strip() == license_text, path
    print(f'PASS: {path.name} contents and MIT notice')


wheel = root / f'python/proto_contract_runtime-{version}-py3-none-any.whl'
verify_zip(wheel, ['proto_contract.py', f'proto_contract_runtime-{version}.dist-info/METADATA'],
           f'proto_contract_runtime-{version}.dist-info/licenses/LICENSE')
with tarfile.open(root / f'python/proto_contract_runtime-{version}.tar.gz') as archive:
    assert archive.extractfile(f'proto_contract_runtime-{version}/LICENSE').read().decode('utf-8-sig').strip() == license_text
    assert archive.extractfile(f'proto_contract_runtime-{version}/pyproject.toml') is not None
print('PASS: Python source distribution includes build metadata and MIT notice')

classes = ['ContractClientInterceptor', 'ContractServerInterceptor', 'StrictServerInterceptor', 'ServiceCoverage', 'ContractServerBuilder']
verify_zip(root / f'java/proto-contract-runtime-{version}.jar',
           [f'io/github/dunkymole/protocontract/{name}.class' for name in classes], 'META-INF/LICENSE')
with zipfile.ZipFile(root / f'java/proto-contract-runtime-{version}-sources.jar') as archive:
    assert all(f'io/github/dunkymole/protocontract/{name}.java' in archive.namelist() for name in classes)
print('PASS: Java sources included')
with zipfile.ZipFile(root / f'java/proto-contract-runtime-{version}-javadoc.jar') as archive:
    assert 'index.html' in archive.namelist()
print('PASS: Java API documentation included')

nuget = root / f'dotnet/ProtoContract.Runtime.{version}.nupkg'
verify_zip(nuget, ['lib/net8.0/ProtoContract.Runtime.dll', 'README.md', 'ProtoContract.Runtime.nuspec'], 'LICENSE')
with zipfile.ZipFile(nuget) as archive:
    metadata = ET.fromstring(archive.read('ProtoContract.Runtime.nuspec'))
    dependencies = metadata.findall('.//{*}dependency')
    assert any(dep.get('id') == 'Grpc.Core.Api' for dep in dependencies)
print('PASS: NuGet declares gRPC dependency')

prefix = f'github.com/dunkymole/proto-contract@v{version}/'
verify_zip(root / f'go/github.com/dunkymole/proto-contract/@v/v{version}.zip',
           [prefix + 'go.mod', prefix + 'runtimes/go/protocontract/interceptor.go',
            prefix + 'internal/contract/testdata/options/policy.proto',
            prefix + 'internal/contract/testdata/options/service.proto',
            prefix + 'runtimes/contract-vectors.json'], prefix + 'LICENSE')

verify_zip(root / f'dotnet/ProtoContract.AspNetCore.{version}.nupkg',
           ['lib/net8.0/ProtoContract.AspNetCore.dll', 'README.md', 'ProtoContract.AspNetCore.nuspec'], 'LICENSE')
