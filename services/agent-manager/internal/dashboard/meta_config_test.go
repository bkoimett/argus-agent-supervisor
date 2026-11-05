package dashboard

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/argus-command-center/argus/services/agent-manager/internal/meta"
)

func providerServer() *httptest.Server {
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

func testConnReq(s *Server, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/meta/test-connection", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleMetaTestConnection(rec, r)
	return rec
}

// Test uses unsaved form values (entered fake key must fail, entered good
// key must pass), must not persist, and must not mutate the running client.
func TestMetaTestConnectionUsesUnsavedValuesWithoutSaving(t *testing.T) {
	provider := providerServer()
	defer provider.Close()
	path := filepath.Join(t.TempDir(), ".env")
	// Running/saved client holds a different, deliberately wrong setup.
	stored := meta.NewClient(meta.Config{APIKey: "good-key", BaseURL: provider.URL, Model: "good-model"})
	s := &Server{MetaEnvPath: path, metaClientOverride: stored}

	rec := testConnReq(s, `{"base_url":"`+provider.URL+`","api_key":"fake-key","model":"good-model"}`)
	if !strings.Contains(rec.Body.String(), `"success":false`) || !strings.Contains(rec.Body.String(), "rejected the API key") {
		t.Fatalf("fake key: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "fake-key") {
		t.Fatal("response leaked entered key")
	}
	rec = testConnReq(s, `{"baseUrl":"`+provider.URL+`","apiKey":"good-key","model":"good-model"}`)
	if !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("good unsaved key (camelCase): %s", rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("Test Connection must not save the .env file")
	}
	if stored.GetBaseURL() != provider.URL || stored.GetModel() != "good-model" {
		t.Fatal("Test Connection mutated the running client")
	}
}

func TestMetaTestConnectionMissingKey(t *testing.T) {
	s := &Server{MetaEnvPath: filepath.Join(t.TempDir(), ".env"), metaClientOverride: meta.NewClient(meta.Config{})}
	rec := testConnReq(s, `{"base_url":"https://example.com/v1","api_key":"","model":"m"}`)
	if !strings.Contains(rec.Body.String(), `"success":false`) || !strings.Contains(rec.Body.String(), "missing API key") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

// GET must report the persisted file even when the running client started
// empty (e.g. service started before the file was written, or by another
// instance): the file is the source of truth for configuration status.
func TestMetaConfigGetReloadsPersistedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := writeMetaEnvFile(path, "https://example.com/v1", "k", "m1"); err != nil {
		t.Fatal(err)
	}
	client := meta.NewClient(meta.Config{})
	s := &Server{MetaEnvPath: path, metaClientOverride: client}
	r := httptest.NewRequest(http.MethodGet, "/api/meta/config", nil)
	rec := httptest.NewRecorder()
	s.handleMetaConfigGet(rec, r)
	body := rec.Body.String()
	if !strings.Contains(body, `"configured":true`) || !strings.Contains(body, `"model":"m1"`) || !strings.Contains(body, `"has_key":true`) {
		t.Fatalf("body=%s", body)
	}
	if strings.Contains(body, `"api_key"`) {
		t.Fatal("GET must never return the API key")
	}
}

func TestMetaConfigGetConfiguredRequiresKey(t *testing.T) {
	s := &Server{MetaEnvPath: filepath.Join(t.TempDir(), ".env"), metaClientOverride: meta.NewClient(meta.Config{BaseURL: "https://example.com/v1", Model: "m"})}
	r := httptest.NewRequest(http.MethodGet, "/api/meta/config", nil)
	rec := httptest.NewRecorder()
	s.handleMetaConfigGet(rec, r)
	if !strings.Contains(rec.Body.String(), `"configured":false`) || !strings.Contains(rec.Body.String(), `"has_key":false`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
	s.metaClientOverride = meta.NewClient(meta.Config{APIKey: "k", BaseURL: "https://example.com/v1", Model: "m"})
	rec = httptest.NewRecorder()
	s.handleMetaConfigGet(rec, r)
	if !strings.Contains(rec.Body.String(), `"configured":true`) || !strings.Contains(rec.Body.String(), `"has_key":true`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
}


func saveReq(s *Server, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/meta/config", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleMetaConfigSave(rec, r)
	return rec
}

func TestMetaConfigSaveSuccessAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", ".env")
	client := meta.NewClient(meta.Config{})
	s := &Server{MetaEnvPath: path, metaClientOverride: client}
	// camelCase body, as the desktop app sends it
	rec := saveReq(s, `{"baseUrl":"https://example.com/v1/","apiKey":"secret-key","model":"m1"}`)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-key") {
		t.Fatal("secret leaked in response")
	}
	vals, err := readMetaEnvFile(path)
	if err != nil || vals["META_BASE_URL"] != "https://example.com/v1" || vals["META_MODEL"] != "m1" || vals["META_API_KEY"] != "secret-key" {
		t.Fatalf("reload vals=%v err=%v", vals, err)
	}
	if client.GetBaseURL() != "https://example.com/v1" || client.GetModel() != "m1" || !client.IsEnabled() {
		t.Fatalf("client not updated: %s %s", client.GetBaseURL(), client.GetModel())
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm=%v want 0600", fi.Mode().Perm())
	}
	// Simulated restart: a fresh client built from the reloaded file values.
	fresh := meta.NewClient(meta.Config{APIKey: vals["META_API_KEY"], BaseURL: vals["META_BASE_URL"], Model: vals["META_MODEL"]})
	if !fresh.IsEnabled() || fresh.GetModel() != "m1" {
		t.Fatal("fresh client from reloaded file not configured")
	}
}

func TestMetaConfigSaveValidationFailure(t *testing.T) {
	s := &Server{MetaEnvPath: filepath.Join(t.TempDir(), ".env"), metaClientOverride: meta.NewClient(meta.Config{})}
	rec := saveReq(s, `{"base_url":"","model":""}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "base_url is required") || !strings.Contains(rec.Body.String(), "request_id") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = saveReq(s, `{"base_url":"ftp://x","model":"m"}`)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "http or https") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMetaConfigSaveFilesystemFailure(t *testing.T) {
	// Parent is a file, so MkdirAll/write must fail with a useful error.
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Server{MetaEnvPath: filepath.Join(blocker, ".env"), metaClientOverride: meta.NewClient(meta.Config{})}
	rec := saveReq(s, `{"base_url":"https://example.com/v1","api_key":"k","model":"m"}`)
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), "save meta config") || !strings.Contains(rec.Body.String(), "request_id") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}
