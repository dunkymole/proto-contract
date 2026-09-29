FROM golang:1.24-bookworm AS contract-plugin-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /protoc-gen-proto-contract ./cmd/protoc-gen-proto-contract

FROM python:3.13-slim
ARG SCHEMA
ARG BUILD_ID
WORKDIR /app
ENV BUILD_ID=${BUILD_ID}
COPY examples/rollout/proto /schema
COPY examples/rollout/contracts /contracts
COPY examples/rollout/server.py /app/server.py
COPY runtimes/python/proto_contract.py /app/proto_contract.py
COPY --from=contract-plugin-build /protoc-gen-proto-contract /usr/local/bin/protoc-gen-proto-contract
RUN pip install --no-cache-dir grpcio==1.71.0 grpcio-tools==1.71.0 && \
    python -m grpc_tools.protoc -I "/schema/${SCHEMA}" \
      --python_out=/app --grpc_python_out=/app \
      --plugin=protoc-gen-proto-contract=/usr/local/bin/protoc-gen-proto-contract \
      --proto-contract_out=/app \
      --proto-contract_opt="lang=python,bindings=/contracts/${SCHEMA}.python.bindings.json" \
      demo/v1/echo.proto
EXPOSE 50051 9090
ENTRYPOINT ["python", "/app/server.py"]
