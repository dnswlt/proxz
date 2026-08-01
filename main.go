// Command proxz is a proxy for Confluence, Jira and Bitbucket Data Center REST
// APIs.
//
// It exists so an LLM agent can reach those systems without ever handling a
// personal access token. By default it is read-only: only GET is compiled in,
// and the other verbs fail at argument parsing. Building with the writes tag
// permits the remaining HTTP methods; see writes_disabled.go and
// writes_enabled.go, and README.md for the threat model.
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/term"
)

const usage = usageTagline + `

Usage:
  proxz get <url>              Perform a GET; the site is derived from the URL
  proxz get <site> <path>      Same, naming the site explicitly
` + usageWriteVerbs + `  proxz methods                List HTTP methods permitted by this build
  proxz sites                  List configured sites
  proxz login <site> <url>     Store a personal access token for a site
  proxz logout <site>          Remove a site

Examples:
  proxz get https://jira.corp/rest/api/2/issue/PROJ-123
  proxz get 'https://wiki.corp/rest/api/content/12345?expand=body.storage'
  proxz get bitbucket /rest/api/1.0/projects/FOO/repos
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "proxz: "+err.Error())
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return nil
	}
	switch args[0] {
	case "get", "post", "put", "patch", "delete", "head", "options":
		if err := checkMethodAllowed(args[0]); err != nil {
			return err
		}
		return cmdMethod(strings.ToUpper(args[0]), args[1:])
	case "sites":
		return cmdSites(args[1:])
	case "methods":
		return cmdMethods(args[1:])
	case "login":
		return cmdLogin(args[1:])
	case "logout":
		return cmdLogout(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		// A verb this build refuses is reported by checkMethodAllowed above,
		// which states the rule; anything reaching here is not a verb at all.
		return fmt.Errorf("unknown command %q; run 'proxz help'", args[0])
	}
}

func cmdMethod(method string, args []string) error {
	fs := flag.NewFlagSet(strings.ToLower(method), flag.ContinueOnError)
	bodyFile := fs.String("body-file", "", "file holding the request body, or - for stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	args = fs.Args()
	if *bodyFile != "" && (method == http.MethodGet || method == http.MethodHead) {
		return fmt.Errorf("%s takes no request body", method)
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	var site *Site
	var path string
	switch len(args) {
	case 1:
		// A whole URL: work out which site it belongs to.
		site, path, err = cfg.siteForURL(args[0])
	case 2:
		site, err = cfg.site(args[0])
		path = args[1]
	default:
		return fmt.Errorf("usage: proxz get <url>\n   or: proxz get <site> <path>")
	}
	if err != nil {
		return err
	}
	body, err := readBody(*bodyFile)
	if err != nil {
		return err
	}
	return fetch(method, site, path, body, os.Stdout)
}

// readBody loads the request payload named by --body-file. No flag means no
// body; "-" reads stdin.
func readBody(bodyFile string) ([]byte, error) {
	switch bodyFile {
	case "":
		return nil, nil
	case "-":
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading request body from stdin: %w", err)
		}
		return body, nil
	default:
		body, err := os.ReadFile(bodyFile)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
		return body, nil
	}
}

func cmdSites(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("usage: proxz sites")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if len(cfg.Sites) == 0 {
		fmt.Println("no sites configured; run: proxz login <site> <url>")
		return nil
	}
	for _, name := range cfg.siteNames() {
		s := cfg.Sites[name]
		fmt.Printf("%-12s %s  [%s]\n", name, s.BaseURL, strings.Join(s.prefixes(), " "))
	}
	return nil
}

func cmdMethods(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("usage: proxz methods")
	}
	printAllowedMethods()
	return nil
}

func cmdLogin(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: proxz login <site> <url>\n  e.g. proxz login jira https://jira.corp")
	}
	name := args[0]
	site := &Site{BaseURL: strings.TrimSuffix(args[1], "/")}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	// Keep any per-site prefix override the config already carries.
	if old := cfg.Sites[name]; old != nil {
		site.AllowedPrefixes = old.AllowedPrefixes
	}

	if !usingBuildKey() {
		fmt.Fprintln(os.Stderr,
			"warning: this binary was built without a private key, so the token will be\n"+
				"scrambled with the key published in this repo. Build with `make` instead.")
	}
	pat, err := readToken()
	if err != nil {
		return err
	}
	if pat == "" {
		return fmt.Errorf("no token entered")
	}
	if site.Token, err = scramble(pat); err != nil {
		return err
	}

	cfg.Sites[name] = site
	if err := cfg.save(); err != nil {
		return err
	}
	path, _ := configPath()
	fmt.Printf("stored token for %s (%s) in %s\n", name, site.BaseURL, path)
	return nil
}

func cmdLogout(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: proxz logout <site>")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if _, err := cfg.site(args[0]); err != nil {
		return err
	}
	delete(cfg.Sites, args[0])
	if err := cfg.save(); err != nil {
		return err
	}
	fmt.Printf("removed %s\n", args[0])
	return nil
}

// readToken reads a PAT from the terminal without echoing it, so it does not
// end up on screen or in shell history. If stdin is not a terminal the token
// is read from it directly, which allows `proxz login jira < token.txt`.
func readToken() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		var tok string
		if _, err := fmt.Fscanln(os.Stdin, &tok); err != nil {
			return "", fmt.Errorf("reading token from stdin: %w", err)
		}
		return strings.TrimSpace(tok), nil
	}
	fmt.Fprint(os.Stderr, "Personal access token: ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("reading token: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}
