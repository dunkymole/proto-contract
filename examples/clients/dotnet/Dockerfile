FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build
WORKDIR /src
COPY proto /proto
COPY runtimes/dotnet /runtimes/dotnet
COPY examples/clients/dotnet .
RUN dotnet publish ProtoContractClientDemo.csproj -c Release -o /out
FROM mcr.microsoft.com/dotnet/runtime:9.0
COPY --from=build /out /app
WORKDIR /app
ENTRYPOINT ["dotnet","ProtoContractClientDemo.dll"]
