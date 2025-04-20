package http

import (
	"net/http"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
)

// Server hosts HTTP endpoints for uploading .proto definitions and registering mocks.
type Server struct {
	mockRegistry       mocks.Registry
	descriptorRegistry reflection.DescriptorRegistry
}

// NewServer returns an http.ServeMux with all config routes registered.
func NewServer(dr reflection.DescriptorRegistry, mr mocks.Registry) *http.ServeMux {
	mux := http.NewServeMux()
	s := &Server{mockRegistry: mr, descriptorRegistry: dr}
	mux.HandleFunc("/upload_proto", s.handleUploadProto)
	mux.HandleFunc("/upload_protos", s.handleBulkUploadProtos)
	mux.HandleFunc("/injest", s.handleIngestProto)
	mux.HandleFunc("/compile", s.handleCompile)
	mux.HandleFunc("/mocks", s.handleAddMock)
	return mux
}
