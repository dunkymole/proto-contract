FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build
WORKDIR /src
COPY proto /proto
COPY runtimes/dotnet /runtimes/dotnet
COPY examples/servers/dotnet .
RUN dotnet publish ProtoContractDemo.csproj -c Release -o /out
FROM mcr.microsoft.com/dotnet/aspnet:9.0
COPY --from=build /out /app
WORKDIR /app
EXPOSE 50054
ENTRYPOINT ["dotnet","ProtoContractDemo.dll"]
