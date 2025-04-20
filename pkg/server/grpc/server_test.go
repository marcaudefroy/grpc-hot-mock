package grpc_test

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/proxy"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	grpcServer "github.com/marcaudefroy/grpc-hot-mock/pkg/server/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

// fakeServerStream implémente grpc.ServerStream pour tester Handler
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

type fakeTransport struct {
	method  string
	header  metadata.MD
	trailer metadata.MD
}

func (f *fakeTransport) Method() string                  { return f.method }
func (f *fakeTransport) SetHeader(md metadata.MD) error  { f.header = md; return nil }
func (f *fakeTransport) SendHeader(md metadata.MD) error { f.header = md; return nil }
func (f *fakeTransport) SetTrailer(md metadata.MD) error { f.trailer = md; return nil }

func TestHandler_NoMock_NoProxy(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	handler := grpcServer.Handler(mr, dr, nil)

	stream := newFakeServerStream("/svc/Method")
	err := handler(nil, stream)
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("expected Unimplemented, got %v", st.Code())
	}
}

func TestHandler_MockResponse(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	hello := `syntax = "proto3"; package example;
message HelloRequest { string name = 1; }
message HelloReply   { string message = 1; }`
	if err := dr.RegisterProtoFile("hello.proto", hello); err != nil {
		t.Fatalf("register proto failed: %v", err)
	}

	mc := mocks.MockConfig{
		Service:      "example.Greeter",
		Method:       "SayHello",
		ResponseType: "example.HelloReply",
		MockResponse: map[string]any{"message": "hi"},
		GrpcStatus:   0,
		Headers:      map[string]string{"h": "v"},
		DelayMs:      0,
	}
	mr := &mocks.DefaultRegistry{}
	mr.RegisterMock(mc)

	handler := grpcServer.Handler(mr, dr, proxy.New(""))
	stream := newFakeServerStream("/example.Greeter/SayHello")
	if err := handler(nil, stream); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// headers
	if got := stream.header.Get("h"); len(got) != 1 || got[0] != "v" {
		t.Errorf("expected header h=v, got %v", stream.header)
	}
	// payload
	if len(stream.msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(stream.msgs))
	}
	dyn, ok := stream.msgs[0].(*dynamicpb.Message)
	if !ok {
		t.Fatalf("expected dynamicpb.Message, got %T", stream.msgs[0])
	}
	out, err := protojson.Marshal(dyn)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var obj map[string]string
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if obj["message"] != "hi" {
		t.Errorf("expected message=hi, got %v", obj)
	}
}

func TestHandler_GrpcStatusError(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	foo := `syntax = "proto3"; package example; message Foo {}`
	if err := dr.RegisterProtoFile("foo.proto", foo); err != nil {
		t.Fatalf("register proto failed: %v", err)
	}

	mc := mocks.MockConfig{
		Service:      "example.Greeter",
		Method:       "SayHello",
		ResponseType: "example.Foo",
		GrpcStatus:   int(codes.PermissionDenied),
		ErrorString:  "Error example",
	}
	mr := &mocks.DefaultRegistry{}
	mr.RegisterMock(mc)

	handler := grpcServer.Handler(mr, dr, proxy.New(""))
	stream := newFakeServerStream("/example.Greeter/SayHello")
	err := handler(nil, stream)
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
	if st.Message() != "Error example" {
		t.Errorf("expected message Error example, got %v", st.Message())
	}
}

func TestHandler_WellKnownTimestamp(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	ts := `syntax = "proto3"; package example;
import "google/protobuf/timestamp.proto";
message Event { string id = 1; google.protobuf.Timestamp occurredAt = 2; }
message EventRequest { string id = 1; }
service EventService { rpc GetEvent(EventRequest) returns (Event); }`
	if err := dr.RegisterProtoFile("ts.proto", ts); err != nil {
		t.Fatalf("register ts.proto failed: %v", err)
	}

	mockTime := "2021-07-01T12:00:00Z"
	mc := mocks.MockConfig{
		Service:      "example.EventService",
		Method:       "GetEvent",
		ResponseType: "example.Event",
		MockResponse: map[string]any{
			"id":         "evt-123",
			"occurredAt": mockTime,
		},
	}
	mr := &mocks.DefaultRegistry{}
	mr.RegisterMock(mc)

	handler := grpcServer.Handler(mr, dr, proxy.New(""))
	stream := newFakeServerStream("/example.EventService/GetEvent")
	if err := handler(nil, stream); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if len(stream.msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(stream.msgs))
	}
	dyn := stream.msgs[0].(*dynamicpb.Message)
	out, _ := protojson.Marshal(dyn)
	var obj struct {
		ID         string `json:"id"`
		OccurredAt string `json:"occurredAt"`
	}
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if obj.ID != "evt-123" || obj.OccurredAt != mockTime {
		t.Errorf("unexpected %+v", obj)
	}
}
