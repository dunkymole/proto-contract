"""Verify distribution contents and license notices before consumer tests."""
import pathlib
import tarfile
import zipfile
import xml.etree.ElementTree as ET

root = pathlib.Path('/artifacts/packages')
license_text = pathlib.Path('/license').read_text(encoding='utf-8-sig').strip()


def verify_zip(path, required, license_path):
    with zipfile.ZipFile(path) as archive:
        assert set(required).issubset(archive.namelist()), (path, archive.namelist())
        assert archive.read(license_path).decode('utf-8-sig').strip() == license_text, path
    print(f'PASS: {path.name} contents and MIT notice')


wheel = root / 'python/proto_contract_runtime-0.3.0-py3-none-any.whl'
verify_zip(wheel, ['proto_contract.py', 'proto_contract_runtime-0.3.0.dist-info/METADATA'],
           'proto_contract_runtime-0.3.0.dist-info/licenses/LICENSE')
with tarfile.open(root / 'python/proto_contract_runtime-0.3.0.tar.gz') as archive:
    assert archive.extractfile('proto_contract_runtime-0.3.0/LICENSE').read().decode('utf-8-sig').strip() == license_text
    assert archive.extractfile('proto_contract_runtime-0.3.0/pyproject.toml') is not None
print('PASS: Python source distribution includes build metadata and MIT notice')

classes = ['ContractClientInterceptor', 'ContractServerInterceptor', 'StrictServerInterceptor', 'ServiceCoverage', 'ContractServerBuilder']
verify_zip(root / 'java/proto-contract-runtime-0.3.0.jar',
           [f'io/github/dunkymole/protocontract/{name}.class' for name in classes], 'META-INF/LICENSE')
with zipfile.ZipFile(root / 'java/proto-contract-runtime-0.3.0-sources.jar') as archive:
    assert all(f'io/github/dunkymole/protocontract/{name}.java' in archive.namelist() for name in classes)
print('PASS: Java sources included')
with zipfile.ZipFile(root / 'java/proto-contract-runtime-0.3.0-javadoc.jar') as archive:
    assert 'index.html' in archive.namelist()
print('PASS: Java API documentation included')

nuget = root / 'dotnet/ProtoContract.Runtime.0.3.0.nupkg'
verify_zip(nuget, ['lib/net8.0/ProtoContract.Runtime.dll', 'README.md', 'ProtoContract.Runtime.nuspec'], 'LICENSE')
with zipfile.ZipFile(nuget) as archive:
    metadata = ET.fromstring(archive.read('ProtoContract.Runtime.nuspec'))
    dependencies = metadata.findall('.//{*}dependency')
    assert any(dep.get('id') == 'Grpc.Core.Api' for dep in dependencies)
print('PASS: NuGet declares gRPC dependency')

prefix = 'github.com/dunkymole/proto-contract@v0.3.0/'
verify_zip(root / 'go/github.com/dunkymole/proto-contract/@v/v0.3.0.zip',
           [prefix + 'go.mod', prefix + 'runtimes/go/protocontract/interceptor.go',
            prefix + 'runtimes/contract-vectors.json'], prefix + 'LICENSE')

verify_zip(root / 'dotnet/ProtoContract.AspNetCore.0.3.0.nupkg',
           ['lib/net8.0/ProtoContract.AspNetCore.dll', 'README.md', 'ProtoContract.AspNetCore.nuspec'], 'LICENSE')
