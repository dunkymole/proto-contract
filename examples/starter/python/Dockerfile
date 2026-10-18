FROM golang:1.24-bookworm AS codegen
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
COPY contracts/demo.echo.json contracts/demo.echo.json
COPY proto proto
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
RUN go build -o /proto-contract ./cmd/proto-contract && /proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json && /proto-contract generate --lock contracts/demo.echo.json --lang python --out /echo_contract.py

FROM python:3.13-slim
WORKDIR /app
COPY artifacts/packages/python/ /packages/
RUN python -m venv /venv && /venv/bin/pip install /packages/*.whl grpcio-tools==1.71.0
COPY proto/demo/v1/echo.proto proto/demo/v1/echo.proto
RUN /venv/bin/python -m grpc_tools.protoc -Iproto --python_out=. --grpc_python_out=. proto/demo/v1/echo.proto
COPY --from=codegen /echo_contract.py .
COPY examples/starter/python/main.py .
CMD ["/venv/bin/python", "main.py"]
