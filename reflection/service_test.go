package reflection_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/marcaudefroy/grpc-hot-mock/reflection"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
)

// fakeStream implements reflectionv1.ServerReflection_ServerReflectionInfoServer
// for testing reflection service handlers.
type fakeStream struct {
	requests  []*reflectionv1.ServerReflectionRequest
	responses []*reflectionv1.ServerReflectionResponse
	recvIndex int
}

func (f *fakeStream) Recv() (*reflectionv1.ServerReflectionRequest, error) {
	if f.recvIndex >= len(f.requests) {
		return nil, io.EOF
	}
	r := f.requests[f.recvIndex]
	f.recvIndex++
	return r, nil
}

func (f *fakeStream) RecvMsg(m any) error {
	req, err := f.Recv()
	if err != nil {
		return err
	}

	pm, ok := m.(proto.Message)
	if !ok {
		return fmt.Errorf("RecvMsg: expected proto.Message, got %T", m)
	}

	if resetter, ok := pm.(interface{ Reset() }); ok {
		resetter.Reset()
	}

	proto.Merge(pm, req)
	return nil
}

func (f *fakeStream) SendMsg(m any) error {
	resp, ok := m.(*reflectionv1.ServerReflectionResponse)
	if !ok {
		return fmt.Errorf("SendMsg: expected *reflectionv1.ServerReflectionResponse, got %T", m)
	}
	f.responses = append(f.responses, resp)
	return nil
}

func (f *fakeStream) Send(resp *reflectionv1.ServerReflectionResponse) error {
	f.responses = append(f.responses, resp)
	return nil
}

func (f *fakeStream) SetHeader(metadata metadata.MD) error {
	return nil
}

func (f *fakeStream) Context() context.Context {
	return context.Background()
}

func (f *fakeStream) SendHeader(metadata metadata.MD) error {
	return nil
}

func (f *fakeStream) SetTrailer(metadata metadata.MD) {
}

func TestRegisterProtoFileAndGetDescriptor(t *testing.T) {
	registry := reflection.NewDefaultDescriptorRegistry()
	helloProto := `syntax = "proto3";
package example;

message HelloRequest { string name = 1; }
message HelloReply   { string message = 1; }
service Greeter {
  rpc SayHello (HelloRequest) returns (HelloReply);
}`

	// Register the proto file
	if err := registry.RegisterProtoFile("hello.proto", helloProto); err != nil {
		t.Fatalf("RegisterProtoFile failed: %v", err)
	}

	// Retrieve descriptors
	if _, ok := registry.GetMessageDescriptor("example.HelloRequest"); !ok {
		t.Error("HelloRequest descriptor not found")
	}
	if _, ok := registry.GetMessageDescriptor("example.HelloReply"); !ok {
		t.Error("HelloReply descriptor not found")
	}
}

func TestServerReflection_ListServices(t *testing.T) {
	registry := reflection.NewDefaultDescriptorRegistry()
	helloProto := `syntax = "proto3";
package example;

service Greeter { rpc SayHello (HelloRequest) returns (HelloReply); }
message HelloRequest { string name = 1; }
message HelloReply   { string message = 1; }`
	// Register proto
	if err := registry.RegisterProtoFile("hello.proto", helloProto); err != nil {
		t.Fatalf("failed to register proto: %v", err)
	}

	// Prepare a ListServices request
	req := &reflectionv1.ServerReflectionRequest{
		Host: "",
		MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{
			ListServices: "",
		},
	}
	stream := &fakeStream{requests: []*reflectionv1.ServerReflectionRequest{req}}

	// Call reflection
	if err := registry.ServerReflectionInfo(stream); err != nil {
		t.Fatalf("ServerReflectionInfo failed: %v", err)
	}

	// Expect one response
	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}
	resp := stream.responses[0]
	lsr, ok := resp.MessageResponse.(*reflectionv1.ServerReflectionResponse_ListServicesResponse)
	if !ok {
		t.Fatalf("expected ListServicesResponse, got %T", resp.MessageResponse)
	}
	services := lsr.ListServicesResponse.Service
	if len(services) != 1 || services[0].Name != "example.Greeter" {
		t.Errorf("unexpected services list: %v", services)
	}
}

func TestServerReflection_FileByFilename_NotFound(t *testing.T) {
	registry := reflection.NewDefaultDescriptorRegistry()
	// No protos registered
	req := &reflectionv1.ServerReflectionRequest{
		Host: "",
		MessageRequest: &reflectionv1.ServerReflectionRequest_FileByFilename{
			FileByFilename: "nonexistent.proto",
		},
	}
	stream := &fakeStream{requests: []*reflectionv1.ServerReflectionRequest{req}}

	if err := registry.ServerReflectionInfo(stream); err != nil {
		t.Fatalf("ServerReflectionInfo failed: %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}
	r := stream.responses[0]
	errResp, ok := r.MessageResponse.(*reflectionv1.ServerReflectionResponse_ErrorResponse)
	if !ok {
		t.Fatalf("expected ErrorResponse, got %T", r.MessageResponse)
	}
	if codes.Code(errResp.ErrorResponse.ErrorCode) != codes.NotFound {
		t.Errorf("expected NotFound code, got %v", errResp.ErrorResponse.ErrorCode)
	}
}
