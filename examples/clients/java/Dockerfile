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
RUN /proto-contract check --proto proto/demo/v1/echo.proto --proto-path proto --service demo.v1.EchoService --lock contracts/demo.echo.json && sh scripts/generate-example-contracts.sh java

FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /src
COPY examples/clients/java/pom.xml .
RUN mvn -q dependency:go-offline
COPY proto/demo/v1/echo.proto proto/demo/v1/echo.proto
COPY runtimes/java/src/main/java src/main/java
COPY examples/clients/java/src/main/java src/main/java
COPY runtimes/contract-vectors.json /contract-vectors.json
COPY --from=contract-codegen /src/generated/java/Contract.java src/main/java/protocontract/generated/echo/Contract.java
RUN mvn -q package -DskipTests
FROM eclipse-temurin:21-jre
COPY --from=build /src/target/proto-contract-client-demo-0.1.0.jar /client.jar
COPY --from=build /contract-vectors.json /contract-vectors.json
ENTRYPOINT ["java","-jar","/client.jar"]
