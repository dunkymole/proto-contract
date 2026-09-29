FROM golang:1.24-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /usr/local/bin/proto-contract ./cmd/proto-contract
ENTRYPOINT ["proto-contract"]
