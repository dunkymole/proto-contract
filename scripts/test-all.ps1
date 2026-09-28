$ErrorActionPreference = "Stop"
docker build -t proto-contract-tool .
docker run --rm -v "${PWD}:/workspace" -w /workspace proto-contract-tool check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
docker compose up --build --abort-on-container-exit --exit-code-from python-client
$code = $LASTEXITCODE
docker compose down --remove-orphans
exit $code
