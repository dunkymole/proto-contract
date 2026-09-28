FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY gen gen
COPY runtimes/go runtimes/go
COPY examples/clients/go examples/clients/go
RUN CGO_ENABLED=0 go build -o /client ./examples/clients/go
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /client /client
ENTRYPOINT ["/client"]
