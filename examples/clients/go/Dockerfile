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
RUN /proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json && sh scripts/generate-example-contracts.sh go

FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY artifacts/packages/go /packages/go
ENV GOPROXY=file:///packages/go,https://proxy.golang.org
ENV GONOSUMDB=github.com/dunkymole/proto-contract
RUN go mod init starter.local/demo && go mod edit -require=github.com/dunkymole/proto-contract@v0.2.1
COPY gen gen
COPY examples/clients/go examples/clients/go
COPY --from=contract-codegen /src/generated/go/contract.go gen/go/contracts/echo/contract.go
RUN sed -i 's|github.com/dunkymole/proto-contract/gen/|starter.local/demo/gen/|g' examples/clients/go/main.go && go mod tidy
RUN CGO_ENABLED=0 go build -o /client ./examples/clients/go
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /client /client
ENTRYPOINT ["/client"]
