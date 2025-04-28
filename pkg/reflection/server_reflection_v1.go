package reflection

import (
	"io"

	"google.golang.org/grpc/codes"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type FileDescriptorsGetter interface {
	GetFileDescriptors() []protoreflect.FileDescriptor
}

type ServerReflectionV1 struct {
	// ServerReflectionServer handles gRPC reflection requests
	reflectionv1.ServerReflectionServer
	fdg FileDescriptorsGetter
}

func NewServerReflectionV1(fdg FileDescriptorsGetter) *ServerReflectionV1 {
	return &ServerReflectionV1{fdg: fdg}
}

// ServerReflectionInfo handles the bi-directional reflection stream, routing each request to helpers
func (s *ServerReflectionV1) ServerReflectionInfo(
	stream reflectionv1.ServerReflection_ServerReflectionInfoServer,
) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}

		host := req.GetHost()
		orig := req

		switch r := req.GetMessageRequest().(type) {
		case *reflectionv1.ServerReflectionRequest_ListServices:
			resp := s.buildListServicesResponse(host, orig)
			if err := stream.Send(resp); err != nil {
				return err
			}

		case *reflectionv1.ServerReflectionRequest_FileByFilename:
			resp := s.buildFileByFilenameResponse(host, orig, r.FileByFilename)
			if err := stream.Send(resp); err != nil {
				return err
			}

		case *reflectionv1.ServerReflectionRequest_FileContainingSymbol:
			resp := s.buildFileContainingSymbolResponse(host, orig, r.FileContainingSymbol)
			if err := stream.Send(resp); err != nil {
				return err
			}

		default:
			// unsupported reflection method
			if err := stream.Send(s.errorResponse(host, orig, codes.Unimplemented, "request type not supported")); err != nil {
				return err
			}
		}
	}
}

// buildListServicesResponse constructs a response listing all registered services
func (s *ServerReflectionV1) buildListServicesResponse(host string, orig *reflectionv1.ServerReflectionRequest) *reflectionv1.ServerReflectionResponse {
	seen := map[string]struct{}{}
	svcResp := &reflectionv1.ListServiceResponse{}

	for _, fd := range s.fdg.GetFileDescriptors() {
		for i := 0; i < fd.Services().Len(); i++ {
			name := string(fd.Services().Get(i).FullName())
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				svcResp.Service = append(svcResp.Service, &reflectionv1.ServiceResponse{Name: name})
			}
		}
	}

	return &reflectionv1.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1.ServerReflectionResponse_ListServicesResponse{ListServicesResponse: svcResp},
	}
}

// buildFileByFilenameResponse finds and returns the FileDescriptorProto bytes for a given filename
func (s *ServerReflectionV1) buildFileByFilenameResponse(host string, orig *reflectionv1.ServerReflectionRequest, filename string) *reflectionv1.ServerReflectionResponse {
	fdpBytes, found := s.lookupFileDescriptorProtoBytes(func(fd protoreflect.FileDescriptor) bool {
		return fd.Path() == filename
	})

	if !found {
		return s.errorResponse(host, orig, codes.NotFound, "file not found")
	}
	return &reflectionv1.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1.ServerReflectionResponse_FileDescriptorResponse{FileDescriptorResponse: &reflectionv1.FileDescriptorResponse{FileDescriptorProto: [][]byte{fdpBytes}}},
	}
}

// buildFileContainingSymbolResponse returns the FileDescriptorProto bytes containing a given service or message symbol
func (s *ServerReflectionV1) buildFileContainingSymbolResponse(host string, orig *reflectionv1.ServerReflectionRequest, symbol string) *reflectionv1.ServerReflectionResponse {
	fdpBytes, found := s.lookupFileDescriptorProtoBytes(func(fd protoreflect.FileDescriptor) bool {
		// search services
		for i := range fd.Services().Len() {
			if string(fd.Services().Get(i).FullName()) == symbol {
				return true
			}
		}
		// search messages
		for i := range fd.Messages().Len() {
			if string(fd.Messages().Get(i).FullName()) == symbol {
				return true
			}
		}
		return false
	})

	if !found {
		return s.errorResponse(host, orig, codes.NotFound, "symbol not found")
	}
	return &reflectionv1.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1.ServerReflectionResponse_FileDescriptorResponse{FileDescriptorResponse: &reflectionv1.FileDescriptorResponse{FileDescriptorProto: [][]byte{fdpBytes}}},
	}
}

// lookupFileDescriptorProtoBytes searches allFileDescriptors using match and returns the marshaled FileDescriptorProto bytes
func (s *ServerReflectionV1) lookupFileDescriptorProtoBytes(match func(protoreflect.FileDescriptor) bool) ([]byte, bool) {
	for _, fd := range s.fdg.GetFileDescriptors() {
		if match(fd) {
			fdp := protodesc.ToFileDescriptorProto(fd)
			b, _ := proto.Marshal(fdp)
			return b, true
		}
	}
	return nil, false
}

// errorResponse constructs a standard reflection error response with the given code and message
func (s *ServerReflectionV1) errorResponse(host string, orig *reflectionv1.ServerReflectionRequest, code codes.Code, msg string) *reflectionv1.ServerReflectionResponse {
	return &reflectionv1.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1.ServerReflectionResponse_ErrorResponse{ErrorResponse: &reflectionv1.ErrorResponse{ErrorCode: int32(code), ErrorMessage: msg}},
	}
}
