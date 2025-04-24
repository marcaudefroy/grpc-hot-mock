package http

import (
	"net/http"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/history"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
)

// Server hosts HTTP endpoints for uploading .proto definitions and registering mocks.
type Server struct {
	mockRegistry       mocks.Registry
	descriptorRegistry reflection.DescriptorRegistry
	historyRegistry    history.RegistryReader
}

// NewServer returns an http.ServeMux with all config routes registered.
func NewServer(dr reflection.DescriptorRegistry, mr mocks.Registry, hr history.RegistryReader) *http.ServeMux {
	mux := http.NewServeMux()
	s := &Server{mockRegistry: mr, descriptorRegistry: dr, historyRegistry: hr}
	mux.HandleFunc("/upload-proto", s.handleUploadProto)
	mux.HandleFunc("/upload-protos", s.handleBulkUploadProtos)
	mux.HandleFunc("/injest", s.handleIngestProto)
	mux.HandleFunc("/compile", s.handleCompile)
	mux.HandleFunc("/mocks", s.handleAddMock)
	mux.HandleFunc("/history", s.handleHistory)
	return mux
}
