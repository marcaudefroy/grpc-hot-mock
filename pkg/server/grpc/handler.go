package grpc

import (
	"encoding/json"
	"time"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/history"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/proxy"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Handler returns a grpc.StreamHandler that applies mock logic or proxies to a backend.
// It looks up a mock configuration by fullMethod, applies optional delay and headers,
// builds a dynamic response or returns a gRPC status error, and falls back to proxy if no mock.
func Handler(
	mockRegistry mocks.Registry,
	descriptorRegistry reflection.DescriptorRegistry,
	historyRegistry history.RegistryWriter,
	p *proxy.Proxy,
) grpc.StreamHandler {
	return func(srv any, stream grpc.ServerStream) error {
		fullMethod, _ := grpc.MethodFromServerStream(stream)

		if grpclog.V(2) {
			grpclog.Infof("[UnknownServiceHandler] method call gRPC received: %s", fullMethod)
		}

		methodDescriptor, ok := descriptorRegistry.GetMethodDescriptor(fullMethod)
		if !ok {
			if p == nil {
				return status.Errorf(codes.Unimplemented, "Method descriptor for %s doesn't exist on registry and proxy isn't enabled", fullMethod)
			}
		}

		mc, hasMock := mockRegistry.GetMock(fullMethod)
		if !hasMock {
			if p == nil {
				return status.Errorf(codes.Unimplemented, "No mock found for %s and proxy isn't enabled", fullMethod)
			}
			wrappedStream, ok := stream.(*wrappedServerStream)
			if ok {
				wrappedStream.proxified = true
			}

			if grpclog.V(2) {
				grpclog.Infof("[UnknownServiceHandler] No mock found, handle request by the proxy")
			}
			return p.Handle(srv, stream)
		}

		dynReq := dynamicpb.NewMessage(methodDescriptor.Input())
		if err := stream.RecvMsg(dynReq); err != nil {
			return status.Errorf(codes.Internal, "failed to receive message: %v", err)
		}

		if grpclog.V(2) {
			grpclog.Infof("[UnknownServiceHandler] Mock found")
		}
		if mc.DelayMs > 0 {
			time.Sleep(time.Duration(mc.DelayMs) * time.Millisecond)
		}

		if len(mc.Headers) > 0 {
			if err := stream.SendHeader(metadata.New(mc.Headers)); err != nil {
				return err
			}
		}

		if mc.GrpcStatus != 0 {
			return status.Errorf(codes.Code(mc.GrpcStatus), "%s", mc.ErrorString)
		}

		dyn := dynamicpb.NewMessage(methodDescriptor.Output())
		raw, _ := json.Marshal(mc.MockResponse)
		if err := protojson.Unmarshal(raw, dyn); err != nil {
			if grpclog.V(2) {
				grpclog.Infof("[UnknownServiceHandler] json→message: %v", err)
			}
			return status.Errorf(codes.Internal, "json→message: %v", err)
		}

		return stream.SendMsg(dyn)
	}
}
