package grpc

import (
	"log"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/history"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/proxy"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	reflectionv1alpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// NewServer creates a grpc.Server with:
//   - the Reflection service registered from descriptorRegistry
//   - an UnknownServiceHandler using the mock/proxy Handler
func NewServer(
	proxyAddr string,
	descriptorRegistry reflection.DescriptorRegistry,
	mockRegistry mocks.Registry,
	historyRegistry history.RegistryWriter,
) *grpc.Server {
	var p *proxy.Proxy
	if proxyAddr != "" {
		var err error
		p, err = proxy.New(proxyAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Printf("Unable to initiate proxy : %v", err)
		}
	}

	srv := grpc.NewServer(
		grpc.UnknownServiceHandler(Handler(mockRegistry, descriptorRegistry, historyRegistry, p)),
		grpc.ForceServerCodecV2(proxy.NewDefaultMultiplexCodec()),
		grpc.StreamInterceptor(StreamInterceptor(historyRegistry)),
	)
	serverReflectionV1 := reflection.NewServerReflectionV1(descriptorRegistry)
	serverReflectionV1alpha := reflection.NewServerReflectionV1Alpha(descriptorRegistry)

	reflectionv1.RegisterServerReflectionServer(srv, serverReflectionV1)

	// DEPRECATED but still used by some client on production
	reflectionv1alpha.RegisterServerReflectionServer(srv, serverReflectionV1alpha)
	return srv
}
