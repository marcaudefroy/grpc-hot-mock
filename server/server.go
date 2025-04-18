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
	mux.HandleFunc("/mocks", s.handleAddMock)

	return mux
}
