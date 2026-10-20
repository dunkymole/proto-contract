$ErrorActionPreference = 'Stop'
Push-Location (Join-Path $PSScriptRoot '..')
try {
    $buildId = [guid]::NewGuid().ToString('N')
    $artifactRoot = [IO.Path]::GetFullPath((Join-Path $PWD 'artifacts'))
    $exportRoot = Join-Path $artifactRoot "package-build-$buildId"
    $packageRoot = Join-Path $artifactRoot 'packages'
    $previousRoot = Join-Path $artifactRoot "previous-packages-$buildId"
    foreach ($candidate in @($exportRoot, $packageRoot, $previousRoot)) {
        if (-not [IO.Path]::GetFullPath($candidate).StartsWith($artifactRoot + [IO.Path]::DirectorySeparatorChar)) {
            throw 'Package output must remain inside the workspace artifacts directory'
        }
    }
    docker build --file packaging/Dockerfile --target artifacts --output "type=local,dest=$exportRoot" .
    if ($LASTEXITCODE -ne 0) { throw 'Package build failed' }
    # Retain old artifacts separately; never mix versions in the current inventory.
    if (Test-Path -LiteralPath $packageRoot) { Move-Item -LiteralPath $packageRoot -Destination $previousRoot }
    Move-Item -LiteralPath (Join-Path $exportRoot 'packages') -Destination $packageRoot
    $files = Get-ChildItem artifacts/packages -Recurse -File |
        Where-Object { $_.Name -ne 'manifest.json' } |
        Sort-Object FullName |
        ForEach-Object {
            @{ path = [IO.Path]::GetRelativePath((Join-Path $PWD 'artifacts/packages'), $_.FullName).Replace('\', '/'); sha256 = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant() }
        }
    $manifest = @{
        sourceRevision = (git rev-parse HEAD)
        sourceHasChanges = [bool](git status --porcelain)
        adapterVersion = (Get-Content -LiteralPath release.json -Raw | ConvertFrom-Json).version
        artifacts = @($files)
    }
    $manifest | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 artifacts/packages/manifest.json
}
finally { Pop-Location }
