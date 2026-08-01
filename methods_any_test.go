//go:build any_methods

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchSendsBodiedWrite covers what an any_methods build exists for: the
// request must carry the method, the payload, a Content-Type the Atlassian
// APIs accept, and a Content-Length rather than a chunked body.
func TestFetchSendsBodiedWrite(t *testing.T) {
	var gotMethod, gotType, gotBody string
	var gotLen int64
	var gotTE []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod, gotType, gotBody = r.Method, r.Header.Get("Content-Type"), string(b)
		gotLen, gotTE = r.ContentLength, r.TransferEncoding
		w.WriteHeader(http.StatusCreated)
		io.WriteString(w, `{"id":"10000"}`)
	}))
	defer srv.Close()

	const payload = `{"body":"Fixed in 8f4c2a1"}`
	site := &Site{BaseURL: srv.URL, Token: "plain"}
	var out strings.Builder
	err := fetch(http.MethodPost, site, "/rest/api/2/issue/PROJ-123/comment", []byte(payload), &out)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotBody != payload {
		t.Errorf("body = %q, want %q", gotBody, payload)
	}
	if gotType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json; Jira and Confluence answer 415 without it", gotType)
	}
	if gotLen != int64(len(payload)) {
		t.Errorf("ContentLength = %d, want %d", gotLen, len(payload))
	}
	if len(gotTE) != 0 {
		t.Errorf("Transfer-Encoding = %v, want none: proxies in front of Data Center reject chunked bodies", gotTE)
	}
	if out.String() != `{"id":"10000"}` {
		t.Errorf("response body = %q", out.String())
	}
}

// A bodyless write must not announce a JSON payload it is not sending.
func TestFetchOmitsContentTypeWithoutBody(t *testing.T) {
	var gotType string
	var gotLen int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType, gotLen = r.Header.Get("Content-Type"), r.ContentLength
	}))
	defer srv.Close()

	site := &Site{BaseURL: srv.URL, Token: "plain"}
	var out strings.Builder
	if err := fetch(http.MethodDelete, site, "/rest/api/2/issue/PROJ-123", nil, &out); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotType != "" {
		t.Errorf("Content-Type = %q, want none for a bodyless request", gotType)
	}
	if gotLen != 0 {
		t.Errorf("ContentLength = %d, want 0", gotLen)
	}
}

// The write verbs must still be held to the same path allowlist as GET, or
// enabling writes would quietly widen what the token can reach.
func TestWritesObeyPathAllowlist(t *testing.T) {
	site := &Site{BaseURL: "https://jira.corp", Token: "plain"}
	var out strings.Builder
	err := fetch(http.MethodPost, site, "/secure/admin", []byte(`{}`), &out)
	if err == nil {
		t.Fatal("expected a POST outside the allowed prefixes to be refused")
	}
	if !strings.Contains(err.Error(), "outside the allowed prefixes") {
		t.Errorf("error = %q, want the allowlist explanation", err)
	}
}

// readBody must not block on a terminal, and must not attach a body to a GET.
func TestReadBodySkipsGet(t *testing.T) {
	body, err := readBody(http.MethodGet)
	if err != nil {
		t.Fatalf("readBody: %v", err)
	}
	if body != nil {
		t.Errorf("readBody(GET) = %q, want nil", body)
	}
}
