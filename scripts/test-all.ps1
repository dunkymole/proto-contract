$ErrorActionPreference = "Stop"
docker build -t proto-contract-tool .
docker run --rm -v "${PWD}:/workspace" -w /workspace proto-contract-tool check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
docker compose build
try {
    docker compose up -d go-server java-server dotnet-server python-server
    foreach ($client in @("python-client", "go-client", "java-client", "dotnet-client")) {
        docker compose run --rm $client
        if ($LASTEXITCODE -ne 0) { throw "$client failed" }
    }
    Write-Host "All 16 client/server combinations passed"
}
finally {
    docker compose down --remove-orphans
}
