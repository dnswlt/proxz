package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestScrambleRoundTrip(t *testing.T) {
	const pat = "NDc4OTAxMjM0NTY3OjhhYmNkZWY="
	scrambled, err := scramble(pat)
	if err != nil {
		t.Fatalf("scramble: %v", err)
	}
	if strings.Contains(scrambled, pat) {
		t.Errorf("scrambled value still contains the plaintext token: %q", scrambled)
	}
	if !strings.HasPrefix(scrambled, scramblePrefix) {
		t.Errorf("scrambled value %q lacks prefix %q", scrambled, scramblePrefix)
	}
	got, err := unscramble(scrambled)
	if err != nil {
		t.Fatalf("unscramble: %v", err)
	}
	if got != pat {
		t.Errorf("round trip: got %q, want %q", got, pat)
	}
}

func TestScrambleIsRandomized(t *testing.T) {
	// A fresh nonce each time means the same PAT does not produce a stable
	// ciphertext an observer could recognize across config files.
	a, err := scramble("token")
	if err != nil {
		t.Fatal(err)
	}
	b, err := scramble("token")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("scrambling the same token twice produced identical output")
	}
}

func TestUnscramblePassesThroughPlaintext(t *testing.T) {
	got, err := unscramble("hand-pasted-token")
	if err != nil {
		t.Fatalf("unscramble: %v", err)
	}
	if got != "hand-pasted-token" {
		t.Errorf("got %q, want the value unchanged", got)
	}
}

func TestUnscrambleRejectsTamperedValue(t *testing.T) {
	scrambled, err := scramble("token")
	if err != nil {
		t.Fatal(err)
	}
	// Flip a character in the ciphertext body.
	tampered := scrambled[:len(scrambled)-2] + "AA"
	if _, err := unscramble(tampered); err == nil {
		t.Error("expected an error for a tampered token, got nil")
	}
}

func TestCheckPathAllows(t *testing.T) {
	site := &Site{BaseURL: "https://jira.corp"}
	tests := []struct {
		raw       string
		wantPath  string
		wantQuery string
	}{
		{"/rest/api/2/issue/PROJ-123", "/rest/api/2/issue/PROJ-123", ""},
		{"/rest/api/content/12345?expand=body.storage", "/rest/api/content/12345", "expand=body.storage"},
		{"/rest/api/1.0/projects/FOO/repos", "/rest/api/1.0/projects/FOO/repos", ""},
		{"/rest/api/2/search?jql=project%3DFOO&maxResults=50", "/rest/api/2/search", "jql=project%3DFOO&maxResults=50"},
	}
	for _, tt := range tests {
		got, err := checkPath(site, tt.raw)
		if err != nil {
			t.Errorf("checkPath(%q): unexpected error %v", tt.raw, err)
			continue
		}
		if got.Path != tt.wantPath {
			t.Errorf("checkPath(%q).Path = %q, want %q", tt.raw, got.Path, tt.wantPath)
		}
		if got.RawQuery != tt.wantQuery {
			t.Errorf("checkPath(%q).RawQuery = %q, want %q", tt.raw, got.RawQuery, tt.wantQuery)
		}
	}
}

func TestCheckPathRejects(t *testing.T) {
	site := &Site{BaseURL: "https://jira.corp"}
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"relative", "rest/api/2/issue/PROJ-1"},
		{"absolute URL", "https://evil.example/rest/api"},
		{"scheme-relative host", "//evil.example/rest/api"},
		{"outside allowlist", "/secure/admin"},
		{"traversal out of prefix", "/rest/../secure/admin"},
		{"encoded traversal", "/rest/api/../../secure/admin"},
		{"root", "/"},
	}
	for _, tt := range tests {
		if _, err := checkPath(site, tt.raw); err == nil {
			t.Errorf("%s: checkPath(%q) succeeded, want an error", tt.name, tt.raw)
		}
	}
}

func TestCheckPathCustomPrefixes(t *testing.T) {
	site := &Site{BaseURL: "https://x", AllowedPrefixes: []string{"/rest/", "/plugins/servlet/"}}
	if _, err := checkPath(site, "/plugins/servlet/thing"); err != nil {
		t.Errorf("custom prefix should be allowed: %v", err)
	}
	if _, err := checkPath(site, "/secure/admin"); err == nil {
		t.Error("path outside custom prefixes should be rejected")
	}
}

// TestFetchSendsGetWithBearer is the end-to-end check that the request leaving
// proxz is a GET carrying the unscrambled PAT.
func TestFetchSendsGetWithBearer(t *testing.T) {
	var gotMethod, gotAuth, gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotAuth, gotURL = r.Method, r.Header.Get("Authorization"), r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"key":"PROJ-123"}`)
	}))
	defer srv.Close()

	token, err := scramble("s3cret-pat")
	if err != nil {
		t.Fatal(err)
	}
	site := &Site{BaseURL: srv.URL, Token: token}

	var out strings.Builder
	err = fetch(http.MethodGet, site, "/rest/api/2/issue/PROJ-123?fields=summary", &out)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotAuth != "Bearer s3cret-pat" {
		t.Errorf("Authorization = %q, want the unscrambled PAT", gotAuth)
	}
	if want := "/rest/api/2/issue/PROJ-123?fields=summary"; gotURL != want {
		t.Errorf("URL = %q, want %q", gotURL, want)
	}
	if out.String() != `{"key":"PROJ-123"}` {
		t.Errorf("body = %q", out.String())
	}
}

func TestFetchReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"errorMessages":["Issue does not exist"]}`, http.StatusNotFound)
	}))
	defer srv.Close()

	site := &Site{BaseURL: srv.URL, Token: "plain"}
	var out strings.Builder
	err := fetch(http.MethodGet, site, "/rest/api/2/issue/NOPE-1", &out)
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error %q should mention the status code", err)
	}
	// The body is still written so the caller can see the API's explanation.
	if !strings.Contains(out.String(), "Issue does not exist") {
		t.Errorf("body should still be written on error, got %q", out.String())
	}
}

// TestFetchRefusesCrossHostRedirect makes sure the PAT is never replayed to a
// host other than the configured one.
func TestFetchRefusesCrossHostRedirect(t *testing.T) {
	var leakedAuth string
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leakedAuth = r.Header.Get("Authorization")
	}))
	defer evil.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/rest/api", http.StatusFound)
	}))
	defer srv.Close()

	site := &Site{BaseURL: srv.URL, Token: "plain"}
	var out strings.Builder
	err := fetch(http.MethodGet, site, "/rest/api/2/myself", &out)
	if err == nil {
		t.Fatal("expected the cross-host redirect to be refused")
	}
	if leakedAuth != "" {
		t.Errorf("Authorization header leaked to the redirect target: %q", leakedAuth)
	}
}

func TestRunRejectsNonGetVerbs(t *testing.T) {
	if checkMethodAllowed("post") == nil {
		t.Skip("skipping test when any_methods build tag is enabled")
	}
	for _, verb := range []string{"post", "put", "delete", "patch"} {
		err := run([]string{verb, "jira", "/rest/api/2/issue"})
		if err == nil {
			t.Errorf("run(%q) succeeded, want an error", verb)
			continue
		}
		if !strings.Contains(err.Error(), "only performs GET") {
			t.Errorf("run(%q) error = %q, want it to explain the GET-only rule", verb, err)
		}
	}
}

func TestBuildKeyChangesDerivedKey(t *testing.T) {
	orig := buildKey
	defer func() { buildKey = orig }()

	buildKey = ""
	fallback, err := scramble("token")
	if err != nil {
		t.Fatal(err)
	}
	buildKey = "machine-specific-key"
	if _, err := unscramble(fallback); err == nil {
		t.Error("a token scrambled without a build key must not decode with one")
	}
	private, err := scramble("token")
	if err != nil {
		t.Fatal(err)
	}
	got, err := unscramble(private)
	if err != nil || got != "token" {
		t.Errorf("round trip under a build key: got %q, err %v", got, err)
	}
	buildKey = "a-different-key"
	if _, err := unscramble(private); err == nil {
		t.Error("a token from another build key must not decode")
	}
}

func TestUsingBuildKey(t *testing.T) {
	orig := buildKey
	defer func() { buildKey = orig }()
	buildKey = ""
	if usingBuildKey() {
		t.Error("usingBuildKey() = true with an empty buildKey")
	}
	buildKey = "x"
	if !usingBuildKey() {
		t.Error("usingBuildKey() = false with a buildKey set")
	}
}

func TestSiteForURL(t *testing.T) {
	cfg := &Config{Sites: map[string]*Site{
		"jira":       {BaseURL: "https://jira.corp"},
		"confluence": {BaseURL: "https://wiki.corp"},
		"ctx":        {BaseURL: "https://corp.example/bitbucket"},
		"root":       {BaseURL: "https://corp.example"},
	}}
	tests := []struct {
		raw      string
		wantBase string
		wantPath string
	}{
		{"https://jira.corp/rest/api/2/issue/PROJ-1", "https://jira.corp", "/rest/api/2/issue/PROJ-1"},
		{"https://wiki.corp/rest/api/content/1?expand=body.storage", "https://wiki.corp", "/rest/api/content/1?expand=body.storage"},
		// The context-path site must win over the one at the bare host.
		{"https://corp.example/bitbucket/rest/api/1.0/projects", "https://corp.example/bitbucket", "/rest/api/1.0/projects"},
		{"https://corp.example/rest/api/2/myself", "https://corp.example", "/rest/api/2/myself"},
	}
	for _, tt := range tests {
		site, path, err := cfg.siteForURL(tt.raw)
		if err != nil {
			t.Errorf("siteForURL(%q): unexpected error %v", tt.raw, err)
			continue
		}
		if site.BaseURL != tt.wantBase {
			t.Errorf("siteForURL(%q) site = %q, want %q", tt.raw, site.BaseURL, tt.wantBase)
		}
		if path != tt.wantPath {
			t.Errorf("siteForURL(%q) path = %q, want %q", tt.raw, path, tt.wantPath)
		}
	}

	for _, raw := range []string{
		"https://unknown.corp/rest/api/2/myself", // host not configured
		"http://jira.corp/rest/api/2/myself",     // scheme differs from the configured https
		"/rest/api/2/myself",                     // not a URL
		"jira",                                   // not a URL
	} {
		if _, _, err := cfg.siteForURL(raw); err == nil {
			t.Errorf("siteForURL(%q) succeeded, want an error", raw)
		}
	}
}

// TestGetByURLEnforcesPrefixes checks that arriving via a full URL is subject
// to exactly the same allowlist as the two-argument form.
func TestGetByURLEnforcesPrefixes(t *testing.T) {
	cfg := &Config{Sites: map[string]*Site{"jira": {BaseURL: "https://jira.corp"}}}
	site, path, err := cfg.siteForURL("https://jira.corp/secure/admin")
	if err != nil {
		t.Fatalf("siteForURL: %v", err)
	}
	if _, err := checkPath(site, path); err == nil {
		t.Error("a URL outside the allowed prefixes must still be rejected")
	}
}

// TestCheckPathAttachments covers the attachment subtrees, which are the only
// paths allowed outside /rest/. The surrounding /secure/ tree must stay shut.
func TestCheckPathAttachments(t *testing.T) {
	site := &Site{BaseURL: "https://jira.corp"}
	allowed := []string{
		"/secure/attachment/10000/screenshot.png",
		"/download/attachments/12345/design.pdf?version=1&modificationDate=1700000000000",
	}
	for _, p := range allowed {
		if _, err := checkPath(site, p); err != nil {
			t.Errorf("checkPath(%q) should be allowed: %v", p, err)
		}
	}
	denied := []string{
		"/secure/admin",
		"/secure/AdminSummary.jspa",
		"/secure/Dashboard.jspa",
		"/download/resources/some.js",
	}
	for _, p := range denied {
		if _, err := checkPath(site, p); err == nil {
			t.Errorf("checkPath(%q) should be rejected", p)
		}
	}
}
