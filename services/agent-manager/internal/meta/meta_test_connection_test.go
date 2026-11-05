package meta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeProvider rejects any key other than "good-key" with 401, and only
// offers model "good-model".
func fakeProvider(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-key" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			return
		}
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"good-model"}]}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
}

func TestTestConnectionValidKeyAndModel(t *testing.T) {
	srv := fakeProvider(t)
	defer srv.Close()
	c := NewClient(Config{APIKey: "good-key", BaseURL: srv.URL, Model: "good-model"})
	if err := c.TestConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTestConnectionFakeKeyFails(t *testing.T) {
	srv := fakeProvider(t)
	defer srv.Close()
	c := NewClient(Config{APIKey: "fake-key", BaseURL: srv.URL, Model: "good-model"})
	err := c.TestConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rejected the API key") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "fake-key") {
		t.Fatal("error leaked the API key")
	}
}

func TestTestConnectionMissingKeyFails(t *testing.T) {
	c := NewClient(Config{BaseURL: "https://example.com/v1", Model: "m"})
	// NewClient preserves values but stays disabled without a key; construct
	// the raw state TestConnection must reject.
	c.config.APIKey = ""
	if err := c.TestConnection(context.Background()); err == nil || !strings.Contains(err.Error(), "missing API key") {
		t.Fatalf("err=%v", err)
	}
}

func TestTestConnectionUnavailableModelFails(t *testing.T) {
	srv := fakeProvider(t)
	defer srv.Close()
	c := NewClient(Config{APIKey: "good-key", BaseURL: srv.URL, Model: "nope"})
	if err := c.TestConnection(context.Background()); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("err=%v", err)
	}
}

func TestTestConnectionNetworkErrorFails(t *testing.T) {
	srv := fakeProvider(t)
	url := srv.URL
	srv.Close()
	c := NewClient(Config{APIKey: "good-key", BaseURL: url, Model: "good-model"})
	if err := c.TestConnection(context.Background()); err == nil {
		t.Fatal("expected network error")
	}
}
