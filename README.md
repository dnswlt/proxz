# proxz

A read-only proxy for Confluence, Jira and Bitbucket **Data Center** REST APIs.

It exists so an LLM agent can fetch data from those systems without ever
touching a personal access token, and without being able to issue anything but
a `GET`.

## Build

```sh
make
```

The first build generates a random 32-byte key at `~/.config/proxz/build.key`
(mode 0600) and links it into the binary. Later builds reuse it, so stored
tokens keep working. The key is deliberately kept **outside** this directory
and is never committed — nothing in this repo reveals it.

Losing the key is not a disaster: rebuild and re-run `proxz login <site>`.

A plain `go build` still works for hacking on the code, but falls back to a key
published in this repo, and `login` warns you about it. Don't put a real PAT in
a keyless build.

Single static binary, no runtime dependencies.

## Setup (you do this once, not the agent)

```sh
./proxz login jira       https://jira.corp
./proxz login confluence https://confluence.corp
./proxz login bitbucket  https://bitbucket.corp
```

Each prompts for a PAT without echoing it, so the token never lands on screen
or in shell history. Config goes to `~/.config/proxz/config.json` (override
with `PROXZ_CONFIG` or `XDG_CONFIG_HOME`), mode `0600`.

## Usage

```sh
proxz get jira       /rest/api/2/issue/PROJ-123
proxz get jira       '/rest/api/2/search?jql=project%3DFOO&maxResults=50'
proxz get confluence '/rest/api/content/12345?expand=body.storage'
proxz get bitbucket  /rest/api/1.0/projects/FOO/repos

proxz sites          # list configured sites (never prints tokens)
proxz logout jira    # remove a site
```

No flags. 30s timeout.
Pipe to `jq` if you want it formatted.

The response body goes to stdout; errors go to stderr with a non-zero exit.
On an HTTP error the body is still printed, since the API's own error message
is usually the useful part.

## What this actually guarantees

Enforced, and covered by tests:

- **GET only.** There is no code path that issues another method. `proxz post
  ...` fails at argument parsing, before any network call.
- **Path allowlist.** Only paths under `/rest/` are permitted (to change them, edit `allowed_prefixes`
  in the config). Absolute URLs, protocol-relative `//host/...`, and
  `..` traversal are all rejected.
- **No cross-host redirects.** The `Authorization` header can never be replayed
  to a host other than the configured one.
- **No token echo.** No command prints a stored token, and no error message
  includes the header.
- **Config permissions.** proxz refuses to run if the config is group- or
  world-readable.

### On the token scrambling — read this part

Tokens are stored AES-GCM-scrambled under a key that is generated on your
machine and linked into the binary at build time. Reading this repo tells an
attacker nothing: the key is not in it.

The point is that an agent with broad filesystem read access cannot
`cat config.json` and walk away with a PAT it can paste into `curl` or carry
into its context. It sees base64 noise.

Still, be clear about the limits — **this raises the bar, it does not make
tokens safe from a local attacker.** To recover a token you need the config
file, plus the key (either `~/.config/proxz/build.key` or the built binary,
since `-ldflags -X` embeds the key as a plain string — `strings proxz` will
show it), plus a reimplementation of the derivation in `secret.go`. Anyone with
your user account and some determination can do all three. What they cannot do
is stumble over a plaintext secret, which is the realistic failure mode here.

So: keep the config file and the key file out of git, keep them mode 0600, and
if you need an actual guarantee use an OS keychain and accept the portability
cost.

## Telling your agent about it

Something like this in `CLAUDE.md` works well:

```markdown
To read from Jira/Confluence/Bitbucket, use `proxz get <site> <path>`,
e.g. `proxz get jira /rest/api/2/issue/PROJ-123`.
Never look for, read, or ask for API tokens — proxz handles auth itself.
It is read-only by design; there is no way to write through it.
```

## Tests

```sh
make test
```
