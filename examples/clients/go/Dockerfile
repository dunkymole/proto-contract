FROM golang:1.24-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
COPY proto proto
COPY runtimes/go runtimes/go
COPY examples/servers/go/gen examples/servers/go/gen
COPY examples/clients/go examples/clients/go
RUN CGO_ENABLED=0 go build -o /client ./examples/clients/go
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /client /client
ENTRYPOINT ["/client"]
