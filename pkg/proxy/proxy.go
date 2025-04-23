package proxy

import (
	"fmt"
	"io"

	_ "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/grpclog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Proxy forwards gRPC calls to an upstream backend server when no mock is configured.
// It handles both unary and bidirectional-stream RPCs, propagating metadata.
type Proxy struct {
	conn grpc.ClientConnInterface
}

// New creates a new Proxy to target, enforcing a raw codec and plaintext transport.
func New(target string, opts ...grpc.DialOption) (*Proxy, error) {
	opts = append(opts,
		grpc.WithDefaultCallOptions(grpc.ForceCodecV2(NewDefaultMultiplexCodec())),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}
	return &Proxy{conn: conn}, nil
}

// Handle inspects the first message to decide between unary or streaming proxying.
func (p *Proxy) Handle(_ interface{}, serverStream grpc.ServerStream) error {
	fullMethod, _ := grpc.MethodFromServerStream(serverStream)

	desc := &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}
	targetStream, err := p.conn.NewStream(serverStream.Context(), desc, fullMethod, grpc.WaitForReady(true))
	if err != nil {
		return fmt.Errorf("proxy new stream: %w", err)
	}

	if grpclog.V(2) {
		grpclog.Infof("[proxy] Send request to target")
	}

	errCh := make(chan error, 2)
	// Client -> Target
	go func() {
		for {
			var msg []byte
			if err := serverStream.RecvMsg(&msg); err != nil {
				if err != io.EOF {
					if grpclog.V(2) {
						grpclog.Infof("[proxy] Error while recv message from client %v", err)
					}
				} else {
					if grpclog.V(2) {
						grpclog.Infof("[proxy] EOF from client")
					}
				}
				errCh <- err
				return
			}
			if grpclog.V(2) {
				grpclog.Infof("[proxy] Message received from client, follow it to target")
			}
			err := targetStream.SendMsg(msg)
			if err != nil {
				if grpclog.V(2) {
					grpclog.Infof("[proxy] Error while sending message to target: %v", err)
				}
				errCh <- err
				return
			}
			if grpclog.V(2) {
				grpclog.Infof("[proxy] Message followed to target sucessfully")
			}

		}
	}()

	// Target -> client
	go func() {
		for {
			var msg []byte
			if err := targetStream.RecvMsg(&msg); err != nil {
				if err != io.EOF {
					if grpclog.V(2) {
						grpclog.Infof("[proxy] Error while recv message from target %v", err)
					}
				} else {
					if grpclog.V(2) {
						grpclog.Infof("[proxy] EOF from target")
					}
				}
				errCh <- err
				return
			}
			grpclog.Infof("[proxy] Message received from target, follow it to client")
			err := serverStream.SendMsg(msg)
			if err != nil {
				if grpclog.V(2) {
					grpclog.Infof("[proxy] Error while sending message to client: %v", err)
				}
				errCh <- err
				return
			}
			if grpclog.V(2) {
				grpclog.Infof("[proxy] Message followed to client sucessfully")
			}
		}
	}()

	// gRPC streams are full‑duplex: each side half‑closes its send stream when done, producing exactly one io.EOF per direction.
	// Sequence:
	//   • Client: N DATA frames (requests) → END_STREAM → io.EOF on client side
	//   • Server: N DATA frames (responses) → END_STREAM → io.EOF on server side
	firstErr := <-errCh
	if firstErr != nil && firstErr != io.EOF {
		return firstErr
	}
	<-errCh
	return nil
}
