FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /src
COPY examples/servers/java/pom.xml .
RUN mvn -q dependency:go-offline
COPY proto/demo/v1/echo.proto proto/demo/v1/echo.proto
COPY runtimes/java/src/main/java src/main/java
COPY examples/servers/java/src/main/java src/main/java
RUN mvn -q package -DskipTests
FROM eclipse-temurin:21-jre
COPY --from=build /src/target/proto-contract-server-demo-0.1.0.jar /server.jar
EXPOSE 50053
ENTRYPOINT ["java","-jar","/server.jar"]
