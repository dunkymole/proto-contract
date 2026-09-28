FROM golang:1.24-bookworm AS contract-codegen
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY contracts/demo.echo.json ./contracts/demo.echo.json
RUN go build -o /proto-contract ./cmd/proto-contract
RUN /proto-contract generate --lock contracts/demo.echo.json --lang python --out /generated/echo_contract.py && \
    /proto-contract generate --lock contracts/demo.echo.json --lang go --package echocontract --out /generated/go/contract.go && \
    /proto-contract generate --lock contracts/demo.echo.json --lang java --package protocontract.generated.echo --out /generated/java/Contract.java && \
    /proto-contract generate --lock contracts/demo.echo.json --lang dotnet --package ProtoContract.Generated.Echo --out /generated/dotnet/Contract.cs
FROM python:3.13-slim
WORKDIR /app
COPY examples/clients/python/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY proto/demo/v1/echo.proto /app/proto/demo/v1/echo.proto
RUN python -m grpc_tools.protoc -I/app/proto --python_out=/app --grpc_python_out=/app /app/proto/demo/v1/echo.proto
COPY runtimes/python/proto_contract.py /app/runtime/proto_contract.py
COPY examples/clients/python/client.py /app/client.py
COPY --from=contract-codegen /generated/echo_contract.py /app/echo_contract.py
ENTRYPOINT ["python", "/app/client.py"]
