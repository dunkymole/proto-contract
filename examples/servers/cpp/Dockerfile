FROM debian:bookworm-slim AS build
RUN apt-get update && apt-get install -y --no-install-recommends build-essential cmake protobuf-compiler protobuf-compiler-grpc libgrpc++-dev libprotobuf-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY proto proto
COPY runtimes/cpp runtime
COPY examples/servers/cpp/server.cc examples/servers/cpp/CMakeLists.txt ./
RUN cmake -S . -B build -DCMAKE_BUILD_TYPE=Release && cmake --build build -j2
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends libgrpc++1.51 libprotobuf32 && rm -rf /var/lib/apt/lists/*
COPY --from=build /src/build/server /server
EXPOSE 50052
ENTRYPOINT ["/server"]
