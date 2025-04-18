package reflection

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/grpc/codes"
	reflectionv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type DescriptorRegistry interface {
	reflectionv1.ServerReflectionServer
	GetMessageDescriptor(fullName string) (protoreflect.MessageDescriptor, bool)
	RegisterFiles(fds linker.Files)
	RegisterProtoFile(filename, content string) error
}

type defaultDescriptorRegistry struct {
	protoFiles   map[string]string
	protoFilesMu sync.RWMutex

	allFileDescriptors []protoreflect.FileDescriptor
	allFileDescMu      sync.RWMutex

	schemaRegistry   map[string]protoreflect.MessageDescriptor
	schemaRegistryMu sync.RWMutex
}

func NewDefaultDescriptorRegistry() DescriptorRegistry {
	d := defaultDescriptorRegistry{}
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		d.allFileDescMu.Lock()
		d.allFileDescriptors = append(d.allFileDescriptors, fd)
		d.allFileDescMu.Unlock()
		return true
	})
	return &d
}

func (s *defaultDescriptorRegistry) compileProto(filename, content string) (linker.Files, error) {
	s.protoFilesMu.Lock()
	if s.protoFiles == nil {
		s.protoFiles = map[string]string{}
	}
	s.protoFiles[filename] = content
	s.protoFilesMu.Unlock()

	base := &protocompile.SourceResolver{
		ImportPaths: []string{"."},
		Accessor:    protocompile.SourceAccessorFromMap(s.protoFiles),
	}

	resolver := protocompile.WithStandardImports(base)

	compiler := protocompile.Compiler{Resolver: resolver}
	return compiler.Compile(context.Background(), filename)
}

func (s *defaultDescriptorRegistry) RegisterProtoFile(filename, content string) error {
	fds, err := s.compileProto(filename, content)
	if err != nil {
		return fmt.Errorf("compile error : %w", err)
	}
	s.RegisterFiles(fds)
	return nil
}

func (s *defaultDescriptorRegistry) GetMessageDescriptor(fullName string) (protoreflect.MessageDescriptor, bool) {
	s.schemaRegistryMu.RLock()
	defer s.schemaRegistryMu.RUnlock()
	md, ok := s.schemaRegistry[fullName]
	return md, ok
}

func (s *defaultDescriptorRegistry) RegisterFiles(fds linker.Files) {
	s.allFileDescMu.Lock()
	defer s.allFileDescMu.Unlock()
	for _, fd := range fds {
		if s.allFileDescriptors == nil {
			s.allFileDescriptors = []protoreflect.FileDescriptor{}
		}
		s.allFileDescriptors = append(s.allFileDescriptors, fd)
		s.schemaRegistryMu.Lock()
		if s.schemaRegistry == nil {
			s.schemaRegistry = map[string]protoreflect.MessageDescriptor{}
		}
		for i := 0; i < fd.Messages().Len(); i++ {
			md := fd.Messages().Get(i)
			s.schemaRegistry[string(md.FullName())] = md
			log.Printf("Registered schema: %s", md.FullName())
		}
		s.schemaRegistryMu.Unlock()
	}
}

func (s *defaultDescriptorRegistry) ServerReflectionInfo(
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
			if err := stream.Send(s.errorResponse(host, orig, codes.Unimplemented, "request type not supported")); err != nil {
				return err
			}
		}
	}
}

func (s *defaultDescriptorRegistry) buildListServicesResponse(host string, orig *reflectionv1.ServerReflectionRequest) *reflectionv1.ServerReflectionResponse {
	seen := map[string]struct{}{}
	svcResp := &reflectionv1.ListServiceResponse{}

	s.allFileDescMu.RLock()
	defer s.allFileDescMu.RUnlock()
	for _, fd := range s.allFileDescriptors {
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
		MessageResponse: &reflectionv1.ServerReflectionResponse_ListServicesResponse{
			ListServicesResponse: svcResp,
		},
	}
}

func (s *defaultDescriptorRegistry) buildFileByFilenameResponse(host string, orig *reflectionv1.ServerReflectionRequest, filename string) *reflectionv1.ServerReflectionResponse {
	fdpBytes, found := s.lookupFileDescriptorProtoBytes(func(fd protoreflect.FileDescriptor) bool {
		return fd.Path() == filename
	})

	if !found {
		return s.errorResponse(host, orig, codes.NotFound, "file not found")
	}
	return &reflectionv1.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1.ServerReflectionResponse_FileDescriptorResponse{
			FileDescriptorResponse: &reflectionv1.FileDescriptorResponse{
				FileDescriptorProto: [][]byte{fdpBytes},
			},
		},
	}
}

func (s *defaultDescriptorRegistry) buildFileContainingSymbolResponse(host string, orig *reflectionv1.ServerReflectionRequest, symbol string) *reflectionv1.ServerReflectionResponse {
	fdpBytes, found := s.lookupFileDescriptorProtoBytes(func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			if string(fd.Services().Get(i).FullName()) == symbol {
				return true
			}
		}
		for i := 0; i < fd.Messages().Len(); i++ {
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
		MessageResponse: &reflectionv1.ServerReflectionResponse_FileDescriptorResponse{
			FileDescriptorResponse: &reflectionv1.FileDescriptorResponse{
				FileDescriptorProto: [][]byte{fdpBytes},
			},
		},
	}
}

func (s *defaultDescriptorRegistry) lookupFileDescriptorProtoBytes(match func(protoreflect.FileDescriptor) bool) ([]byte, bool) {
	s.allFileDescMu.RLock()
	defer s.allFileDescMu.RUnlock()
	for _, fd := range s.allFileDescriptors {
		if match(fd) {
			fdp := protodesc.ToFileDescriptorProto(fd)
			b, _ := proto.Marshal(fdp)
			return b, true
		}
	}
	return nil, false
}

func (s *defaultDescriptorRegistry) errorResponse(host string, orig *reflectionv1.ServerReflectionRequest, code codes.Code, msg string) *reflectionv1.ServerReflectionResponse {
	return &reflectionv1.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1.ServerReflectionResponse_ErrorResponse{
			ErrorResponse: &reflectionv1.ErrorResponse{
				ErrorCode:    int32(code),
				ErrorMessage: msg,
			},
		},
	}
}
