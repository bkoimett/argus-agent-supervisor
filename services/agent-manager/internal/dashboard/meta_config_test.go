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
