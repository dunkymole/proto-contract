FROM golang:1.24-bookworm AS build
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
COPY proto proto
COPY runtimes/go runtimes/go
COPY examples/servers/go examples/servers/go
RUN mkdir -p examples/servers/go/gen && protoc -I proto --go_out=examples/servers/go/gen --go_opt=paths=source_relative --go-grpc_out=examples/servers/go/gen --go-grpc_opt=paths=source_relative proto/demo/v1/echo.proto
RUN CGO_ENABLED=0 go build -o /server ./examples/servers/go
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
EXPOSE 50051
ENTRYPOINT ["/server"]
