FROM golang:1.24-bookworm AS contract-codegen
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY contracts/demo.echo.json ./contracts/demo.echo.json
COPY contracts/native-bindings.*.json ./contracts/
COPY proto ./proto
COPY scripts/generate-example-contracts.sh ./scripts/generate-example-contracts.sh
RUN go build -o /proto-contract ./cmd/proto-contract && go build -o /protoc-gen-proto-contract ./cmd/protoc-gen-proto-contract
RUN /proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json && sh scripts/generate-example-contracts.sh python

FROM python:3.13-slim
WORKDIR /app
COPY examples/clients/python/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY proto/demo/v1/echo.proto /app/proto/demo/v1/echo.proto
RUN python -m grpc_tools.protoc -I/app/proto --python_out=/app --grpc_python_out=/app /app/proto/demo/v1/echo.proto
COPY runtimes/python/proto_contract.py /app/runtime/proto_contract.py
COPY examples/clients/python/client.py /app/client.py
COPY --from=contract-codegen /src/generated/python/echo_contract.py /app/echo_contract.py
ENTRYPOINT ["python", "/app/client.py"]
