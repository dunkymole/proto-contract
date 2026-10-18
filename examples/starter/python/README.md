# Installed-package starter

Run `./scripts/test-packages.ps1` from the repository root to build unpublished
packages, run this starter, and run the complete cross-language matrix.

To rerun just the starter after building the artifacts:

```sh
docker build -f examples/starter/python/Dockerfile -t proto-contract-starter .
docker run --rm proto-contract-starter
```

Expected output:

```text
PASS: generated client accepted
PASS: missing and incompatible contracts rejected before handler entry
```

`main.py` is the entire application. It runs an ephemeral local gRPC server and
client, then shuts both down. Its only runtime dependency is the installed wheel;
it imports application-specific generated protobuf and contract modules alongside
it. The server rejects calls without metadata and calls from a different contract
major version before entering the handler.

For a separate project, create a virtual environment, install the local wheel and
`grpcio-tools==1.71.0`, copy your schema and checked contract lock, then generate:

```sh
python -m grpc_tools.protoc -Iproto --python_out=. --grpc_python_out=. proto/demo/v1/echo.proto
proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
proto-contract generate --lock contracts/demo.echo.json --lang python --out echo_contract.py
python main.py
```

Install/build the compiler separately as described in `docs/COMPILER.md`. Both
builds derive their service identity and version from their own checked lock.
The literal incompatible version in `main.py` is a negative test fixture.
