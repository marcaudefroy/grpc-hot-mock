package grpc

import (
	"encoding/json"
	"log"
	"time"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/proxy"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
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
	p *proxy.Proxy,
) grpc.StreamHandler {
	return func(srv any, stream grpc.ServerStream) error {
		fullMethod, _ := grpc.MethodFromServerStream(stream)
		log.Printf("gRPC call: %s", fullMethod)

		mc, hasMock := mockRegistry.GetMock(fullMethod)
		if hasMock {
			if mc.DelayMs > 0 {
				time.Sleep(time.Duration(mc.DelayMs) * time.Millisecond)
			}
			if len(mc.Headers) > 0 {
				if err := stream.SendHeader(metadata.New(mc.Headers)); err != nil {
					return err
				}
			}

			desc, ok := descriptorRegistry.GetMessageDescriptor(mc.ResponseType)
			if !ok {
				return status.Errorf(codes.Internal, "schema %q not found", mc.ResponseType)
			}

			if mc.GrpcStatus != 0 {
				return status.Errorf(codes.Code(mc.GrpcStatus), "%s", mc.ErrorString)
			}

			dyn := dynamicpb.NewMessage(desc)
			raw, _ := json.Marshal(mc.MockResponse)
			if err := protojson.Unmarshal(raw, dyn); err != nil {
				log.Printf("Mock JSON payload: %s", raw)
				return status.Errorf(codes.Internal, "json→message: %v", err)
			}

			return stream.SendMsg(dyn)
		}

		if p == nil {
			return status.Errorf(codes.Unimplemented, "no mock and no proxy")
		}
		return p.Handle(srv, stream)
	}
}
