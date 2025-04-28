package reflection

import (
	"context"
	"fmt"
	"log"
	"slices"
	"sync"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/linker"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// This service transforms raw .proto definitions into fully linked FileDescriptor objects
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
//   - IngestProtoFile(name, src)
//     Store raw .proto source in memory for deferred compilation.
//   - CompileAndRegister() error
//     Compile all previously ingested sources in one go and register their descriptors.
type DescriptorRegistry interface {
	// GetMessageDescriptor returns the MessageDescriptor for a given fully-qualified name
	GetMessageDescriptor(fullName string) (protoreflect.MessageDescriptor, bool)
	GetMethodDescriptor(fullName string) (protoreflect.MethodDescriptor, bool)

	GetFileDescriptors() []protoreflect.FileDescriptor

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
	messageDescriptorRegistry   map[string]protoreflect.MessageDescriptor
	messageDescriptorRegistryMu sync.RWMutex

	methodDescriptorRegistry   map[string]protoreflect.MethodDescriptor
	methodDescriptorRegistryMu sync.RWMutex
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

		s.messageDescriptorRegistryMu.Lock()
		if s.messageDescriptorRegistry == nil {
			s.messageDescriptorRegistry = map[string]protoreflect.MessageDescriptor{}
		}

		for i := range fd.Messages().Len() {
			md := fd.Messages().Get(i)
			if _, exists := s.messageDescriptorRegistry[string(md.FullName())]; !exists {
				s.messageDescriptorRegistry[string(md.FullName())] = md
				log.Printf("message descriptor registered : %s", md.FullName())
			}
		}
		s.messageDescriptorRegistryMu.Unlock()

		s.methodDescriptorRegistryMu.Lock()
		if s.methodDescriptorRegistry == nil {
			s.methodDescriptorRegistry = map[string]protoreflect.MethodDescriptor{}
		}

		for i := 0; i < fd.Services().Len(); i++ {
			svc := fd.Services().Get(i)

			for j := 0; j < svc.Methods().Len(); j++ {
				method := svc.Methods().Get(j)
				fullMethodName := fmt.Sprintf("/%s/%s", svc.FullName(), method.Name())
				s.methodDescriptorRegistry[fullMethodName] = method
				log.Printf("message descriptor registered: %s", fullMethodName)
			}
		}
		s.methodDescriptorRegistryMu.Unlock()
	}
}

func (s *defaultDescriptorRegistry) GetFileDescriptors() []protoreflect.FileDescriptor {
	s.allFileDescMu.RLock()
	defer s.allFileDescMu.RUnlock()
	descriptorsCopy := make([]protoreflect.FileDescriptor, len(s.allFileDescriptors))
	copy(descriptorsCopy, s.allFileDescriptors)
	return descriptorsCopy
}

// GetMessageDescriptor retrieves a message descriptor by full name
func (s *defaultDescriptorRegistry) GetMessageDescriptor(fullName string) (protoreflect.MessageDescriptor, bool) {
	s.messageDescriptorRegistryMu.RLock()
	defer s.messageDescriptorRegistryMu.RUnlock()
	md, ok := s.messageDescriptorRegistry[fullName]
	return md, ok
}

// GetMessageDescriptor retrieves a message descriptor by full name
func (s *defaultDescriptorRegistry) GetMethodDescriptor(fullName string) (protoreflect.MethodDescriptor, bool) {
	s.methodDescriptorRegistryMu.RLock()
	defer s.methodDescriptorRegistryMu.RUnlock()
	md, ok := s.methodDescriptorRegistry[fullName]
	return md, ok
}
