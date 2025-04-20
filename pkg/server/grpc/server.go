package grpc

import (
	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/proxy"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	"google.golang.org/grpc"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

// NewServer creates a grpc.Server with:
//   - the Reflection service registered from descriptorRegistry
//   - an UnknownServiceHandler using the mock/proxy Handler
func NewServer(
	proxyAddr string,
	descriptorRegistry reflection.DescriptorRegistry,
	mockRegistry mocks.Registry,
) *grpc.Server {
	var p *proxy.Proxy
	if proxyAddr != "" {
		p = proxy.New(proxyAddr)
	}

	srv := grpc.NewServer(
		grpc.UnknownServiceHandler(Handler(mockRegistry, descriptorRegistry, p)),
	)
	reflectionv1.RegisterServerReflectionServer(srv, descriptorRegistry)
	return srv
}
