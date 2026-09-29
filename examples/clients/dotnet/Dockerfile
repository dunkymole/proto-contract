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

FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build
WORKDIR /src
COPY proto /proto
COPY runtimes/dotnet /runtimes/dotnet
COPY examples/clients/dotnet .
COPY runtimes/contract-vectors.json /contract-vectors.json
COPY --from=contract-codegen /generated/dotnet/Contract.cs Generated/Contract.cs
RUN dotnet publish ProtoContractClientDemo.csproj -c Release -o /out
FROM mcr.microsoft.com/dotnet/runtime:9.0
COPY --from=build /out /app
COPY --from=build /contract-vectors.json /contract-vectors.json
WORKDIR /app
ENTRYPOINT ["dotnet","ProtoContractClientDemo.dll"]
