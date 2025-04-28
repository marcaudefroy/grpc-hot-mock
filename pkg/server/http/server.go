package http

import (
	"log"
	"net/http"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/history"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
)

// Server hosts HTTP endpoints for uploading .proto definitions and registering mocks.
type Server struct {
	mockRegistry       mocks.Registry
	descriptorRegistry reflection.DescriptorRegistry
	historyRegistry    history.RegisterReadWriter
}

func logRequest(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[HTTP] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		handler(w, r)
	}
}

// NewServer returns an http.ServeMux with all config routes registered.
func NewServer(dr reflection.DescriptorRegistry, mr mocks.Registry, hr history.RegisterReadWriter) *http.ServeMux {
	mux := http.NewServeMux()
	s := &Server{mockRegistry: mr, descriptorRegistry: dr, historyRegistry: hr}

	mux.HandleFunc("/protos/register/json", logRequest(s.handleUploadProtoJSON))
	mux.HandleFunc("/protos/register/file", logRequest(s.handleUploadProtoFile))

	mux.HandleFunc("/protos/ingest/json", logRequest(s.handleIngestProtoJSON))
	mux.HandleFunc("/protos/ingest/file", logRequest(s.handleIngestProtoFile))
	mux.HandleFunc("/protos/ingest/compile", logRequest(s.handleCompile))

	mux.HandleFunc("/mocks", logRequest(s.handleAddMock))

	mux.HandleFunc("/history", logRequest(s.handleHistory))
	mux.HandleFunc("/history/clear", logRequest(s.clearHistory))
	return mux
}
