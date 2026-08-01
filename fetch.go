package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// checkPath validates a caller-supplied request path against a site's
// allowlist. It rejects anything that could redirect the request away from the
// configured base URL, or reach outside the permitted REST prefixes.
//
// The returned URL contains only a path and a query; it is never absolute.
func checkPath(site *Site, raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("empty path")
	}
	if !strings.HasPrefix(raw, "/") {
		return nil, fmt.Errorf("path must start with %q, got %q", "/", raw)
	}
	// "//host/x" parses as a protocol-relative URL pointing at another host.
	if strings.HasPrefix(raw, "//") {
		return nil, fmt.Errorf("path must not start with %q", "//")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	if u.Scheme != "" || u.Host != "" {
		return nil, errors.New("path must be relative to the site base URL, not an absolute URL")
	}
	// Resolve "." and ".." before checking the prefix, so that a path like
	// /rest/../secret cannot slip past the allowlist.
	clean := path.Clean(u.Path)
	if clean != u.Path && clean+"/" != u.Path {
		return nil, fmt.Errorf("path must be already normalized; did you mean %q?", clean)
	}
	allowed := false
	for _, p := range site.prefixes() {
		if strings.HasPrefix(clean, p) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, fmt.Errorf("path %q is outside the allowed prefixes for this site (%s)",
			clean, strings.Join(site.prefixes(), ", "))
	}
	return &url.URL{Path: clean, RawQuery: u.RawQuery}, nil
}

// newClient builds an HTTP client that will not follow a redirect to a
// different host, so the Authorization header can never be replayed elsewhere.
func newClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Host != via[0].URL.Host {
				return fmt.Errorf("refusing to follow redirect to another host (%s)", req.URL.Host)
			}
			return nil
		},
	}
}

const requestTimeout = 30 * time.Second

// fetch performs the GET request and streams the response body to w.
func fetch(site *Site, rawPath string, w io.Writer) error {
	rel, err := checkPath(site, rawPath)
	if err != nil {
		return err
	}
	base, err := url.Parse(site.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid base_url %q: %w", site.BaseURL, err)
	}
	if base.Scheme != "https" && base.Scheme != "http" {
		return fmt.Errorf("base_url %q must be http or https", site.BaseURL)
	}
	target := *base
	target.Path = strings.TrimSuffix(base.Path, "/") + rel.Path
	target.RawQuery = rel.RawQuery

	pat, err := site.PAT()
	if err != nil {
		return err
	}

	// http.NewRequest with a hardcoded GET: there is no code path in proxz
	// that issues any other method.
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+pat)
	// These APIs return JSON by default, so asking for it specifically buys
	// nothing and would risk a 406 on endpoints serving something else, such
	// as Bitbucket's raw file and diff resources.
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "proxz/1.0")

	resp, err := newClient(requestTimeout).Do(req)
	if err != nil {
		// url.Error stringifies the full URL but never the headers, so the
		// PAT cannot surface here.
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %s", target.Redacted(), resp.Status)
	}
	return nil
}
