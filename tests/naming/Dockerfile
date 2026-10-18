FROM golang:1.24-bookworm AS codegen
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends protobuf-compiler python3 && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum ./
RUN go mod download && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.6 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
COPY . .
RUN go build -o /proto-contract ./cmd/proto-contract && go build -o /protoc-gen-proto-contract ./cmd/protoc-gen-proto-contract && python3 tests/naming/prepare.py

FROM codegen AS go-check
WORKDIR /out/go
RUN go mod tidy && go test ./... && touch /passed-go

FROM python:3.13-slim AS python-check
WORKDIR /app
RUN pip install --no-cache-dir grpcio-tools==1.71.0
COPY runtimes/python/proto_contract.py .
COPY tests/naming/proto proto
COPY --from=codegen /out/python/ .
RUN python -m grpc_tools.protoc -Iproto --python_out=. --grpc_python_out=. proto/nested-dir/name-edge.proto proto/fallback.proto proto/global.proto proto/keyword.proto && python check.py && touch /passed-python

FROM maven:3.9-eclipse-temurin-21 AS java-check
WORKDIR /src
COPY runtimes/java runtimes/java
RUN mvn -q -f runtimes/java/pom.xml install -DskipTests
WORKDIR /app
COPY tests/naming/proto proto
COPY --from=codegen /out/java/ .
RUN mvn -q package -DskipTests && java -jar target/proto-contract-client-demo-*.jar && touch /passed-java

FROM mcr.microsoft.com/dotnet/sdk:9.0 AS dotnet-check
WORKDIR /src
COPY runtimes/dotnet runtimes/dotnet
COPY runtimes/dotnet-aspnetcore runtimes/dotnet-aspnetcore
WORKDIR /app
COPY tests/naming/proto proto
COPY --from=codegen /out/dotnet/ .
RUN dotnet run --project Check.csproj && touch /passed-dotnet

FROM scratch AS verified
COPY --from=go-check /passed-go /
COPY --from=python-check /passed-python /
COPY --from=java-check /passed-java /
COPY --from=dotnet-check /passed-dotnet /
