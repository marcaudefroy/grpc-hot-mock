
# Architecture Decision Record (ADR)

## Title
Dynamic gRPC Mock Server with Hot-Reloadable Protos and Custom Reflection

## Status
Proposed

## Context

We need a mock server for gRPC that:

- Allows uploading and compiling `.proto` files at runtime.
- Enables defining mocks dynamically via HTTP.
- Supports optional proxying of unmocked calls.
- Exposes service definitions through gRPC Reflection, eliminating client-side `.proto` dependencies.

Constraints:
- gRPC requires services to be registered before `Serve()`.
- Standard Reflection relies on registered services.

## Decision

### Two-Phase Configuration

Protocol Buffers encode messages into a compact binary format based on field numbers and wire types (see [Protobuf encoding](https://developers.google.com/protocol-buffers/docs/encoding)). At runtime, dynamically serializing or deserializing a Protobuf message requires access to its “MessageDescriptor  (see [`protoreflect.MessageDescriptor`](https://pkg.go.dev/google.golang.org/protobuf/reflect/protoreflect#MessageDescriptor)) which fully defines the schema: field names, numbers, types, and default values. Without these descriptors, both client and server lose Protobuf’s strong typing:

Client Impact: Generated client stubs depend on the descriptor to map binary wire data to concrete fields. In the absence of that schema, deserialization yields empty or default values, or outright errors, because the stub cannot determine how to interpret each field number.

Server Impact: On the server side, lacking a MessageDescriptor prevents parsing incoming request payloads to inspect field values for conditional mocking. The mock server is therefore reduced to selecting responses based only on the RPC path and method headers.

Therefore, we separate the mocking workflow into two phases:

1. **`/upload_proto` Endpoint**
   - Accepts `{filename, content}` containing raw `.proto` definitions.
   - Compiles them in-memory with Buf `protocompile` to generate fully linked descriptors (`linker.Files`).
   - Stores all `FileDescriptor` and `MessageDescriptor` in a registry, ensuring message types are known ahead of any mocking.
   

2. **`/mocks` Endpoint**
   - Accepts mock configurations: `{service, method, responseType, mockResponse, ...}`.
   - References the previously uploaded `MessageDescriptor` by `responseType` (e.g. `example.HelloReply`).
   - Stores the mock payload JSON and metadata, ready for invocation.


### Custom Reflection Service v1

- Implement `reflectionv1.ServerReflection` on in-memory descriptors.
- Serve `ListServices` and `FileByFilename` without calling `RegisterService` after `Serve()`.

### Generic gRPC Handler

- Use `grpc.UnknownServiceHandler` to intercept all calls.
- If mock exists, build typed response via `dynamicpb.NewMessage` + `protojson.Unmarshal`.
- Else if `--proxy` set, proxy call using `grpc.NewClient` and metadata forwarding.
- Else return `UNIMPLEMENTED`.

## Alternatives Considered

- **Restart Server** on each proto upload: breaks availability and complicates state.
- **Register Services at Startup** only: no runtime addition of schemas.
- **Use Standard Reflection** with `RegisterService`: conflicts with gRPC’s registration timing.

## Consequences

- **Positive**
  - True hot-reload of proto definitions.
  - Proper Reflection support for dynamic services.
  - Typed responses and optional proxying.
- **Negative**
  - Increased complexity in Reflection implementation.
  - Only unary RPCs supported initially.
  - Manual import resolution required for multi-file protos.

