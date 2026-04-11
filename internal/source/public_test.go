package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublicSourceFallsBack(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadGateway)
	}))
	defer bad.Close()

	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.8"))
	}))
	defer good.Close()

	src := NewPublic(PublicConfig{
		IPv4URLs: []string{bad.URL, good.URL},
		Client:   good.Client(),
	})
	got, err := src.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got.IPv4.String() != "203.0.113.8" {
		t.Fatalf("expected fallback result, got %s", got.IPv4)
	}
}
