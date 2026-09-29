FROM golang:1.24-bookworm AS contract-codegen
ARG SCHEMA
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY examples/rollout/proto /schema
COPY examples/rollout/contracts /contracts
RUN go build -o /proto-contract ./cmd/proto-contract && \
    /proto-contract check --proto "/schema/${SCHEMA}/demo/v1/echo.proto" --proto-path "/schema/${SCHEMA}" \
      --service demo.v1.EchoService --lock "/contracts/${SCHEMA}.lock.json" && \
    /proto-contract generate --lock "/contracts/${SCHEMA}.lock.json" --lang python --out /generated/echo_contract.py

FROM python:3.13-slim
ARG SCHEMA
ARG BUILD_ID
WORKDIR /app
ENV BUILD_ID=${BUILD_ID}
COPY examples/rollout/proto /schema
COPY examples/rollout/server.py /app/server.py
COPY runtimes/python/proto_contract.py /app/proto_contract.py
COPY --from=contract-codegen /generated/echo_contract.py /app/echo_contract.py
RUN pip install --no-cache-dir grpcio==1.71.0 grpcio-tools==1.71.0 && \
    python -m grpc_tools.protoc -I "/schema/${SCHEMA}" --python_out=/app --grpc_python_out=/app demo/v1/echo.proto
EXPOSE 50051 9090
ENTRYPOINT ["python", "/app/server.py"]
