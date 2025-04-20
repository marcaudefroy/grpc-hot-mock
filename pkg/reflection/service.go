package reflection

import (
	"context"
	"fmt"
	"io"
	"log"
	"slices"
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

// This service transforms raw .proto definitions into fully linked FileDescriptor objects
// and exposes them through the gRPC Reflection v1 API for dynamic schema discovery.
//
// Workflow:
//   1) Load .proto sources into memory.
//   2) Compile sources into FileDescriptors, resolving imports (including well-known types).
//   3) Register descriptors to serve reflection requests.

// Usage patterns:
//   • Quick load & register: ingest, compile, and register a single file in one call.
//   • Batch processing: ingest multiple files first, then compile & register them all together.
//   • Manual control: invoke load, compile, and register steps separately as needed.

// For only one file :
//   • RegisterProtoFile(name, src)
//     Ingest, compile immediately, and register resulting descriptors.

// Process multiple files :
//   • IngestProtoFile(name, src)
//     Store raw .proto source in memory for deferred compilation.
//   • CompileAndRegister() error
//     Compile all previously ingested sources in one go and register their descriptors.
//
// Use by calling:
//   reflectionv1.RegisterServerReflectionServer(grpcServer, registry)

type DescriptorRegistry interface {
	// ServerReflectionServer handles gRPC reflection requests
	reflectionv1.ServerReflectionServer

	// GetMessageDescriptor returns the MessageDescriptor for a given fully-qualified name
	GetMessageDescriptor(fullName string) (protoreflect.MessageDescriptor, bool)

	// RegisterProtoFile ingests and compiles a single .proto file, registering its descriptors
	RegisterProtoFile(filename, content string) error

	// IngestProtoFile stores the raw .proto content without compiling
	IngestProtoFile(filename, content string)

	// CompileAndRegister compiles all ingested proto files and registers their descriptors
	CompileAndRegister() error

	// Compile compiles all ingested proto files into linker.Files
	Compile() (linker.Files, error)

	// RegisterFiles adds the given FileDescriptors into the registry
	RegisterFiles(fds linker.Files)
}

type defaultDescriptorRegistry struct {
	// raw .proto sources keyed by filename
	protoFiles     map[string]string
	protoFileNames []string
	protoFilesMu   sync.RWMutex

	// all FileDescriptors available for reflection
	allFileDescriptors []protoreflect.FileDescriptor
	allFileDescMu      sync.RWMutex

	// mapping of message fullnames to their descriptors
	schemaRegistry   map[string]protoreflect.MessageDescriptor
	schemaRegistryMu sync.RWMutex
}

// NewDefaultDescriptorRegistry creates a registry preloaded with all standard Protobuf descriptors
func NewDefaultDescriptorRegistry() DescriptorRegistry {
	d := defaultDescriptorRegistry{}
	// Load built-in well-known types from the global registry
	protoregistry.GlobalFiles.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		d.allFileDescMu.Lock()
		d.allFileDescriptors = append(d.allFileDescriptors, fd)
		d.allFileDescMu.Unlock()
		return true
	})
	return &d
}

// IngestProtoFile stores the filename and content in memory without compiling
func (s *defaultDescriptorRegistry) IngestProtoFile(filename, content string) {
	s.protoFilesMu.Lock()
	defer s.protoFilesMu.Unlock()
	if s.protoFiles == nil {
		s.protoFiles = map[string]string{}
	}
	s.protoFiles[filename] = content

	if s.protoFileNames == nil {
		s.protoFileNames = []string{}
	}
	if !slices.Contains(s.protoFileNames, filename) {
		s.protoFileNames = append(s.protoFileNames, filename)
	}
}

// RegisterProtoFile ingests the file and immediately compiles and registers its descriptors
func (s *defaultDescriptorRegistry) RegisterProtoFile(filename, content string) error {
	s.IngestProtoFile(filename, content)
	fds, err := s.Compile()
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}
	s.RegisterFiles(fds)
	return nil
}

// CompileAndRegister compiles all ingested proto files and registers the resulting descriptors
func (s *defaultDescriptorRegistry) CompileAndRegister() error {
	fds, err := s.Compile()
	if err != nil {
		return fmt.Errorf("compile error: %w", err)
	}
	s.RegisterFiles(fds)
	return nil
}

// Compile transforms all ingested .proto sources into linked FileDescriptors
func (s *defaultDescriptorRegistry) Compile() (linker.Files, error) {
	base := &protocompile.SourceResolver{
		ImportPaths: []string{"."},
		Accessor:    protocompile.SourceAccessorFromMap(s.protoFiles),
	}
	resolver := protocompile.WithStandardImports(base)

	compiler := protocompile.Compiler{Resolver: resolver}
	return compiler.Compile(context.Background(), s.protoFileNames...)
}

// RegisterFiles adds new descriptors and extracts message schemas, skipping duplicates
func (s *defaultDescriptorRegistry) RegisterFiles(fds linker.Files) {
	s.allFileDescMu.Lock()
	defer s.allFileDescMu.Unlock()
	for _, fd := range fds {
		s.allFileDescriptors = append(s.allFileDescriptors, fd)
		s.schemaRegistryMu.Lock()
		if s.schemaRegistry == nil {
			s.schemaRegistry = map[string]protoreflect.MessageDescriptor{}
		}

		for i := range fd.Messages().Len() {
			md := fd.Messages().Get(i)
			if _, exists := s.schemaRegistry[string(md.FullName())]; !exists {
				s.schemaRegistry[string(md.FullName())] = md
				log.Printf("Registered schema: %s", md.FullName())
			}
		}
		s.schemaRegistryMu.Unlock()
	}
}

// GetMessageDescriptor retrieves a message descriptor by full name
func (s *defaultDescriptorRegistry) GetMessageDescriptor(fullName string) (protoreflect.MessageDescriptor, bool) {
	s.schemaRegistryMu.RLock()
	defer s.schemaRegistryMu.RUnlock()
	md, ok := s.schemaRegistry[fullName]
	return md, ok
}

// ServerReflectionInfo handles the bi-directional reflection stream, routing each request to helpers
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
			// unsupported reflection method
			if err := stream.Send(s.errorResponse(host, orig, codes.Unimplemented, "request type not supported")); err != nil {
				return err
			}
		}
	}
}

// buildListServicesResponse constructs a response listing all registered services
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
		MessageResponse: &reflectionv1.ServerReflectionResponse_ListServicesResponse{ListServicesResponse: svcResp},
	}
}

// buildFileByFilenameResponse finds and returns the FileDescriptorProto bytes for a given filename
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
		MessageResponse: &reflectionv1.ServerReflectionResponse_FileDescriptorResponse{FileDescriptorResponse: &reflectionv1.FileDescriptorResponse{FileDescriptorProto: [][]byte{fdpBytes}}},
	}
}

// buildFileContainingSymbolResponse returns the FileDescriptorProto bytes containing a given service or message symbol
func (s *defaultDescriptorRegistry) buildFileContainingSymbolResponse(host string, orig *reflectionv1.ServerReflectionRequest, symbol string) *reflectionv1.ServerReflectionResponse {
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

// errorResponse constructs a standard reflection error response with the given code and message
func (s *defaultDescriptorRegistry) errorResponse(host string, orig *reflectionv1.ServerReflectionRequest, code codes.Code, msg string) *reflectionv1.ServerReflectionResponse {
	return &reflectionv1.ServerReflectionResponse{
		ValidHost:       host,
		OriginalRequest: orig,
		MessageResponse: &reflectionv1.ServerReflectionResponse_ErrorResponse{ErrorResponse: &reflectionv1.ErrorResponse{ErrorCode: int32(code), ErrorMessage: msg}},
	}
}
