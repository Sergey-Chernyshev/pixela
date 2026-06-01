package gitlab

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommitFileUpdate(t *testing.T) {
	var gotPath, gotToken string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotToken = r.Header.Get("PRIVATE-TOKEN")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := New(srv.URL, "tok-123")
	if !c.Enabled() {
		t.Fatal("client should be enabled with a token")
	}
	err := c.CommitFile(context.Background(), "acme/storefront", "feat/x", ActionUpdate,
		"tests/__screenshots__/home.png", []byte("PNGDATA"), "msg", "Demo", "demo@pixela.dev")
	if err != nil {
		t.Fatalf("CommitFile: %v", err)
	}

	// path is URL-escaped project id + commits endpoint; token in header.
	if gotPath != "/api/v4/projects/acme%2Fstorefront/repository/commits" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotToken != "tok-123" {
		t.Fatalf("token = %q", gotToken)
	}
	if body["branch"] != "feat/x" || body["commit_message"] != "msg" || body["author_email"] != "demo@pixela.dev" {
		t.Fatalf("body meta = %v", body)
	}
	actions, _ := body["actions"].([]any)
	if len(actions) != 1 {
		t.Fatalf("actions = %v", body["actions"])
	}
	a := actions[0].(map[string]any)
	if a["action"] != "update" || a["file_path"] != "tests/__screenshots__/home.png" || a["encoding"] != "base64" {
		t.Fatalf("action = %v", a)
	}
	if dec, _ := base64.StdEncoding.DecodeString(a["content"].(string)); string(dec) != "PNGDATA" {
		t.Fatalf("content did not round-trip base64: %v", a["content"])
	}
}

func TestCommitFileDeleteOmitsContent(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	if err := New(srv.URL, "t").CommitFile(context.Background(), "1", "main", ActionDelete, "old.png", nil, "m", "n", "e"); err != nil {
		t.Fatalf("delete commit: %v", err)
	}
	a := body["actions"].([]any)[0].(map[string]any)
	if a["action"] != "delete" {
		t.Fatalf("action = %v", a["action"])
	}
	if _, ok := a["content"]; ok {
		t.Fatalf("delete action must not carry content: %v", a)
	}
}

func TestSetCommitStatus(t *testing.T) {
	var gotPath string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	err := New(srv.URL, "t").SetCommitStatus(context.Background(), "9", "abc123", StateSuccess, "pixela", "https://px/x", "all approved")
	if err != nil {
		t.Fatalf("SetCommitStatus: %v", err)
	}
	if gotPath != "/api/v4/projects/9/statuses/abc123" {
		t.Fatalf("path = %q", gotPath)
	}
	if body["state"] != "success" || body["target_url"] != "https://px/x" {
		t.Fatalf("body = %v", body)
	}
}

func TestNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()
	if err := New(srv.URL, "t").SetCommitStatus(context.Background(), "1", "s", StatePending, "n", "", ""); err == nil {
		t.Fatal("expected an error on 403")
	}
}

func TestDisabledWithoutToken(t *testing.T) {
	if New("https://gitlab.com", "").Enabled() {
		t.Fatal("client without a token must be disabled")
	}
}
