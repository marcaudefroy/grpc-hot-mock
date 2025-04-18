package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/marcaudefroy/grpc-hot-mock/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/reflection"
)

type uploadProtoRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

func (s HTTPServer) handleUploadProto(w http.ResponseWriter, r *http.Request) {
	var req uploadProtoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Filename == "" || req.Content == "" {
		http.Error(w, "filename and content required", http.StatusBadRequest)
		return
	}
	err := s.descriptorRegistry.RegisterProtoFile(req.Filename, req.Content)
	if err != nil {
		http.Error(w, "compile error: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "proto %s uploaded, descriptors registered", req.Filename)
}

// BulkUploadRequest est le JSON que l’on attend sur /upload_protos
type BulkUploadRequest struct {
	Files []struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	} `json:"files"`
}

// handleBulkUploadProtos permet d’uploader plusieurs .proto en une seule requête
func (s HTTPServer) handleBulkUploadProtos(w http.ResponseWriter, r *http.Request) {
	var req BulkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Files) == 0 {
		http.Error(w, "no proto files provided", http.StatusBadRequest)
		return
	}

	for _, f := range req.Files {
		if f.Filename == "" || f.Content == "" {
			http.Error(w, "filename and content required for all files", http.StatusBadRequest)
			return
		}
		s.descriptorRegistry.IngestProtoFile(f.Filename, f.Content)
	}

	if err := s.descriptorRegistry.CompileAndRegister(); err != nil {
		http.Error(w, fmt.Sprintf("failed to compile files: %v", err), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "%d proto files ingested and registered", len(req.Files))
}

func (s HTTPServer) injestProto(w http.ResponseWriter, r *http.Request) {
	var req BulkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Files) == 0 {
		http.Error(w, "no proto files provided", http.StatusBadRequest)
		return
	}

	for _, f := range req.Files {
		if f.Filename == "" || f.Content == "" {
			http.Error(w, "filename and content required for all files", http.StatusBadRequest)
			return
		}
		s.descriptorRegistry.IngestProtoFile(f.Filename, f.Content)
	}

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "%d proto files ingested", len(req.Files))
}

func (s HTTPServer) compile(w http.ResponseWriter, r *http.Request) {
	if err := s.descriptorRegistry.CompileAndRegister(); err != nil {
		http.Error(w, fmt.Sprintf("failed to compile files: %v", err), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "proto files compiles")
}

func (s HTTPServer) handleAddMock(w http.ResponseWriter, r *http.Request) {
	var mc mocks.MockConfig
	if err := json.NewDecoder(r.Body).Decode(&mc); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if mc.Service == "" || mc.Method == "" || mc.ResponseType == "" {
		http.Error(w, "service, method and responseType required", http.StatusBadRequest)
		return
	}
	s.mockRegistry.RegisterMock(mc)
	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "mock registered for %s/%s", mc.Service, mc.Method)
}

type HTTPServer struct {
	mockRegistry       mocks.Registry
	descriptorRegistry reflection.DescriptorRegistry
}

func NewServer(descriptorRegistry reflection.DescriptorRegistry, mockRegistry mocks.Registry) *http.ServeMux {
	mux := http.NewServeMux()
	s := &HTTPServer{
		mockRegistry:       mockRegistry,
		descriptorRegistry: descriptorRegistry,
	}
	mux.HandleFunc("/upload_proto", s.handleUploadProto)
	mux.HandleFunc("/upload_protos", s.handleBulkUploadProtos)
	mux.HandleFunc("/injest", s.injestProto)
	mux.HandleFunc("/compile", s.compile)
	mux.HandleFunc("/mocks", s.handleAddMock)

	return mux
}
