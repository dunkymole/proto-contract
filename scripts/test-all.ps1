$ErrorActionPreference = "Stop"
docker build -t proto-contract-tool .
if ($LASTEXITCODE -ne 0) { throw "Compiler build failed" }
docker run --rm -v "${PWD}:/workspace" -w /workspace proto-contract-tool check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
if ($LASTEXITCODE -ne 0) { throw "Contract check failed" }
docker compose build
if ($LASTEXITCODE -ne 0) { throw "Matrix build failed" }
try {
    docker compose up -d go-server java-server dotnet-server python-server grpc-bridge
    if ($LASTEXITCODE -ne 0) { throw "Server startup failed" }
    foreach ($client in @("python-client", "go-client", "java-client", "dotnet-client", "grpc-bridge-client")) {
        docker compose run --rm -T $client
        if ($LASTEXITCODE -ne 0) { throw "$client failed" }
    }
    Write-Host "All 20 client/server combinations passed (including TypeScript via grpc-bridge)"
}
finally {
    docker compose down --remove-orphans
}
