# proxz

A read-only proxy for Confluence, Jira and Bitbucket **Data Center** REST APIs.

It lets an LLM agent read from those systems without ever handling a personal
access token, and without being able to issue anything but a `GET`.

It does the job of an MCP server without the server: any agent that can run a
shell command can use it.

## Build

Requires Go 1.25 or later.

```sh
git clone https://github.com/dnswlt/proxz.git
cd proxz
make
cp proxz ~/bin/    # or anywhere on your PATH
```

The result is a single static binary with no runtime dependencies. Build with
`make`, not `go build`: only `make` links in the machine-local key that
encrypts stored tokens (see
[Token storage](#token-storage-and-its-limits)).

`make` produces a read-only binary. For a build that can also write, see
[Writes](#writes).

## Setup

Configure each site once:

```sh
proxz login jira       https://jira.corp
proxz login confluence https://confluence.corp
proxz login bitbucket  https://bitbucket.corp
```

Each prompts for a PAT without echoing it, so the token never lands on screen
or in shell history. Config goes to `~/.config/proxz/config.json` (override
with `PROXZ_CONFIG` or `XDG_CONFIG_HOME`), mode `0600`.

## Usage

```sh
proxz get https://jira.corp/rest/api/2/issue/PROJ-123
proxz get 'https://jira.corp/rest/api/2/search?jql=project%3DFOO&maxResults=50'

# or name the site instead of repeating the host
proxz get jira       /rest/api/2/issue/PROJ-123
proxz get confluence '/rest/api/content/12345?expand=body.storage'
proxz get bitbucket  /rest/api/1.0/projects/FOO/repos

proxz sites          # list configured sites (never prints tokens)
proxz logout jira    # remove a site
```

A whole URL works as well as a site plus path: proxz matches the URL against
the configured sites (longest base URL wins, so context paths resolve
correctly) and refuses any host it does not recognise. The path allowlist
applies either way: `/rest/`, plus `/secure/attachment/` and
`/download/attachments/`, since Jira and Confluence serve attachment bytes from
outside their REST trees.

No flags, 30s timeout. The response body goes to stdout unmodified; pipe to
`jq` to format it. Errors go to stderr with a non-zero exit, and on an HTTP
error the body is still printed, since the API's own error message is usually
the useful part.

## Writes

`make build-any` compiles in `post`, `put`, `patch` and `delete`; `make` does
not. The choice is a build tag rather than a config flag because a config flag
is something an agent can edit.

```sh
echo '{"body":"Fixed in 8f4c2a1"}' |
  proxz post https://jira.corp/rest/api/2/issue/PROJ-123/comment
```

The body comes from stdin and is sent as `application/json`. Everything else
still applies: same path allowlist, same host check, same refusal to follow a
redirect off-host. `proxz methods` prints what the binary you have permits, so
an agent can ask rather than guess.

## Agent skill

[`skills/`](skills/) contains an agent skill covering the endpoints worth
knowing per product, attachments, pagination, and error messages. Agent Skills
are an open format, so it works in Claude Code, Copilot and anything else
implementing the spec. Installation: [`skills/README.md`](skills/README.md).

## Security properties

Enforced, and covered by tests:

- **GET only, unless built otherwise.** In a default build there is no code
  path that issues another method: `proxz post ...` fails at argument parsing,
  before any network call. A `make build-any` binary lifts this, and only this
  — see [Writes](#writes).
- **Path allowlist.** Only paths under `/rest/`, `/secure/attachment/` and
  `/download/attachments/` are permitted (to change them, edit
  `allowed_prefixes` in the config). Absolute URLs, protocol-relative
  `//host/...`, and `..` traversal are all rejected.
- **No cross-host redirects.** The `Authorization` header can never be replayed
  to a host other than the configured one.
- **No token echo.** No command prints a stored token, and no error message
  includes the header.
- **Owner-only config.** proxz writes the config at mode 0600.

### Token storage, and its limits

Tokens are stored AES-GCM-encrypted under a key generated on your machine and
linked into the binary at build time. The first `make` writes a random 32-byte
key to `~/.config/proxz/build.key` (mode 0600); later builds reuse it, so
stored tokens survive a rebuild. The key stays outside the working tree and is
never committed — it is not in this repo. Lose it and you rebuild and re-run
`proxz login <site>`.

A plain `go build` produces a working binary, but falls back to a key published
in this repo, and `login` warns you about it. Do not put a real PAT in a
keyless build.

The threat this addresses is narrow: an agent with broad filesystem read access
cannot `cat config.json` and walk away with a usable PAT. It sees base64 noise.

**This raises the bar; it does not make tokens safe from a local attacker.**
Recovering a token requires the config file, the key (from
`~/.config/proxz/build.key`, or from the binary — `-ldflags -X` embeds it as a
plain string, so `strings proxz` will show it), and a reimplementation of the
derivation in `secret.go`. Anyone with access to your user account can do all
three. What they cannot do is stumble over a plaintext secret.

Keep the config and key files out of git and at mode 0600. If you need a real
guarantee, use an OS keychain and accept the portability cost.

## Tests

```sh
make test
```
