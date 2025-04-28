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
- **History**: Save history of used mock (useful for test).
- **Dynamic Reflection**: Custom Reflection v1 service serving in-memory descriptors.
- **Dynamic Mocks**: Define mocks at runtime for any service/method, returning proper Protobuf messages.
- **Optional Proxy**: Forward unmocked calls to a real backend via `--proxy` flag or "PROXY_TARGET" env.
- **Unary RPC Support**: Generic handler for unary calls.
- **Log system**: use grpclog to log all requests for the moment.

### Currently Not Supported (Coming soon)

- **Error Custom**: Mock doesn't support custom error details.
- **Streaming RPCs**: Client‑streaming, server‑streaming, and bidirectional RPCs.
- **Advanced Matcher** : Mock doesn't support advancded matcher (regexp, eq, etc.. on payload).
- **Advanced Auth Schemes**: Beyond simple metadata forwarding, OAuth token refresh interceptors, or other custom credential flows.

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
go run cmd/main.go \
  -grpc_port=":50051" \
  -http_port=":8080" \
  [--proxy="localhost:50052"]
```

- `-grpc_port`: address for gRPC (default `:50051`).
- `-http_port`: address for HTTP config API (default `:8080`).
- `--proxy`: optional backend for proxying unmocked calls.

### Upload a .proto

```bash
curl -X POST http://localhost:8080/upload-proto \
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

### Show history

```bash
curl -XGET http://localhost:8080/history
[
    {
        "id": "5643fa11-cde4-4139-9428-a827e795f126",
        "start_time": "2025-04-28T17:18:52.444474+02:00",
        "end_time": "2025-04-28T17:18:52.446256+02:00",
        "full_method": "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
        "messages": [
            {
                "direction": "recv",
                "timestamp": "2025-04-28T17:18:52.444733+02:00",
                "recognized": true,
                "proxified": false,
                "payload_string": "{\"listServices\":\"*\"}",
                "payload": {
                    "MessageRequest": {
                        "ListServices": "*"
                    }
                }
            },
            {
                "direction": "send",
                "timestamp": "2025-04-28T17:18:52.444874+02:00",
                "recognized": true,
                "proxified": false,
                "payload_string": "{\"originalRequest\":{\"listServices\":\"*\"}, \"listServicesResponse\":{\"service\":[{\"name\":\"grpc.reflection.v1.ServerReflection\"}, {\"name\":\"grpc.reflection.v1alpha.ServerReflection\"}, {\"name\":\"example.Greeter\"}]}}",
                "payload": {
                    "original_request": {
                        "MessageRequest": {
                            "ListServices": "*"
                        }
                    },
                    "MessageResponse": {
                        "ListServicesResponse": {
                            "service": [
                                {
                                    "name": "grpc.reflection.v1.ServerReflection"
                                },
                                {
                                    "name": "grpc.reflection.v1alpha.ServerReflection"
                                },
                                {
                                    "name": "example.Greeter"
                                }
                            ]
                        }
                    }
                }
            }
        ],
        "state": "CLOSED",
        "grpc_code": 0,
        "grpc_message": ""
    },
    {
        "id": "eb5e1519-5a94-4c5c-95df-8ea9832840ef",
        "start_time": "2025-04-28T17:18:58.753754+02:00",
        "end_time": "2025-04-28T17:18:58.857932+02:00",
        "full_method": "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
        "messages": [
            {
                "direction": "recv",
                "timestamp": "2025-04-28T17:18:58.753833+02:00",
                "recognized": true,
                "proxified": false,
                "payload_string": "{\"fileContainingSymbol\":\"example.Greeter\"}",
                "payload": {
                    "MessageRequest": {
                        "FileContainingSymbol": "example.Greeter"
                    }
                }
            },
            {
                "direction": "send",
                "timestamp": "2025-04-28T17:18:58.754086+02:00",
                "recognized": true,
                "proxified": false,
                "payload_string": "{\"originalRequest\":{\"fileContainingSymbol\":\"example.Greeter\"}, \"fileDescriptorResponse\":{\"fileDescriptorProto\":[\"CgtoZWxsby5wcm90bxIHZXhhbXBsZSIiCgxIZWxsb1JlcXVlc3QSEgoEbmFtZRgBIAEoCVIEbmFtZSImCgpIZWxsb1JlcGx5EhgKB21lc3NhZ2UYASABKAlSB21lc3NhZ2UyQQoHR3JlZXRlchI2CghTYXlIZWxsbxIVLmV4YW1wbGUuSGVsbG9SZXF1ZXN0GhMuZXhhbXBsZS5IZWxsb1JlcGx5YgZwcm90bzM=\"]}}",
                "payload": {
                    "original_request": {
                        "MessageRequest": {
                            "FileContainingSymbol": "example.Greeter"
                        }
                    },
                    "MessageResponse": {
                        "FileDescriptorResponse": {
                            "file_descriptor_proto": [
                                "CgtoZWxsby5wcm90bxIHZXhhbXBsZSIiCgxIZWxsb1JlcXVlc3QSEgoEbmFtZRgBIAEoCVIEbmFtZSImCgpIZWxsb1JlcGx5EhgKB21lc3NhZ2UYASABKAlSB21lc3NhZ2UyQQoHR3JlZXRlchI2CghTYXlIZWxsbxIVLmV4YW1wbGUuSGVsbG9SZXF1ZXN0GhMuZXhhbXBsZS5IZWxsb1JlcGx5YgZwcm90bzM="
                            ]
                        }
                    }
                }
            }
        ],
        "state": "CLOSED",
        "grpc_code": 0,
        "grpc_message": ""
    },
    {
        "id": "5702a9b3-8519-4166-ae90-ff0fd42a63f3",
        "start_time": "2025-04-28T17:18:58.75592+02:00",
        "end_time": "2025-04-28T17:18:58.857448+02:00",
        "full_method": "/example.Greeter/SayHello",
        "messages": [
            {
                "direction": "recv",
                "timestamp": "2025-04-28T17:18:58.756657+02:00",
                "recognized": true,
                "proxified": false,
                "payload_string": "{\"name\":\"Alice\"}",
                "payload": {
                    "name": "Alice"
                }
            },
            {
                "direction": "send",
                "timestamp": "2025-04-28T17:18:58.857437+02:00",
                "recognized": true,
                "proxified": false,
                "payload_string": "{\"message\":\"Hello from grpc-hot-mock!\"}",
                "payload": {
                    "message": "Hello from grpc-hot-mock!"
                }
            }
        ],
        "state": "CLOSED",
        "grpc_code": 0,
        "grpc_message": ""
    }
]
```


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
