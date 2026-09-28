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

FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen gen
COPY runtimes/go runtimes/go
COPY examples/clients/go examples/clients/go
COPY --from=contract-codegen /generated/go/contract.go gen/go/contracts/echo/contract.go
RUN CGO_ENABLED=0 go build -o /client ./examples/clients/go
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /client /client
ENTRYPOINT ["/client"]
