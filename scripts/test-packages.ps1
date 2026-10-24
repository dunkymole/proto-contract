$ErrorActionPreference = 'Stop'
$previousBridgeRef = $env:GRPC_BRIDGE_REF
$previousComposeProject = $env:COMPOSE_PROJECT_NAME
if (-not $env:GRPC_BRIDGE_REF) { $env:GRPC_BRIDGE_REF = '56a7fc57078d4003553a4b0e5ec048c4bf9abef2' }
if (-not $env:COMPOSE_PROJECT_NAME) { $env:COMPOSE_PROJECT_NAME = "pc-packages-$PID" }
Push-Location (Join-Path $PSScriptRoot '..')
try {
    & ./scripts/build-packages.ps1
    docker run --rm -v "${PWD}/artifacts:/artifacts:ro" -v "${PWD}/packaging:/verification:ro" -v "${PWD}/LICENSE:/license:ro" python:3.13-slim python /verification/verify-artifacts.py
    if ($LASTEXITCODE -ne 0) { throw 'Artifact contents verification failed' }
    $packageVersion = (Get-Content -LiteralPath release.json -Raw | ConvertFrom-Json).version
    docker run --rm -v "${PWD}:/repo:ro" -w /repo node:24-alpine sh -c "apk add --no-cache protobuf && node npm/test-installed-package.cjs /repo/artifacts/packages/npm/dunkymole-proto-contract-$packageVersion.tgz"
    if ($LASTEXITCODE -ne 0) { throw 'Installed npm compiler/plugin verification failed' }
    # Artifacts are local files only. No registry publication takes place.
    docker build --file examples/starter/python/Dockerfile --tag proto-contract-starter .
    if ($LASTEXITCODE -ne 0) { throw 'Starter build failed' }
    docker run --rm proto-contract-starter
    if ($LASTEXITCODE -ne 0) { throw 'Starter verification failed' }
    & ./scripts/test-all.ps1 -SkipPackageBuild
}
finally {
    $env:GRPC_BRIDGE_REF = $previousBridgeRef
    $env:COMPOSE_PROJECT_NAME = $previousComposeProject
    Pop-Location
}
