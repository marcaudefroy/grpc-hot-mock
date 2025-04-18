package api

import (
	"encoding/json"
	"log"
	"time"

	"github.com/marcaudefroy/grpc-hot-mock/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/proxy"
	"github.com/marcaudefroy/grpc-hot-mock/reflection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

type HotServer struct {
	*grpc.Server
	mockRegistry       mocks.Registry
	descriptorRegistry reflection.DescriptorRegistry
}

func (s HotServer) GenericHandler(proxy *proxy.Proxy) grpc.StreamHandler {
	return func(srv interface{}, stream grpc.ServerStream) error {
		fullMethod, _ := grpc.MethodFromServerStream(stream)
		log.Printf("gRPC call: %s", fullMethod)

		mc, hasMock := s.mockRegistry.GetMock(fullMethod)
		if hasMock {
			if mc.DelayMs > 0 {
				time.Sleep(time.Duration(mc.DelayMs) * time.Millisecond)
			}
			if len(mc.Headers) > 0 {
				err := stream.SendHeader(metadata.New(mc.Headers))
				if err != nil {
					return err
				}
			}
			mdesc, ok := s.descriptorRegistry.GetMessageDescriptor(mc.ResponseType)
			if !ok {
				return status.Errorf(codes.Internal, "schema %q not found", mc.ResponseType)
			}
			if mc.GrpcStatus != 0 {
				return status.Errorf(codes.Code(mc.GrpcStatus), "%s", mc.ErrorString)
			}
			dyn := dynamicpb.NewMessage(mdesc)
			data, _ := json.Marshal(mc.MockResponse)
			if err := protojson.Unmarshal(data, dyn); err != nil {
				log.Printf("Mock JSON payload: %s", data)
				return status.Errorf(codes.Internal, "json→message: %v", err)
			}

			return stream.SendMsg(dyn)
		}
		if proxy == nil {
			return status.Errorf(codes.Unimplemented, "no mock and no proxy")
		}

		return proxy.Handle(srv, stream)
	}
}

func NewServer(proxyAddr string, descriptorRegistry reflection.DescriptorRegistry, mockRegistry mocks.Registry) *HotServer {
	s := &HotServer{
		mockRegistry:       mockRegistry,
		descriptorRegistry: descriptorRegistry,
	}

	var p *proxy.Proxy
	if proxyAddr != "" {
		p = proxy.NewProxy(proxyAddr)
	}

	server := grpc.NewServer(
		grpc.UnknownServiceHandler(s.GenericHandler(p)),
	)

	// Activate reflection server
	reflectionv1.RegisterServerReflectionServer(server, descriptorRegistry)

	s.Server = server
	return s
}
