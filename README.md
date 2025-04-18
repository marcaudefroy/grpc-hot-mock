# grpc-hot-mock

**grpc-hot-mock** is a dynamic, hot-reloadable gRPC mock/proxy server that lets you:

- **Upload** `.proto` definitions at runtime, compile them in-memory, and extract descriptors.
- **Mock** any service/method by returning typed messages built from JSON.
- **Discover** services via a custom gRPC Reflection v1 service—no static `.proto` files needed by clients.
- **Optionally [*draft*] proxy** non-mocked calls to a real gRPC backend. 

## Table of Contents

1. [Features](#features)
2. [Prerequisites](#prerequisites)
3. [Installation](#installation)
4. [Usage](#usage)
   - [Start the Server](#start-the-server)
   - [Upload a .proto](#upload-a-proto)
   - [Register a Mock](#register-a-mock)
   - [Invoke with grpcurl](#invoke-with-grpcurl)
   - [HTTP Config Endpoints](#http-config-endpoints)
5. [Architecture Overview](#architecture-overview)
6. [Extending](#extending)
7. [License](#license)

---

## Features

- **Hot‑reloadable Protos**: Upload and compile `.proto` files on the fly via HTTP.
- **Well‑Known Types Auto‑Loading**: Well-known types like `google.protobuf.Timestamp` is automatically loaded (no need to post .proto file for thoses types).
- **Automated multi‑file import resolution**: upload multiple `.proto` files in a single request and compile them together, automatically resolving all inter‑file dependencies.
- **Dynamic Reflection**: Custom Reflection v1 service serving in-memory descriptors.
- **Dynamic Mocks**: Define mocks at runtime for any service/method, returning proper Protobuf messages.
- **Optional Proxy**: Forward unmocked calls to a real backend via `--proxy` flag.
- **Unary RPC Support**: Generic handler for unary calls.

### Currently Not Supported (Coming soon)

- **Error Custom**: Mock doesn't support custom error details.
- **Streaming RPCs**: Client‑streaming, server‑streaming, and bidirectional RPCs.
- **Advanced Matcher** : Mock doesn't support advancded matcher (regexp, eq, etc.. on payload).
- **Advanced Auth Schemes**: Beyond simple metadata forwarding, OAuth token refresh interceptors, or other custom credential flows.
- **Log system**: Logging system is not implemented yet.

## Prerequisites

- Go 1.20+

---

## Installation

```bash
git clone https://github.com/marcaudefroy/grpc-hot-mock.git
cd grpc-hot-mock
go mod tidy
```

---

## Usage

### Start the Server

```bash
go run main.go \
  -grpc_port=":50051" \
  -http_port=":8080" \
  [--proxy="localhost:50052"]
```

- `-grpc_port`: address for gRPC (default `:50051`).
- `-http_port`: address for HTTP config API (default `:8080`).
- `--proxy`: optional backend for proxying unmocked calls.

### Upload a .proto

```bash
curl -X POST http://localhost:8080/upload_proto \
     -H "Content-Type: application/json" \
     -d '{
           "filename":"hello.proto",
           "content":"syntax=\"proto3\"; package example; message HelloRequest{string name=1;} message HelloReply{string message=1;} service Greeter{rpc SayHello(HelloRequest) returns(HelloReply);}"
         }'
```

- Compiles in-memory and registers all message and service descriptors.

### Register a Mock

```bash
curl -X POST http://localhost:8080/mocks \
     -H "Content-Type: application/json" \
     -d '{
           "service":"example.Greeter",
           "method":"SayHello",
           "responseType":"example.HelloReply",
           "mockResponse": {"message":"Hello from grpc-hot-mock!"},
           "grpcStatus":0,
           "headers":{"custom":"header"},
           "delayMs":100
         }'
```

- Defines a mock for `/example.Greeter/SayHello`.


### Invoke with grpcurl

```bash
grpcurl -plaintext localhost:50051 list

grpcurl -plaintext \
  -d '{"name":"Alice"}' localhost:50051 example.Greeter/SayHello
```

- Uses Reflection v1: no need for `.proto` on the client.

### HTTP Config Endpoints

- `POST /upload_proto` — `{"filename":"...","content":"..."}`
- `POST /mocks` — `{"service":"...","method":"...","responseType":"...","mockResponse":{...},...}`

---

## Architecture Overview

1. **In-Memory Storage**: Maps filenames to `.proto` content.
2. **Dynamic Compilation**: Buf `protocompile` produces `linker.Files` descriptors.
3. **Descriptor Registry**: Stores all `FileDescriptor` and `MessageDescriptor` for runtime.
4. **Custom Reflection v1**: Implements `ServerReflection` on in-memory descriptors.
5. **Generic Handler**: Intercepts all RPCs, applies mock logic or proxies to backend.

---

## License

MIT License
