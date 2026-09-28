FROM golang:1.24-bookworm AS contract-codegen
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY contracts/demo.echo.json ./contracts/demo.echo.json
COPY proto ./proto
RUN go build -o /proto-contract ./cmd/proto-contract
RUN /proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json
RUN /proto-contract generate --lock contracts/demo.echo.json --lang python --out /generated/echo_contract.py && \
    /proto-contract generate --lock contracts/demo.echo.json --lang go --package echocontract --out /generated/go/contract.go && \
    /proto-contract generate --lock contracts/demo.echo.json --lang java --package protocontract.generated.echo --out /generated/java/Contract.java && \
    /proto-contract generate --lock contracts/demo.echo.json --lang dotnet --package ProtoContract.Generated.Echo --out /generated/dotnet/Contract.cs

FROM python:3.13-slim
WORKDIR /app
COPY examples/servers/python/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY proto/demo/v1/echo.proto /app/proto/demo/v1/echo.proto
RUN python -m grpc_tools.protoc -I/app/proto --python_out=/app --grpc_python_out=/app /app/proto/demo/v1/echo.proto
COPY runtimes/python/proto_contract.py /app/proto_contract.py
COPY examples/servers/python/server.py /app/server.py
COPY --from=contract-codegen /generated/echo_contract.py /app/echo_contract.py
COPY examples/servers/python/test_contract.py /app/test_contract.py
RUN python -m unittest test_contract
EXPOSE 50055
ENTRYPOINT ["python", "/app/server.py"]
