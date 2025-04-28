package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marcaudefroy/grpc-hot-mock/pkg/history"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/mocks"
	"github.com/marcaudefroy/grpc-hot-mock/pkg/reflection"
	httpServer "github.com/marcaudefroy/grpc-hot-mock/pkg/server/http"
)

func TestHandleRegisterProtoJSON(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	hr := &history.DefaultRegistry{}
	mux := httpServer.NewServer(dr, mr, hr)

	// Successful registration
	payload := map[string]any{"files": []map[string]string{
		{"filename": "test.proto", "content": "syntax = \"proto3\"; package p; message M{}"},
	}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/protos/register/json", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created, got %d", rec.Code)
	}
	assertNoErrorInBody(t, rec.Body)

	// Invalid JSON
	req = httptest.NewRequest(http.MethodPost, "/protos/register/json", bytes.NewReader([]byte(`{invalid`)))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for invalid JSON, got %d", rec.Code)
	}

	// Missing fields
	payload = map[string]any{"files": []map[string]string{
		{"filename": "", "content": ""},
	}}
	body, _ = json.Marshal(payload)
	req = httptest.NewRequest(http.MethodPost, "/protos/register/json", bytes.NewReader(body))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for missing fields, got %d", rec.Code)
	}
}

func TestHandleIngestAndCompile(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	hr := &history.DefaultRegistry{}
	mux := httpServer.NewServer(dr, mr, hr)

	// Ingest only
	payload := map[string]any{"files": []map[string]string{
		{"filename": "a.proto", "content": "syntax = \"proto3\"; package x; message A{}"},
		{"filename": "b.proto", "content": "syntax = \"proto3\"; package x; message B{}"},
	}}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/protos/ingest/json", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for ingestion, got %d", rec.Code)
	}
	assertNoErrorInBody(t, rec.Body)

	// Compile ingested
	req = httptest.NewRequest(http.MethodPost, "/protos/ingest/compile", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK for compilation, got %d", rec.Code)
	}
	assertNoErrorInBody(t, rec.Body)

	// Verify descriptors
	if _, ok := dr.GetMessageDescriptor("x.A"); !ok {
		t.Error("descriptor for x.A not found after compile")
	}
	if _, ok := dr.GetMessageDescriptor("x.B"); !ok {
		t.Error("descriptor for x.B not found after compile")
	}
}

func TestHandleAddMock(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	hr := &history.DefaultRegistry{}
	mux := httpServer.NewServer(dr, mr, hr)

	mock := map[string]any{
		"service":      "svc",
		"method":       "M",
		"responseType": "T",
	}
	body, _ := json.Marshal(mock)
	req := httptest.NewRequest(http.MethodPost, "/mocks", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected 201 Created for mock registration, got %d", rec.Code)
	}
	assertNoErrorInBody(t, rec.Body)

	if _, exists := mr.GetMock("/svc/M"); !exists {
		t.Error("mock not properly registered")
	}
}

func TestHandleHistoryAndClear(t *testing.T) {
	dr := reflection.NewDefaultDescriptorRegistry()
	mr := &mocks.DefaultRegistry{}
	hr := &history.DefaultRegistry{}
	mux := httpServer.NewServer(dr, mr, hr)

	// Fetch history (should be empty initially)
	req := httptest.NewRequest(http.MethodGet, "/history", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK on /history, got %d", rec.Code)
	}

	// Clear history
	req = httptest.NewRequest(http.MethodPost, "/history/clear", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK on /history/clear, got %d", rec.Code)
	}
	assertNoErrorInBody(t, rec.Body)
}

// Helper
func assertNoErrorInBody(t *testing.T, body *bytes.Buffer) {
	var resp map[string]any
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Errorf("failed to decode JSON body: %v", err)
	}
	if errStr, ok := resp["error"]; ok {
		t.Errorf("unexpected error in response: %v", errStr)
	}
}
