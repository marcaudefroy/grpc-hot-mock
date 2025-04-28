# grpc-hot-mock

**grpc-hot-mock** is a dynamic, hot-reloadable gRPC mock/proxy server that lets you:

- **Register** `.proto` definitions at runtime, compile them in-memory, and extract descriptors.
- **Mock** any service/method by returning typed messages built from JSON.
- **Discover** services via a custom gRPC Reflection v1 service—no static `.proto` files needed by clients.
- **Optionally proxy** non-mocked calls to a real gRPC backend. 
- **History Tracking**: Records all gRPC exchanges (sent/received) for inspection.

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
- **History Tracking**: Records all gRPC exchanges (sent/received) for inspection.
- **Dynamic Reflection**: Custom Reflection v1 service serving in-memory descriptors.
- **Dynamic Mocks**: Define mocks at runtime for any service/method, returning proper Protobuf messages.
- **Optional Proxy**: Forward unmocked calls to a real backend via `--proxy` flag or "PROXY_TARGET" env.
- **Unary RPC Support**: Generic handler for unary calls.
- **Structured Logging**: All gRPC activity is logged via `grpclog`.

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

#### With go

```bash
go run cmd/main.go \
  -grpc_port=":50051" \
  -http_port=":8080" \
  [--proxy="localhost:50052"]
```

- `-grpc_port`: address for gRPC (default `:50051`).
- `-http_port`: address for HTTP config API (default `:8080`).
- `--proxy`: optional backend for proxying unmocked calls.

#### With docker

```bash
docker run --name grpc-hot-mock \
  -p 8080:8080 \
  -p 50051:50051 \
  -e PROXY_TARGET=temporal:7233 \
  ghcr.io/marcaudefroy/grpc-hot-mock:latest
```

### Register .proto Files

#### By compiling .proto files immediatly

- Compiles in-memory and registers all message and service descriptors.

##### With proto on json format

```bash
curl -X POST http://localhost:8080/protos/register/json \
  -H "Content-Type: application/json" \
  -d '{
        "files": [
          {
            "filename": "hello.proto",
            "content": "syntax=\"proto3\"; package example; message HelloRequest{string name=1;} message HelloReply{string message=1;} service Greeter{rpc SayHello(HelloRequest) returns(HelloReply);}"
          }
        ]
      }'
```

##### With proto on file format

```bash
curl -X POST http://localhost:8080/protos/register/file \
  -F "files=@/path/to/your/first.proto;fileName=your/relative/path/first.proto" \
  -F "files=@/path/to/your/second.proto;filename=your/relative/path/second.proto" 
```
Remarks : 
- The filename field in the multipart request must match exactly the import path expected inside the .proto files.
- The server extracts the full filename from the Content-Disposition header to correctly resolve imports during compilation.

Example : 

```
.
├── shared/
│   └── common.proto
└── api/
    └── v1/
        └── hello.proto
```

*shared/common.proto*
```.proto
syntax = "proto3";
package shared;

message Empty {}

```

*a*pi/v1/hello.proto
```
syntax = "proto3";
package example.v1;

import "shared/common.proto";

message HelloRequest {
  string name = 1;
}

message HelloReply {
  string message = 1;
  shared.Empty meta = 2;
}

service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply);
}
```

To register the protos, you can use the following command :
```bash
curl -X POST http://localhost:8080/protos/register/file \
  -F "files=@shared/common.proto;filename=shared/common.proto" \
  -F "files=@api/v1/hello.proto;filename=api/v1/hello.proto"
```

#### By ingest .proto files and compile later


```
curl -X POST http://localhost:8080/protos/ingest/json \
  -H "Content-Type: application/json" \
  -d '{
        "files": [
          {
            "filename": "common.proto",
            "content": "syntax=\"proto3\"; package shared; message Empty {}"
          }
        ]
      }'
```

```bash
curl -X POST http://localhost:8080/protos/ingest/file \
  -F "files=@shared/common.proto;filename=shared/common.proto" \
  -F "files=@api/v1/hello.proto;filename=api/v1/hello.proto"
```

*Continue to injest one or more .proto*

At the end, you can compile the protos :

```
curl -X POST http://localhost:8080/protos/ingest/compile
```

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

| Endpoint                  | Method | Description                                              |
|----------------------------|--------|----------------------------------------------------------|
| `/protos/register/json`    | POST   | Upload multiple `.proto` files via JSON and compile immediately. |
| `/protos/register/file`    | POST   | Upload multiple `.proto` files via `multipart/form-data` and compile immediately. |
| `/protos/ingest/json`      | POST   | Ingest multiple `.proto` files via JSON (deferred compilation). |
| `/protos/ingest/file`      | POST   | Ingest multiple `.proto` files via `multipart/form-data` (deferred compilation). |
| `/protos/ingest/compile`   | POST   | Compile and register all previously ingested `.proto` files. |
| `/mocks`                   | POST   | Register a mock configuration for a service/method.      |
| `/history`                 | GET    | Fetch the call history (captured gRPC exchanges).         |
| `/history/clear`           | POST   | Clear the saved call history.                            |


---


## License

MIT License
