$ErrorActionPreference = 'Stop'
$previousBridgeRef = $env:GRPC_BRIDGE_REF
$previousComposeProject = $env:COMPOSE_PROJECT_NAME
if (-not $env:GRPC_BRIDGE_REF) { $env:GRPC_BRIDGE_REF = '87f0e1c86dcd53b64cd24948962ced5b3fc38f99' }
if (-not $env:COMPOSE_PROJECT_NAME) { $env:COMPOSE_PROJECT_NAME = "pc-packages-$PID" }
Push-Location (Join-Path $PSScriptRoot '..')
try {
    & ./scripts/build-packages.ps1
    docker run --rm -v "${PWD}/artifacts:/artifacts:ro" -v "${PWD}/packaging/verify-artifacts.py:/verify.py:ro" -v "${PWD}/LICENSE:/license:ro" python:3.13-slim python /verify.py
    if ($LASTEXITCODE -ne 0) { throw 'Artifact contents verification failed' }
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
