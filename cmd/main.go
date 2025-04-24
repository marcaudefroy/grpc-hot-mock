package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/history"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/server/grpc"
	hotServer "github.com/marcaudefroy/grpc-hot-mock/pkg/server/http"
)

var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	grpcPort := flag.String("grpc_port", ":50051", "gRPC listen address")
	httpPort := flag.String("http_port", ":8080", "HTTP config address")
	proxyAddr := flag.String("proxy", "", "Optional gRPC proxy backend address")

	if *showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	if proxyAddr == nil || *proxyAddr == "" {
		if env := os.Getenv("PROXY_TARGET"); env != "" {
			proxyAddr = &env
		}
	}

	descriptorRegistry := reflection.NewDefaultDescriptorRegistry()
	mockRegistry := &mocks.DefaultRegistry{}
	historyRegistry := &history.DefaultRegistry{}

	httpServer := hotServer.NewServer(descriptorRegistry, mockRegistry, historyRegistry)
	go func() {
		log.Printf("HTTP config server on %s", *httpPort)
		log.Fatal(http.ListenAndServe(*httpPort, httpServer))
	}()

	server := grpc.NewServer(*proxyAddr, descriptorRegistry, mockRegistry, historyRegistry)
	lis, err := net.Listen("tcp", *grpcPort)
	if err != nil {
		log.Fatalf("listen %s: %v", *grpcPort, err)
	}
	log.Printf("gRPC listening on %s (proxy=%q)", *grpcPort, *proxyAddr)
	err = server.Serve(lis)
	if err != nil {
		log.Fatalf("Unable to run grpc server %v", err)
	}
}
