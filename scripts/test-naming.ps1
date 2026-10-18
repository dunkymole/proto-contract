$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    docker build --file tests/naming/Dockerfile --target verified --tag proto-contract-naming-tests .
    if ($LASTEXITCODE -ne 0) { throw 'Native naming conformance tests failed' }
    Write-Host 'All four native generators match inferred bindings'
}
finally { Pop-Location }
