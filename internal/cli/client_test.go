package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites" {
			t.Errorf("got path %q, want /api/v1/sites", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("got method %q, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResponse{OK: true, Data: json.RawMessage(`[]`)})
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	resp, err := client.Get("/api/v1/sites")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !resp.OK {
		t.Error("expected OK response")
	}
}

func TestClientPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("got method %q, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected JSON content type")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResponse{OK: true})
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	_, err := client.Post("/api/v1/sites", map[string]string{"domain": "test.com"})
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
}

func TestClientWithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("got auth %q, want Bearer test-key", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(apiResponse{OK: true})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-key")
	_, err := client.Get("/api/v1/test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
}

func TestClientErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(apiResponse{OK: false, Error: "not found"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	_, err := client.Get("/api/v1/nope")
	if err == nil {
		t.Fatal("expected error for not-found response")
	}
}
