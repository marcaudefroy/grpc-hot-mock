package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/server/grpc"
	hotServer "github.com/marcaudefroy/grpc-hot-mock/pkg/server/http"
)

var (
	protoFiles   = map[string]string{}
	protoFilesMu sync.RWMutex
	proxyAddr    string
)

func main() {
	grpcPort := flag.String("grpc_port", ":50051", "gRPC listen address")
	httpPort := flag.String("http_port", ":8080", "HTTP config address")
	flag.StringVar(&proxyAddr, "proxy", "", "gRPC proxy address (empty to disable)")
	flag.Parse()

	descriptorRegistry := reflection.NewDefaultDescriptorRegistry()
	mockRegistry := &mocks.DefaultRegistry{}
	httpServer := hotServer.NewServer(descriptorRegistry, mockRegistry)
	go func() {
		log.Printf("HTTP config server on %s", *httpPort)
		log.Fatal(http.ListenAndServe(*httpPort, httpServer))
	}()

	server := grpc.NewServer(proxyAddr, descriptorRegistry, mockRegistry)
	lis, err := net.Listen("tcp", *grpcPort)
	if err != nil {
		log.Fatalf("listen %s: %v", *grpcPort, err)
	}
	log.Printf("gRPC listening on %s (proxy=%q)", *grpcPort, proxyAddr)
	err = server.Serve(lis)
	if err != nil {
		log.Fatalf("Unable to run grpc server %v", err)
	}
}
