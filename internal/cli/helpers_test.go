package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSplitKeyValue(t *testing.T) {
	cases := []struct {
		in         string
		key, value string
	}{
		{"KEY=value", "KEY", "value"},
		{"APP_DEBUG=false", "APP_DEBUG", "false"},
		{"URL=postgres://u:p@h/db?x=1", "URL", "postgres://u:p@h/db?x=1"}, // only first '='
		{"EMPTY=", "EMPTY", ""},
	}
	for _, c := range cases {
		k, v, err := splitKeyValue(c.in)
		if err != nil {
			t.Errorf("splitKeyValue(%q): %v", c.in, err)
			continue
		}
		if k != c.key || v != c.value {
			t.Errorf("splitKeyValue(%q) = (%q,%q), want (%q,%q)", c.in, k, v, c.key, c.value)
		}
	}

	if _, _, err := splitKeyValue("NOEQUALS"); err == nil {
		t.Error("expected error for input without '='")
	}
}

func TestClientDelete(t *testing.T) {
	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	if _, err := client.Delete("/api/v1/sites/x"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("got method %q, want DELETE", gotMethod)
	}
}

func TestClientDelete2SendsBody(t *testing.T) {
	var gotMethod, gotCT string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	if _, err := client.Delete2("/api/v1/tokens", map[string]int{"id": 1}); err != nil {
		t.Fatalf("Delete2: %v", err)
	}
	if gotMethod != "DELETE" {
		t.Errorf("got method %q, want DELETE", gotMethod)
	}
	if gotCT != "application/json" {
		t.Errorf("body request should set JSON content type, got %q", gotCT)
	}
}

func TestClientDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json at all"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "")
	if _, err := client.Get("/api/v1/x"); err == nil {
		t.Fatal("expected decode error for non-JSON response")
	}
}

func TestClientNetworkError(t *testing.T) {
	// Point at a closed port: the request must fail with a clear message.
	client := NewClient("http://127.0.0.1:1", "")
	if _, err := client.Get("/api/v1/x"); err == nil {
		t.Fatal("expected network error")
	}
}

func TestNewClientSocketMode(t *testing.T) {
	// With no remote, the client targets the unix control socket.
	c := NewClient("", "")
	if c.socketPath != defaultSocketPath {
		t.Errorf("socketPath = %q, want %q", c.socketPath, defaultSocketPath)
	}
	if c.baseURL != "http://apod" {
		t.Errorf("baseURL = %q, want http://apod", c.baseURL)
	}
}
