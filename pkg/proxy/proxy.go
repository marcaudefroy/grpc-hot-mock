package proxy

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Proxy forwards gRPC calls to an upstream backend server when no mock is configured.
// It implements a bidirectional stream proxy, copying messages between client and backend.
// Metadata from the incoming context is propagated to the backend.
// Proxy implements unary-only gRPC forwarding for calls without mocks.
type Proxy struct {
	conn grpc.ClientConnInterface
}

// New creates a Proxy connected to the given backend address.
// It dials the backend with insecure transport by default; adjust for TLS if needed.
func New(target string) *Proxy {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic("failed to dial backend: " + err.Error())
	}
	return &Proxy{conn: conn}
}

// Handle proxies unary gRPC calls when no mock is configured.
// It receives a single request message, invokes the backend via unary RPC, and returns its response.
func (p *Proxy) Handle(srv any, serverStream grpc.ServerStream) error {
	// Determine full method
	fullMethod, _ := grpc.MethodFromServerStream(serverStream)

	// Receive single request
	var reqMsg any
	if err := serverStream.RecvMsg(&reqMsg); err != nil {
		return status.Errorf(codes.Internal, "proxy receive: %v", err)
	}

	// Propagate metadata
	md, _ := metadata.FromIncomingContext(serverStream.Context())
	ctx := metadata.NewOutgoingContext(serverStream.Context(), md)

	// Invoke unary call on backend
	var respMsg any
	err := p.conn.Invoke(ctx, fullMethod, reqMsg, &respMsg)
	if err != nil {
		return status.Errorf(codes.Internal, "proxy invoke: %v", err)
	}

	// Send response back to client
	return serverStream.SendMsg(respMsg)
}
