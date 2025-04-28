package grpc

import (
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/history"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type wrappedServerStream struct {
	grpc.ServerStream
	streamServerInfo *grpc.StreamServerInfo
	historyRegistry  history.RegistryWriter
	history          *history.History
	proxified        bool
	methodDescriptor protoreflect.MethodDescriptor
}

func StreamInterceptor(historyRegistry history.RegistryWriter, descriptorRegistry reflection.DescriptorRegistry) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		h := history.History{
			ID:         uuid.NewString(),
			StartTime:  time.Now(),
			FullMethod: info.FullMethod,
			Messages:   []history.Message{},
			State:      history.StateOpen,
		}
		historyRegistry.SaveHistory(h)

		wrappedStream := &wrappedServerStream{
			ServerStream:     ss,
			historyRegistry:  historyRegistry,
			streamServerInfo: info,
			history:          &h,
		}
		if method, ok := descriptorRegistry.GetMethodDescriptor(info.FullMethod); ok {
			wrappedStream.methodDescriptor = method
		}

		err := handler(srv, wrappedStream)
		endTime := time.Now()
		h.EndTime = &endTime
		h.State = history.StateClosed
		if s, ok := status.FromError(err); ok {
			h.GrpcCode = int32(s.Code())
			h.GrpcMessage = s.Message()
		} else {
			h.GrpcCode = int32(codes.Unknown)
			h.GrpcMessage = err.Error()
		}
		wrappedStream.historyRegistry.SaveHistory(h)

		return err
	}
}

func (w *wrappedServerStream) SendMsg(m any) error {
	w.recordMessage("send", m)
	return w.ServerStream.SendMsg(m)
}

func (w *wrappedServerStream) RecvMsg(m any) error {
	err := w.ServerStream.RecvMsg(m)
	if err == nil {
		w.recordMessage("recv", m)
	}
	return err
}

func (w *wrappedServerStream) recordMessage(direction string, payload any) {
	var (
		payloadStr string
		payloadObj any
		recognized bool
	)

	switch m := payload.(type) {
	case proto.Message:
		b, err := protojson.Marshal(m)
		if err != nil {
			payloadStr = "<invalid proto>"
			payloadObj = nil
			recognized = false
			break
		}
		payloadStr = string(b)
		recognized = true

		if _, ok := m.(*dynamicpb.Message); ok {
			var tmp map[string]any
			if err := json.Unmarshal(b, &tmp); err == nil {
				payloadObj = tmp
			} else {
				payloadObj = nil
			}
		} else {
			payloadObj = m
		}
	case []byte:
		if w.methodDescriptor != nil {
			var dyn *dynamicpb.Message
			if direction == "recv" {
				dyn = dynamicpb.NewMessage(w.methodDescriptor.Input())
			} else {
				dyn = dynamicpb.NewMessage(w.methodDescriptor.Output())
			}
			if err := proto.Unmarshal(m, dyn); err == nil {
				b, err := protojson.Marshal(dyn)
				if err == nil {
					payloadStr = string(b)

					var tmp map[string]interface{}
					if err := json.Unmarshal(b, &tmp); err == nil {
						payloadObj = tmp
					} else {
						payloadObj = nil
					}

					recognized = true
					break
				}
			}
		}

		payloadStr = encodeBase64(m)
		payloadObj = nil
		recognized = false
	default:
		if b, err := json.Marshal(m); err == nil {
			payloadStr = string(b)
			payloadObj = m
			recognized = true
		} else {
			payloadStr = "<invalid json>"
			recognized = false
		}
	}

	w.history.Messages = append(w.history.Messages, history.Message{
		Direction:     direction,
		Timestamp:     time.Now(),
		Recognized:    recognized,
		Proxified:     w.proxified,
		PayloadString: payloadStr,
		Payload:       payloadObj,
	})
}

func encodeBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func generateID() string {
	return uuid.New().String()
}
