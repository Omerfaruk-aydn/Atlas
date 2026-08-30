package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchToolPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("merhaba dünya"))
	}))
	defer srv.Close()

	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	res, err := FetchTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if res.Content != "merhaba dünya" {
		t.Errorf("expected plain body, got %q", res.Content)
	}
}

func TestFetchToolStripsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><style>body{color:red}</style><script>alert(1)</script></head><body><h1>Başlık</h1><p>metin</p></body></html>`))
	}))
	defer srv.Close()

	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	res, err := FetchTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", res.Content)
	}
	if strings.Contains(res.Content, "alert(1)") {
		t.Errorf("expected script contents to be stripped, got %q", res.Content)
	}
	if strings.Contains(res.Content, "color:red") {
		t.Errorf("expected style contents to be stripped, got %q", res.Content)
	}
	if !strings.Contains(res.Content, "Başlık") || !strings.Contains(res.Content, "metin") {
		t.Errorf("expected visible text to survive stripping, got %q", res.Content)
	}
}

func TestFetchToolHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	res, err := FetchTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError for a 404 response")
	}
}

func TestFetchToolRejectsNonHTTPURL(t *testing.T) {
	input, _ := json.Marshal(map[string]string{"url": "ftp://example.com/file"})
	res, err := FetchTool{}.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Error("expected IsError for a non-http(s) URL")
	}
}
