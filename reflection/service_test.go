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
	"google.golang.org/protobuf/types/descriptorpb"
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

// Test batch ingest and compile order independence
func TestCompileAndRegister_BatchOrderIndependence(t *testing.T) {
	registry := reflection.NewDefaultDescriptorRegistry()

	// Define two protos: common and service, in reverse order
	serviceProto := `
syntax = "proto3";

package example.foo;

import "common.proto";

message FooReq { 
	string field = 1; 
}

service FooService { 
	rpc DoSomething(FooReq) returns (FooReq); 
}
`

	commonProto := `
syntax = "proto3";

package common;

message FooReq {
	string field = 1;
}
`

	// Ingest in wrong order
	registry.IngestProtoFile("service/foo.proto", serviceProto)
	registry.IngestProtoFile("common.proto", commonProto)

	// Compile and register all
	if err := registry.CompileAndRegister(); err != nil {
		t.Fatalf("batch compile failed: %v", err)
	}

	// List services via reflection
	req := &reflectionv1.ServerReflectionRequest{
		MessageRequest: &reflectionv1.ServerReflectionRequest_ListServices{ListServices: ""},
	}
	stream := &fakeStream{requests: []*reflectionv1.ServerReflectionRequest{req}}
	if err := registry.ServerReflectionInfo(stream); err != nil {
		t.Fatalf("ListServices RPC failed: %v", err)
	}
	if len(stream.responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(stream.responses))
	}
	resp := stream.responses[0]
	lsr, ok := resp.MessageResponse.(*reflectionv1.ServerReflectionResponse_ListServicesResponse)
	if !ok {
		t.Fatalf("unexpected response type %T", resp.MessageResponse)
	}
	found := false
	for _, svc := range lsr.ListServicesResponse.Service {
		if svc.Name == "example.foo.FooService" {
			found = true
		}
	}
	if !found {
		t.Errorf("FooService not found in ListServices: %v", lsr.ListServicesResponse.Service)
	}
}

// Test FileByFilename success and fallback error
func TestServerReflection_FileByFilename(t *testing.T) {
	registry := reflection.NewDefaultDescriptorRegistry()
	// Register a simple proto
	hello := `syntax = "proto3"; package example; message A {}`
	if err := registry.RegisterProtoFile("hello.proto", hello); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	// Successful FileByFilename
	reqSuccess := &reflectionv1.ServerReflectionRequest{
		MessageRequest: &reflectionv1.ServerReflectionRequest_FileByFilename{FileByFilename: "hello.proto"},
	}
	stream := &fakeStream{requests: []*reflectionv1.ServerReflectionRequest{reqSuccess}}
	if err := registry.ServerReflectionInfo(stream); err != nil {
		t.Fatalf("RPC error: %v", err)
	}
	resp := stream.responses[0]
	fdResp, ok := resp.MessageResponse.(*reflectionv1.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		t.Fatalf("expected FileDescriptorResponse, got %T", resp.MessageResponse)
	}
	// Unmarshal to check valid descriptorproto bytes
	b := fdResp.FileDescriptorResponse.GetFileDescriptorProto()[0]
	var fdp descriptorpb.FileDescriptorProto
	if err := proto.Unmarshal(b, &fdp); err != nil {
		t.Errorf("failed to unmarshal FileDescriptorProto: %v", err)
	}
	if fdp.GetName() != "hello.proto" {
		t.Errorf("unexpected descriptor name: %s", fdp.GetName())
	}

	// Not found case
	reqFail := &reflectionv1.ServerReflectionRequest{
		MessageRequest: &reflectionv1.ServerReflectionRequest_FileByFilename{FileByFilename: "nope.proto"},
	}
	stream2 := &fakeStream{requests: []*reflectionv1.ServerReflectionRequest{reqFail}}
	if err := registry.ServerReflectionInfo(stream2); err != nil {
		t.Fatalf("RPC error: %v", err)
	}
	r := stream2.responses[0]
	errResp, ok := r.MessageResponse.(*reflectionv1.ServerReflectionResponse_ErrorResponse)
	if !ok {
		t.Fatalf("expected ErrorResponse, got %T", r.MessageResponse)
	}
	if codes.Code(errResp.ErrorResponse.ErrorCode) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", errResp.ErrorResponse.ErrorCode)
	}
}

// Test FileContainingSymbol success
func TestServerReflection_FileContainingSymbol(t *testing.T) {
	registry := reflection.NewDefaultDescriptorRegistry()
	hello := `syntax = "proto3"; package ex;
service Svc { rpc M1(M1Req) returns (M1Req); }
message M1Req {}`
	registry.RegisterProtoFile("test.proto", hello)

	req := &reflectionv1.ServerReflectionRequest{
		MessageRequest: &reflectionv1.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: "ex.Svc"},
	}
	stream := &fakeStream{requests: []*reflectionv1.ServerReflectionRequest{req}}
	if err := registry.ServerReflectionInfo(stream); err != nil {
		t.Fatalf("RPC error: %v", err)
	}
	resp := stream.responses[0]
	fdResp, ok := resp.MessageResponse.(*reflectionv1.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		t.Fatalf("expected FileDescriptorResponse, got %T", resp.MessageResponse)
	}
	// Ensure returned descriptor contains the service name
	b := fdResp.FileDescriptorResponse.GetFileDescriptorProto()[0]
	var fdp descriptorpb.FileDescriptorProto
	_ = proto.Unmarshal(b, &fdp)
	found := false
	for _, svc := range fdp.GetService() {
		if svc.GetName() == "Svc" {
			found = true
		}
	}
	if !found {
		t.Errorf("service Svc not found in descriptor: %v", fdp.GetService())
	}
}
