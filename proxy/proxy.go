package proxy

import (
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/dynamicpb"
)

type Proxy struct {
	proxyAddr string
}

func NewProxy(proxyAddr string) *Proxy {
	return &Proxy{
		proxyAddr: proxyAddr,
	}
}

func (p Proxy) Handle(srv interface{}, stream grpc.ServerStream) error {
	inMD, _ := metadata.FromIncomingContext(stream.Context())
	fullMethod, _ := grpc.MethodFromServerStream(stream)

	log.Printf("Proxying %s → %s", fullMethod, p.proxyAddr)
	var reqMsg dynamicpb.Message
	if err := stream.RecvMsg(&reqMsg); err != nil {
		return status.Errorf(codes.Internal, "recv: %v", err)
	}
	conn, err := grpc.NewClient(p.proxyAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return status.Errorf(codes.Internal, "dial proxy: %v", err)
	}
	defer conn.Close()
	outCtx := metadata.NewOutgoingContext(stream.Context(), inMD)
	var respMsg dynamicpb.Message
	if err := conn.Invoke(outCtx, fullMethod, &reqMsg, &respMsg); err != nil {
		return status.Errorf(codes.Internal, "proxy call: %v", err)
	}
	return stream.SendMsg(&respMsg)
}
