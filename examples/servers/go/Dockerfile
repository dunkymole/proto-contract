FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen gen
COPY runtimes/go runtimes/go
COPY examples/servers/go examples/servers/go
RUN CGO_ENABLED=0 go build -o /server ./examples/servers/go
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
EXPOSE 50051
ENTRYPOINT ["/server"]
