package api_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/marcaudefroy/grpc-hot-mock/api"
	"github.com/marcaudefroy/grpc-hot-mock/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/proxy"
	"github.com/marcaudefroy/grpc-hot-mock/reflection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

// fakeServerStream implements grpc.ServerStream for testing GenericHandler
type fakeServerStream struct {
	method  string
	header  metadata.MD
	trailer metadata.MD
	msgs    []any
}

func newFakeServerStream(method string) *fakeServerStream {
	return &fakeServerStream{method: method}
}

func (f *fakeServerStream) Context() context.Context {
	return grpc.NewContextWithServerTransportStream(
		context.Background(),
		&fakeTransport{method: f.method},
	)
}
func (f *fakeServerStream) SetHeader(md metadata.MD) error  { f.header = md; return nil }
func (f *fakeServerStream) SendHeader(md metadata.MD) error { f.header = md; return nil }
func (f *fakeServerStream) SetTrailer(md metadata.MD)       { f.trailer = md }
func (f *fakeServerStream) SendMsg(m any) error             { f.msgs = append(f.msgs, m); return nil }
func (f *fakeServerStream) RecvMsg(m any) error             { return io.EOF }

func TestGenericHandler_NoMock_NoProxy(t *testing.T) {
	// No mock, nil proxy
	srv := api.NewServer("", reflection.NewDefaultDescriptorRegistry(), &mocks.DefaultRegistry{})
	handler := srv.GenericHandler(nil)
	stream := &fakeServerStream{}
	err := handler(srv, stream)
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("expected Unimplemented, got %v", st.Code())
	}
}

type fakeTransport struct {
	method  string
	header  metadata.MD
	trailer metadata.MD
}

func (f *fakeTransport) Method() string {
	return f.method
}

func (f *fakeTransport) SetHeader(md metadata.MD) error {
	f.header = md
	return nil
}

func (f *fakeTransport) SendHeader(md metadata.MD) error {
	f.header = md
	return nil
}

func (f *fakeTransport) SetTrailer(md metadata.MD) error {
	f.trailer = md
	return nil
}

func TestGenericHandler_MockResponse(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()

	// Register proto schema for HelloReply
	proto := `syntax = "proto3"; package example;
message HelloRequest { string name = 1; }
message HelloReply { string message = 1; }`
	if err := dr.RegisterProtoFile("hello.proto", proto); err != nil {
		t.Fatalf("failed to register proto: %v", err)
	}

	// Prepare mock config
	mc := mocks.MockConfig{
		Service:      "example.Greeter",
		Method:       "SayHello",
		ResponseType: "example.HelloReply",
		MockResponse: map[string]interface{}{"message": "hi"},
		GrpcStatus:   0,
		Headers:      map[string]string{"h": "v"},
		DelayMs:      50,
	}

	mr := &mocks.DefaultRegistry{}
	mr.RegisterMock(mc)
	srv := api.NewServer("", dr, mr)
	handler := srv.GenericHandler(proxy.NewProxy(""))

	stream := newFakeServerStream("/example.Greeter/SayHello")

	err := handler(srv, stream)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// Check headers
	if stream.header.Get("h")[0] != "v" {
		t.Errorf("expected header h=v, got %v", stream.header)
	}
	// Check message payload
	if len(stream.msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(stream.msgs))
	}
	dyn, ok := stream.msgs[0].(*dynamicpb.Message)
	if !ok {
		t.Fatalf("expected dynamicpb.Message, got %T", stream.msgs[0])
	}

	out, err := protojson.Marshal(dyn)
	if err != nil {
		t.Fatalf("protojson.Marshal failed: %v", err)
	}
	var obj map[string]string
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if obj["message"] != "hi" {
		t.Errorf("expected message=hi, got %v", obj)
	}
}

func TestGenericHandler_GrpcStatus(t *testing.T) {
	// Register schema
	dr := reflection.NewDefaultDescriptorRegistry()
	proto := `syntax = "proto3"; package example;
message Foo {}`
	dr.RegisterProtoFile("foo.proto", proto)

	mc := mocks.MockConfig{Service: "example.Greeter", Method: "SayHello", ResponseType: "example.Foo", GrpcStatus: int(codes.PermissionDenied), ErrorString: "Error example"}
	mr := &mocks.DefaultRegistry{}
	mr.RegisterMock(mc)

	srv := api.NewServer("", dr, mr)
	handler := srv.GenericHandler(proxy.NewProxy(""))

	stream := newFakeServerStream("/example.Greeter/SayHello")

	err := handler(srv, stream)
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
	if st.Message() != "Error example" {
		t.Errorf("expected Error example, got %v", st.Message())
	}
}

func TestGenericHandler_WellKnownTimestampType(t *testing.T) {
	// Initialize registry with well-known types
	dr := reflection.NewDefaultDescriptorRegistry()

	// Register ts.proto schema that imports google.protobuf.Timestamp
	tsProto := `syntax = "proto3";
package example;

import "google/protobuf/timestamp.proto";

message Event {
  string id = 1;
  google.protobuf.Timestamp occurred_at = 2;
}

message EventRequest { string id = 1; }

service EventService { rpc GetEvent(EventRequest) returns (Event); }`
	if err := dr.RegisterProtoFile("ts.proto", tsProto); err != nil {
		t.Fatalf("failed to register ts.proto: %v", err)
	}

	// Prepare mock with RFC3339 timestamp string
	mockTime := "2021-07-01T12:00:00Z"
	mc := mocks.MockConfig{
		Service:      "example.EventService",
		Method:       "GetEvent",
		ResponseType: "example.Event",
		MockResponse: map[string]interface{}{
			"id":         "evt-123",
			"occurredAt": mockTime,
		},
		GrpcStatus: 0,
		Headers:    map[string]string{},
		DelayMs:    0,
	}
	mr := &mocks.DefaultRegistry{}
	mr.RegisterMock(mc)

	// Invoke handler
	srv := api.NewServer("", dr, mr)
	handler := srv.GenericHandler(proxy.NewProxy(""))
	stream := newFakeServerStream("/example.EventService/GetEvent")

	err := handler(srv, stream)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	// Validate response
	if len(stream.msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(stream.msgs))
	}
	dyn, ok := stream.msgs[0].(*dynamicpb.Message)
	if !ok {
		t.Fatalf("expected dynamicpb.Message, got %T", stream.msgs[0])
	}

	// Marshal to JSON and check fields
	out, err := protojson.Marshal(dyn)
	if err != nil {
		t.Fatalf("protojson.Marshal failed: %v", err)
	}
	var obj struct {
		ID         string `json:"id"`
		OccurredAt string `json:"occurredAt"`
	}
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if obj.ID != "evt-123" {
		t.Errorf("expected id=evt-123, got %v", obj.ID)
	}
	if obj.OccurredAt != mockTime {
		t.Errorf("expected occurredAt=%v, got %v", mockTime, obj.OccurredAt)
	}
}
