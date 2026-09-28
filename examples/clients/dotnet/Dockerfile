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
FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build
WORKDIR /src
COPY proto /proto
COPY runtimes/dotnet /runtimes/dotnet
COPY examples/clients/dotnet .
COPY --from=contract-codegen /generated/dotnet/Contract.cs Generated/Contract.cs
RUN dotnet publish ProtoContractClientDemo.csproj -c Release -o /out
FROM mcr.microsoft.com/dotnet/runtime:9.0
COPY --from=build /out /app
WORKDIR /app
ENTRYPOINT ["dotnet","ProtoContractClientDemo.dll"]
