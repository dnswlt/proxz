---
name: proxz
description: Read data from self-hosted Jira, Confluence, or Bitbucket Data Center through the proxz CLI. Use whenever a task needs issue details, JQL search results, Confluence page content, or repository files, branches and pull requests from an on-premise Atlassian instance.
---

# Reading Atlassian Data Center data with proxz

`proxz` is a small CLI that performs authenticated **GET** requests against
Jira, Confluence and Bitbucket Data Center. It holds the personal access
tokens itself, so you never see, need, or ask for credentials.

Use it instead of `curl`, `wget`, or any HTTP library for these hosts. A direct
request will fail: you do not have the token, and you should not go looking for
one.

## Commands

```sh
proxz get <site> <path>    # perform a GET; response body goes to stdout
proxz sites                # list configured site names and their base URLs
```

Run `proxz sites` first if you do not know which site names exist. They are
short labels chosen at setup, typically `jira`, `confluence`, `bitbucket`.

The path is everything after the base URL and must start with `/rest/`.
Quote it when it contains a query string:

```sh
proxz get jira /rest/api/2/issue/PROJ-123
proxz get jira '/rest/api/2/search?jql=project%3DPROJ%20AND%20status%3DOpen&maxResults=50'
proxz get confluence '/rest/api/content/12345?expand=body.storage'
proxz get bitbucket /rest/api/1.0/projects/PROJ/repos
```

Output is the response body unchanged — JSON for most endpoints, plain bytes
for things like raw files and diffs. These APIs are verbose: a single Jira
issue is tens of kilobytes, most of it irrelevant. Narrow the request itself
wherever the API allows it, rather than fetching everything and discarding it:

```sh
proxz get jira '/rest/api/2/issue/PROJ-123?fields=summary,status,assignee'
```

Jira takes `fields`, Confluence takes `expand` (request only the expansions you
need), and every list endpoint takes a page size. If you still need to reshape
what comes back and `jq` is available, pipe to it:

```sh
proxz get jira '/rest/api/2/issue/PROJ-123?fields=summary,status' | jq '{key, summary: .fields.summary}'
```

## Useful endpoints

Jira (`/rest/api/2/`):

| Goal | Path |
| --- | --- |
| One issue | `/rest/api/2/issue/PROJ-123` |
| Issue with only some fields | `/rest/api/2/issue/PROJ-123?fields=summary,status,assignee` |
| Search by JQL | `/rest/api/2/search?jql=<url-encoded>&maxResults=50` |
| Issue comments | `/rest/api/2/issue/PROJ-123/comment` |
| Project metadata | `/rest/api/2/project/PROJ` |
| Current user | `/rest/api/2/myself` |

Confluence (`/rest/api/`):

| Goal | Path |
| --- | --- |
| Page by id, with body | `/rest/api/content/12345?expand=body.storage` |
| Page by space and title | `/rest/api/content?spaceKey=SPACE&title=Page+Title&expand=body.storage` |
| Child pages | `/rest/api/content/12345/child/page` |
| Search by CQL | `/rest/api/content/search?cql=<url-encoded>` |

Bitbucket (`/rest/api/1.0/`):

| Goal | Path |
| --- | --- |
| Repos in a project | `/rest/api/1.0/projects/PROJ/repos` |
| One repo | `/rest/api/1.0/projects/PROJ/repos/my-repo` |
| Open pull requests | `/rest/api/1.0/projects/PROJ/repos/my-repo/pull-requests?state=OPEN` |
| One pull request | `/rest/api/1.0/projects/PROJ/repos/my-repo/pull-requests/42` |
| PR diff | `/rest/api/1.0/projects/PROJ/repos/my-repo/pull-requests/42/diff` |
| Branches | `/rest/api/1.0/projects/PROJ/repos/my-repo/branches` |
| File contents | `/rest/api/1.0/projects/PROJ/repos/my-repo/raw/src/main.go` |
| File contents as JSON lines | `/rest/api/1.0/projects/PROJ/repos/my-repo/browse/src/main.go` |
| Directory listing | `/rest/api/1.0/projects/PROJ/repos/my-repo/files/src` |

To read a file at a specific branch, tag or commit, add `at`. Without it you
get the default branch:

```sh
proxz get bitbucket '/rest/api/1.0/projects/PROJ/repos/my-repo/raw/src/main.go?at=refs/heads/main'
proxz get bitbucket '/rest/api/1.0/projects/PROJ/repos/my-repo/raw/README.md?at=8f4c2a1'
```

**Prefer `raw` for reading a file.** It returns the file bytes as-is, with no
JSON wrapper and no pagination, so what you get is the whole file.

`browse` returns the same content as a JSON `lines` array of `{"text": ...}`
objects. It is useful when you want structure, but it is **paginated like any
other list**, so a long file is silently cut off at the default page size. If
you use it, pass `limit` and check `isLastPage` before concluding you have seen
the whole file:

```sh
proxz get bitbucket '/rest/api/1.0/projects/PROJ/repos/my-repo/browse/src/main.go?limit=2000'
```

Pagination parameters differ by product, and mixing them up silently returns a
default page rather than an error:

- Jira: `startAt` and `maxResults`
- Confluence: `start` and `limit`
- Bitbucket: `start` and `limit` (default limit 25)

All three report whether more results remain, so check before assuming a list
is complete.

## Errors

| Message | Meaning |
| --- | --- |
| `unknown command "post"` | proxz is read-only; there is no way to write. Stop and tell the user what you would have changed. |
| `path ... is outside the allowed prefixes` | The path must begin with `/rest/`. A URL copied from the browser is not an API path: Jira's `/browse/PROJ-123` is `/rest/api/2/issue/PROJ-123`, and Confluence's `/display/SPACE/Title` is `/rest/api/content?spaceKey=SPACE&title=Title`. |
| `unknown site "x"` | Run `proxz sites` and use one of the listed names. |
| `no sites configured` | Setup has not been done. Ask the user to run `proxz login <site> <url>`; do not attempt it yourself. |
| `returned 404` | Wrong id, key, or path. The response body usually explains. |
| `returned 401`/`403` | The stored token lacks access, or has expired. Ask the user to re-run `proxz login`. |

## Rules

- **Never** search for, read, print, or ask for API tokens. proxz handles
  authentication. Files like `~/.config/proxz/config.json` are off-limits and
  contain nothing useful to you.
- **Never** try to reach these hosts by another route — `curl`, `wget`,
  `requests`, a browser tool — whether or not proxz just failed.
- proxz cannot create, update, transition, comment on, or delete anything. If a
  task needs a write, do the read-only part, then report exactly what change is
  needed and let the user make it.
