package http

import (
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"mime/multipart"
	"net/http"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
)

type BulkUploadRequest struct {
	Files []ProtoJSON `json:"files"`
}
type ProtoJSON struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// handleUploadProtoJSON ingests and compiles multiple .proto files (into a json payload) in one call.
func (s *Server) handleUploadProtoJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req BulkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if len(req.Files) == 0 {
		writeError(w, http.StatusBadRequest, "no proto files provided")
		return
	}

	// Phase 1: ingest all
	for _, f := range req.Files {
		if f.Filename == "" || f.Content == "" {
			writeError(w, http.StatusBadRequest, "filename and content required for all files")
			return
		}
		s.descriptorRegistry.IngestProtoFile(f.Filename, f.Content)
	}
	// Phase 2: compile and register all
	if err := s.descriptorRegistry.CompileAndRegister(); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to compile files: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, nil)
}

// handleIngestProto ingests multiple .proto sources without compilation.
func (s *Server) handleIngestProtoJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req BulkUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Files) == 0 {
		writeError(w, http.StatusBadRequest, "no proto files provided")
		return
	}
	for _, f := range req.Files {
		if f.Filename == "" || f.Content == "" {
			writeError(w, http.StatusBadRequest, "filename and content required for all files")
			return
		}
		s.descriptorRegistry.IngestProtoFile(f.Filename, f.Content)
	}
	writeJSON(w, http.StatusCreated, nil)
}

// uploadProtoRequest is the payload for /upload-proto

func (s *Server) handleUploadProtoFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	statusCode, err := s.injestProtoFileFromRequest(r)
	if err != nil {
		writeError(w, statusCode, err.Error())
		return
	}
	// Phase 2: compile and register all
	if err := s.descriptorRegistry.CompileAndRegister(); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to compile files: %v", err))
		return
	}

	writeJSON(w, http.StatusCreated, nil)
}

func (s *Server) handleIngestProtoFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	statusCode, err := s.injestProtoFileFromRequest(r)
	if err != nil {
		writeError(w, statusCode, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, nil)
}

func (s *Server) injestProtoFileFromRequest(r *http.Request) (int, error) {
	err := r.ParseMultipartForm(64 << 20) // 64MB max
	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("error parsing multipart form: %w", err)
	}

	form := r.MultipartForm
	files := form.File["files"]
	if len(files) == 0 {
		return http.StatusBadRequest, fmt.Errorf("no files uploaded: %w", err)
	}

	for _, header := range files {
		content, fullPath, err := readFileFromFileHeader(header)
		if err != nil {
			return http.StatusInternalServerError, fmt.Errorf("error reading file: %w", err)
		}

		s.descriptorRegistry.IngestProtoFile(fullPath, string(content))
	}
	return http.StatusAccepted, nil
}

func readFileFromFileHeader(header *multipart.FileHeader) ([]byte, string, error) {
	file, err := header.Open()
	if err != nil {
		return nil, "", fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()
	// Extract the full filename (including path) from the Content-Disposition header.
	// By default, Go's multipart.FileHeader.Filename returns only the base name of the file,
	// as it applies filepath.Base to the filename parameter for security reasons.
	// In the context of gRPC, .proto files often contain import statements with relative paths.
	// To correctly resolve these imports during processing, we need the original full path
	// provided by the client (if any). Therefore, we manually parse the Content-Disposition
	// header using mime.ParseMediaType to retrieve the full filename.
	fullPath, err := extractFullFilename(header)
	if err != nil {
		return nil, "", fmt.Errorf("error extracting full filename: %w", err)
	}

	content := make([]byte, header.Size)
	_, err = file.Read(content)
	if err != nil {
		return nil, fullPath, fmt.Errorf("error reading file: %w", err)
	}
	return content, fullPath, nil
}

func extractFullFilename(header *multipart.FileHeader) (string, error) {
	contentDisposition := header.Header.Get("Content-Disposition")
	_, params, err := mime.ParseMediaType(contentDisposition)
	if err != nil {
		return "", err
	}
	filename, ok := params["filename"]
	if !ok {
		return header.Filename, nil
	}
	return filename, nil
}

// handleCompile compiles and registers all previously ingested .proto sources.
func (s *Server) handleCompile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := s.descriptorRegistry.CompileAndRegister(); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("failed to compile files: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

// handleAddMock registers a new mock configuration.
func (s *Server) handleAddMock(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var mc mocks.MockConfig
	if err := json.NewDecoder(r.Body).Decode(&mc); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if mc.Service == "" || mc.Method == "" {
		writeError(w, http.StatusMethodNotAllowed, "service and method are required")
		return
	}
	s.mockRegistry.RegisterMock(mc)
	writeJSON(w, http.StatusCreated, nil)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	history := s.historyRegistry.GetHistories()
	writeJSON(w, http.StatusOK, history)
}

func (s *Server) clearHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	s.historyRegistry.Clear()
	writeJSON(w, http.StatusOK, map[string]string{"message": "history cleared"})
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		log.Printf("warning: write response failed: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
