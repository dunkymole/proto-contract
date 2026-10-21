FROM python:3.13-slim AS python-package
WORKDIR /source
COPY runtimes/python/ .
RUN pip install --no-cache-dir build==1.2.2.post1 && SOURCE_DATE_EPOCH=1792281600 python -m build --outdir /packages/python

FROM maven:3.9-eclipse-temurin-21 AS java-package
WORKDIR /source
COPY runtimes/java/ .
RUN mvn -B package && mkdir -p /packages/java && cp target/*.jar pom.xml /packages/java/

FROM mcr.microsoft.com/dotnet/sdk:9.0 AS dotnet-package
WORKDIR /source
COPY runtimes/dotnet/ dotnet/
COPY runtimes/dotnet-aspnetcore/ dotnet-aspnetcore/
RUN dotnet pack dotnet/ProtoContract.Runtime.csproj -c Release -o /packages/dotnet && dotnet pack dotnet-aspnetcore/ProtoContract.AspNetCore.csproj -c Release -o /packages/dotnet

FROM python:3.13-slim AS go-package
WORKDIR /source
COPY go.mod go.sum LICENSE README.md release.json ./
COPY cmd/ cmd/
COPY internal/ internal/
COPY runtimes/go/ runtimes/go/
COPY runtimes/contract-vectors.json runtimes/contract-vectors.json
COPY contracts/demo.echo.json contracts/demo.echo.json
COPY gen/go/ gen/go/
COPY scripts/build-go-package.py /build-go-package.py
RUN python /build-go-package.py

FROM node:24-bookworm-slim AS npm-package
COPY --from=golang:1.27.2-bookworm /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
RUN apt-get update && apt-get install -y --no-install-recommends python3 ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /source
COPY go.mod go.sum LICENSE release.json ./
COPY cmd/ cmd/
COPY internal/ internal/
COPY npm/ npm/
COPY scripts/build-npm-package.py scripts/build-npm-package.py
ARG SOURCE_REVISION
RUN python3 scripts/build-npm-package.py --out /packages/npm --source-revision "$SOURCE_REVISION"

FROM scratch AS artifacts
COPY --from=python-package /packages/ /packages/
COPY --from=java-package /packages/ /packages/
COPY --from=dotnet-package /packages/ /packages/
COPY --from=go-package /packages/ /packages/
COPY --from=npm-package /packages/ /packages/
