package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marcaudefroy/grpc-hot-mock/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/reflection"
	"github.com/marcaudefroy/grpc-hot-mock/server"
)

// Test the /upload_proto endpoint with valid and invalid payloads
func TestHandleUploadProto(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	mux := server.NewServer(dr, mr)

	// Successful upload
	payload := map[string]string{"filename": "test.proto", "content": "syntax = \"proto3\"; package p; message M{}"}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/upload_proto", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rec.Code)
	}
	if rec.Body.String() != "proto test.proto uploaded, descriptors registered" {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}

	// Invalid JSON
	req = httptest.NewRequest(http.MethodPost, "/upload_proto", bytes.NewReader([]byte(`{invalid`)))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid JSON, got %d", rec.Code)
	}

	// Missing fields
	payload = map[string]string{"filename": "", "content": ""}
	body, _ = json.Marshal(payload)
	req = httptest.NewRequest(http.MethodPost, "/upload_proto", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for missing fields, got %d", rec.Code)
	}
}

// Test the /injest and /compile endpoints
func TestIngestAndCompileEndpoints(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	mux := server.NewServer(dr, mr)

	// Ingest multiple protos
	bulk := map[string]interface{}{"files": []map[string]string{
		{"filename": "a.proto", "content": "syntax = \"proto3\"; package x; message A{}"},
		{"filename": "b.proto", "content": "syntax = \"proto3\"; package x; message B{}"},
	}}
	body, _ := json.Marshal(bulk)
	req := httptest.NewRequest(http.MethodPost, "/injest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202 Accepted, got %d", rec.Code)
	}

	// Compile ingested protos
	req = httptest.NewRequest(http.MethodPost, "/compile", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	// Now reflection registry should have descriptors
	if _, ok := dr.GetMessageDescriptor("x.A"); !ok {
		t.Error("descriptor for x.A not found after compile")
	}
	if _, ok := dr.GetMessageDescriptor("x.B"); !ok {
		t.Error("descriptor for x.B not found after compile")
	}
}

// Test handleBulkUploadProtos via /upload_protos
func TestHandleBulkUploadProtos(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	mux := server.NewServer(dr, mr)

	bulk := map[string]interface{}{"files": []map[string]string{
		{"filename": "c.proto", "content": "syntax = \"proto3\"; package y; message C{}"},
		{"filename": "d.proto", "content": "syntax = \"proto3\"; package y; message D{}"},
	}}
	body, _ := json.Marshal(bulk)
	req := httptest.NewRequest(http.MethodPost, "/upload_protos", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rec.Code)
	}

	// Descriptors should be immediately available
	if _, ok := dr.GetMessageDescriptor("y.C"); !ok {
		t.Error("descriptor for y.C not found after bulk upload")
	}
	if _, ok := dr.GetMessageDescriptor("y.D"); !ok {
		t.Error("descriptor for y.D not found after bulk upload")
	}
}

// Test handleAddMock endpoint
func TestHandleAddMock(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	mux := server.NewServer(dr, mr)

	// Register mock
	mock := map[string]interface{}{
		"service":      "svc",
		"method":       "M",
		"responseType": "T",
	}
	body, _ := json.Marshal(mock)
	req := httptest.NewRequest(http.MethodPost, "/mocks", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rec.Code)
	}

	// Check registry
	if _, exists := mr.GetMock("/svc/M"); !exists {
		t.Error("mock not registered")
	}
}
