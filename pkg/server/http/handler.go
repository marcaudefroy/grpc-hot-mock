package http

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
)

// uploadProtoRequest is the payload for /upload-proto
type uploadProtoRequest struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// handleUploadProto ingests, compiles, and registers a single .proto file.
func (s *Server) handleUploadProto(w http.ResponseWriter, r *http.Request) {
	var req uploadProtoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Filename == "" || req.Content == "" {
		http.Error(w, "filename and content required", http.StatusBadRequest)
		return
	}
	if err := s.descriptorRegistry.RegisterProtoFile(req.Filename, req.Content); err != nil {
		http.Error(w, "compile error: "+err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	if _, err := fmt.Fprintf(w, "proto %s uploaded, descriptors registered", req.Filename); err != nil {
		log.Printf("warning: write response failed: %v", err)
	}
}

// BulkUploadRequest is the payload for /upload-protos and /injest
type BulkUploadRequest struct {
	Files []struct {
		Filename string `json:"filename"`
		Content  string `json:"content"`
	} `json:"files"`
}

// handleBulkUploadProtos ingests and compiles multiple .proto files in one call.
func (s *Server) handleBulkUploadProtos(w http.ResponseWriter, r *http.Request) {
	var req BulkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Files) == 0 {
		http.Error(w, "no proto files provided", http.StatusBadRequest)
		return
	}
	// Phase 1: ingest all
	for _, f := range req.Files {
		if f.Filename == "" || f.Content == "" {
			http.Error(w, "filename and content required for all files", http.StatusBadRequest)
			return
		}
		s.descriptorRegistry.IngestProtoFile(f.Filename, f.Content)
	}
	// Phase 2: compile and register all
	if err := s.descriptorRegistry.CompileAndRegister(); err != nil {
		http.Error(w, fmt.Sprintf("failed to compile files: %v", err), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
	if _, err := fmt.Fprintf(w, "%d proto files ingested and registered", len(req.Files)); err != nil {
		log.Printf("warning: write response failed: %v", err)
	}
}

// handleIngestProto ingests multiple .proto sources without compilation.
func (s *Server) handleIngestProto(w http.ResponseWriter, r *http.Request) {
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
	if _, err := fmt.Fprintf(w, "%d proto files ingested", len(req.Files)); err != nil {
		log.Printf("warning: write response failed: %v", err)
	}
}

// handleCompile compiles and registers all previously ingested .proto sources.
func (s *Server) handleCompile(w http.ResponseWriter, r *http.Request) {
	if err := s.descriptorRegistry.CompileAndRegister(); err != nil {
		http.Error(w, fmt.Sprintf("failed to compile files: %v", err), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, "proto files compiled and registered"); err != nil {
		log.Printf("warning: write response failed: %v", err)
	}
}

// handleAddMock registers a new mock configuration.
func (s *Server) handleAddMock(w http.ResponseWriter, r *http.Request) {
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
	if _, err := fmt.Fprintf(w, "mock registered for %s/%s", mc.Service, mc.Method); err != nil {
		log.Printf("warning: write response failed: %v", err)
	}
}
