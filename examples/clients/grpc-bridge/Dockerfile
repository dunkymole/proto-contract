# Pin the interceptor implementation from grpc-bridge PR #10 for reproducibility.
FROM alpine:3.22 AS source
RUN apk add --no-cache git
ARG GRPC_BRIDGE_REF=3bf0f1e0fbd49ea9056e2e491420914c3a7b7b1d
RUN git init /bridge && cd /bridge && \
    git remote add origin https://github.com/dunkymole/grpc-bridge.git && \
    git fetch --depth 1 origin "$GRPC_BRIDGE_REF" && git checkout --detach FETCH_HEAD

FROM golang:1.25-alpine AS bridge-build
WORKDIR /src
COPY --from=source /bridge/go.mod ./
COPY --from=source /bridge/cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -o /bridge ./cmd/bridge

FROM scratch AS bridge
COPY --from=bridge-build /bridge /bridge
COPY examples/clients/grpc-bridge/targets.json /config/targets.json
USER 65532:65532
ENV LISTEN=0.0.0.0:8080 UPSTREAM=go-server:50051 TARGETS_FILE=/config/targets.json
ENTRYPOINT ["/bridge"]

FROM node:24-alpine AS package
WORKDIR /bridge/web
COPY --from=source /bridge/web ./
RUN npm ci --no-audit --no-fund && npm pack --pack-destination /tmp

FROM node:24-alpine AS client
RUN apk add --no-cache protobuf
WORKDIR /app
COPY --from=package /tmp/dunkymole-grpc-bridge-*.tgz ./grpc-bridge.tgz
COPY examples/clients/grpc-bridge/package*.json ./
RUN npm ci --no-audit --no-fund
COPY examples/clients/grpc-bridge/*.ts examples/clients/grpc-bridge/tsconfig.json ./
COPY runtimes/typescript ./runtime
COPY proto /proto
RUN mkdir gen && protoc -I /proto \
    --plugin=protoc-gen-es=./node_modules/.bin/protoc-gen-es \
    --es_out=gen --es_opt=target=ts /proto/demo/v1/echo.proto && npm run check && npm test
CMD ["npm", "start"]
