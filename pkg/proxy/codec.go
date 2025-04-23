package proxy

import (
	"google.golang.org/grpc/encoding"
	_ "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
)

type MultiplexCodec struct{ Parent encoding.CodecV2 }

func NewDefaultMultiplexCodec() *MultiplexCodec {
	protoCodec := encoding.GetCodecV2("proto")
	multiplexCodecInstance := &MultiplexCodec{
		Parent: protoCodec,
	}
	return multiplexCodecInstance
}

func (c *MultiplexCodec) Marshal(v any) (mem.BufferSlice, error) {
	if data, ok := v.([]byte); ok {
		return mem.BufferSlice{mem.SliceBuffer(data)}, nil
	}
	return c.Parent.Marshal(v)
}

func (c *MultiplexCodec) Unmarshal(data mem.BufferSlice, v any) (err error) {
	if frame, ok := v.(*[]byte); ok {
		*frame = data.Materialize()
		return nil
	}

	return c.Parent.Unmarshal(data, v)
}

// Not really useful, since we enforce this codec on the server regardless of the header.
// But we need to implement this to satisfy the interface.
func (c MultiplexCodec) Name() string {
	return "multiplex"
}
