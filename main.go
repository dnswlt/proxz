// Command proxz is a read-only proxy for Confluence, Jira and Bitbucket Data
// Center REST APIs.
//
// It exists so an LLM agent can fetch data from those systems without ever
// handling a personal access token, and without being able to issue anything
// but a GET. See README.md for the threat model.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const usage = `proxz - read-only proxy for Jira/Confluence/Bitbucket Data Center REST APIs

Usage:
  proxz get <site> <path>      Perform a GET against a configured site
  proxz sites                  List configured sites
  proxz login <site>           Store a personal access token for a site
  proxz logout <site>          Remove a site

Examples:
  proxz get jira /rest/api/2/issue/PROJ-123
  proxz get confluence '/rest/api/content/12345?expand=body.storage'
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
	case "get":
		return cmdGet(args[1:])
	case "sites":
		return cmdSites(args[1:])
	case "login":
		return cmdLogin(args[1:])
	case "logout":
		return cmdLogout(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	default:
		// Being explicit here matters: the most likely caller is an LLM that
		// guessed at a verb, and the error should teach it the rule.
		return fmt.Errorf("unknown command %q; proxz only performs GET requests", args[0])
	}
}

// parseArgs parses a flag set, allowing flags and positional arguments to be
// interleaved. The standard flag package stops at the first positional, which
// would silently ignore `proxz get jira /rest/... --pretty` - exactly the
// order a caller is most likely to type.
func parseArgs(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) == 0 {
			return positional, nil
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
}

func cmdGet(args []string) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	pretty := fs.Bool("pretty", false, "re-indent JSON responses")
	maxBytes := fs.Int64("max-bytes", 20<<20, "maximum response size to read")
	timeout := fs.Duration("timeout", 30*time.Second, "request timeout")
	accept := fs.String("accept", "application/json", "Accept header to send")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return fmt.Errorf("usage: proxz get <site> <path>")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	site, err := cfg.site(pos[0])
	if err != nil {
		return err
	}
	opts := fetchOptions{
		pretty:   *pretty,
		maxBytes: *maxBytes,
		timeout:  *timeout,
		accept:   *accept,
	}
	return fetch(site, pos[1], opts, os.Stdout)
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
		fmt.Println("no sites configured; run: proxz login <site> --url https://...")
		return nil
	}
	for _, name := range cfg.siteNames() {
		s := cfg.Sites[name]
		fmt.Printf("%-12s %s  [%s]\n", name, s.BaseURL, strings.Join(s.prefixes(), " "))
	}
	return nil
}

func cmdLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	baseURL := fs.String("url", "", "base URL of the instance, e.g. https://jira.corp")
	prefixes := fs.String("prefixes", "", "comma-separated allowed path prefixes (default /rest/)")
	pos, err := parseArgs(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return fmt.Errorf("usage: proxz login <site> --url https://...")
	}
	name := pos[0]

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	site := cfg.Sites[name]
	if site == nil {
		site = &Site{}
	}
	if *baseURL != "" {
		site.BaseURL = strings.TrimSuffix(*baseURL, "/")
	}
	if site.BaseURL == "" {
		return fmt.Errorf("site %q has no base URL; pass --url https://...", name)
	}
	if *prefixes != "" {
		site.AllowedPrefixes = strings.Split(*prefixes, ",")
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
