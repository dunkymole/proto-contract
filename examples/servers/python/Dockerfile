FROM python:3.13-slim
WORKDIR /app
COPY examples/servers/python/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY proto/demo/v1/echo.proto /app/proto/demo/v1/echo.proto
RUN python -m grpc_tools.protoc -I/app/proto --python_out=/app --grpc_python_out=/app /app/proto/demo/v1/echo.proto
COPY runtimes/python/proto_contract.py /app/proto_contract.py
COPY examples/servers/python/server.py /app/server.py
EXPOSE 50055
ENTRYPOINT ["python", "/app/server.py"]
