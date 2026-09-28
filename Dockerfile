FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
RUN CGO_ENABLED=0 go build -o /proto-contract ./cmd/proto-contract
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
COPY --from=build /proto-contract /usr/local/bin/proto-contract
ENTRYPOINT ["proto-contract"]
