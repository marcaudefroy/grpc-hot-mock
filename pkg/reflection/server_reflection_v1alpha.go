package reflection

import (
	"io"

	"google.golang.org/grpc/codes"
	reflectionv1alpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type ServerReflectionV1Alpha struct {
	reflectionv1alpha.ServerReflectionServer
	fdg FileDescriptorsGetter
}

func NewServerReflectionV1Alpha(fdg FileDescriptorsGetter) *ServerReflectionV1Alpha {
	return &ServerReflectionV1Alpha{
		fdg: fdg,
	}
}

func (s *ServerReflectionV1Alpha) ServerReflectionInfo(
	stream reflectionv1alpha.ServerReflection_ServerReflectionInfoServer,
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
		case *reflectionv1alpha.ServerReflectionRequest_ListServices:
			resp := s.buildListServicesResponse(host, orig)
			if err := stream.Send(resp); err != nil {
				return err
			}

		case *reflectionv1alpha.ServerReflectionRequest_FileByFilename:
			resp := s.buildFileByFilenameResponse(host, orig, r.FileByFilename)
			if err := stream.Send(resp); err != nil {
				return err
			}

		case *reflectionv1alpha.ServerReflectionRequest_FileContainingSymbol:
			resp := s.buildFileContainingSymbolResponse(host, orig, r.FileContainingSymbol)
			if err := stream.Send(resp); err != nil {
				return err
			}

		default:
			if err := stream.Send(s.errorResponse(host, orig, codes.Unimplemented, "request type not supported")); err != nil {
				return err
			}
		}
	}
}

func (s *ServerReflectionV1Alpha) buildListServicesResponse(host string, orig *reflectionv1alpha.ServerReflectionRequest) *reflectionv1alpha.ServerReflectionResponse {
	seen := map[string]struct{}{}
	svcResp := &reflectionv1alpha.ListServiceResponse{}

	for _, fd := range s.fdg.GetFileDescriptors() {
		for i := 0; i < fd.Services().Len(); i++ {
			name := string(fd.Services().Get(i).FullName())
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				svcResp.Service = append(svcResp.Service, &reflectionv1alpha.ServiceResponse{Name: name})
			}
		}
	}

	return &reflectionv1alpha.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1alpha.ServerReflectionResponse_ListServicesResponse{ListServicesResponse: svcResp},
	}
}

func (s *ServerReflectionV1Alpha) buildFileByFilenameResponse(host string, orig *reflectionv1alpha.ServerReflectionRequest, filename string) *reflectionv1alpha.ServerReflectionResponse {
	fdpBytes, found := s.lookupFileDescriptorProtoBytes(func(fd protoreflect.FileDescriptor) bool {
		return fd.Path() == filename
	})

	if !found {
		return s.errorResponse(host, orig, codes.NotFound, "file not found")
	}
	return &reflectionv1alpha.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1alpha.ServerReflectionResponse_FileDescriptorResponse{FileDescriptorResponse: &reflectionv1alpha.FileDescriptorResponse{FileDescriptorProto: [][]byte{fdpBytes}}},
	}
}

func (s *ServerReflectionV1Alpha) buildFileContainingSymbolResponse(host string, orig *reflectionv1alpha.ServerReflectionRequest, symbol string) *reflectionv1alpha.ServerReflectionResponse {
	fdpBytes, found := s.lookupFileDescriptorProtoBytes(func(fd protoreflect.FileDescriptor) bool {
		for i := range fd.Services().Len() {
			if string(fd.Services().Get(i).FullName()) == symbol {
				return true
			}
		}
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
	return &reflectionv1alpha.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1alpha.ServerReflectionResponse_FileDescriptorResponse{FileDescriptorResponse: &reflectionv1alpha.FileDescriptorResponse{FileDescriptorProto: [][]byte{fdpBytes}}},
	}
}

func (s *ServerReflectionV1Alpha) lookupFileDescriptorProtoBytes(match func(protoreflect.FileDescriptor) bool) ([]byte, bool) {
	for _, fd := range s.fdg.GetFileDescriptors() {
		if match(fd) {
			fdp := protodesc.ToFileDescriptorProto(fd)
			b, _ := proto.Marshal(fdp)
			return b, true
		}
	}
	return nil, false
}

func (s *ServerReflectionV1Alpha) errorResponse(host string, orig *reflectionv1alpha.ServerReflectionRequest, code codes.Code, msg string) *reflectionv1alpha.ServerReflectionResponse {
	return &reflectionv1alpha.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1alpha.ServerReflectionResponse_ErrorResponse{ErrorResponse: &reflectionv1alpha.ErrorResponse{ErrorCode: int32(code), ErrorMessage: msg}},
	}
}
