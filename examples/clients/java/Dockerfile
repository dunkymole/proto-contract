FROM maven:3.9-eclipse-temurin-21 AS build
WORKDIR /src
COPY examples/clients/java/pom.xml .
RUN mvn -q dependency:go-offline
COPY proto/demo/v1/echo.proto proto/demo/v1/echo.proto
COPY runtimes/java/src/main/java src/main/java
COPY examples/clients/java/src/main/java src/main/java
RUN mvn -q package -DskipTests
FROM eclipse-temurin:21-jre
COPY --from=build /src/target/proto-contract-client-demo-0.1.0.jar /client.jar
ENTRYPOINT ["java","-jar","/client.jar"]
