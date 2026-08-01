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
proxz get <url>            # perform a GET; response body goes to stdout
proxz sites                # list configured sites and their base URLs
```

**Pass a whole URL.** proxz works out which configured site it belongs to and
refuses any host it does not recognise. Quote the URL when it contains a query
string:

```sh
proxz get https://jira.corp/rest/api/2/issue/PROJ-123
proxz get 'https://jira.corp/rest/api/2/search?jql=project%3DPROJ%20AND%20status%3DOpen&maxResults=50'
proxz get 'https://wiki.corp/rest/api/content/12345?expand=body.storage'
proxz get https://bitbucket.corp/rest/api/1.0/projects/PROJ/repos
```

Run `proxz sites` to learn the base URLs. The tables below list paths only, so
join them onto the right base URL from that output.

A site name plus a path also works — `proxz get jira /rest/api/2/issue/PROJ-123`
— but prefer the URL form. A bare `/rest/...` argument looks like a filesystem
path and tends to make agent harnesses stop and ask for confirmation.

Everything after the base URL must start with `/rest/`, in either form — plus
the two attachment download subtrees described under Attachments below.

Output is the response body unchanged — JSON for most endpoints, plain bytes
for things like raw files and diffs. These APIs are verbose: a single Jira
issue is tens of kilobytes, most of it irrelevant. Narrow the request itself
wherever the API allows it, rather than fetching everything and discarding it:

```sh
proxz get 'https://jira.corp/rest/api/2/issue/PROJ-123?fields=summary,status,assignee'
```

Jira takes `fields`, Confluence takes `expand` (request only the expansions you
need), and every list endpoint takes a page size. If you still need to reshape
what comes back and `jq` is available, pipe to it:

```sh
proxz get 'https://jira.corp/rest/api/2/issue/PROJ-123?fields=summary,status' | jq '{key, summary: .fields.summary}'
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
| Attachment metadata | `/rest/api/2/attachment/10000` |
| Current user | `/rest/api/2/myself` |

Confluence (`/rest/api/`):

| Goal | Path |
| --- | --- |
| Page by id, with body | `/rest/api/content/12345?expand=body.storage` |
| Page by space and title | `/rest/api/content?spaceKey=SPACE&title=Page+Title&expand=body.storage` |
| Child pages | `/rest/api/content/12345/child/page` |
| Search by CQL | `/rest/api/content/search?cql=<url-encoded>` |
| Attachments on a page | `/rest/api/content/12345/child/attachment` |

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
proxz get 'https://bitbucket.corp/rest/api/1.0/projects/PROJ/repos/my-repo/raw/src/main.go?at=refs/heads/main'
proxz get 'https://bitbucket.corp/rest/api/1.0/projects/PROJ/repos/my-repo/raw/README.md?at=8f4c2a1'
```

**Prefer `raw` for reading a file.** It returns the file bytes as-is, with no
JSON wrapper and no pagination, so what you get is the whole file.

`browse` returns the same content as a JSON `lines` array of `{"text": ...}`
objects. It is useful when you want structure, but it is **paginated like any
other list**, so a long file is silently cut off at the default page size. If
you use it, pass `limit` and check `isLastPage` before concluding you have seen
the whole file:

```sh
proxz get 'https://bitbucket.corp/rest/api/1.0/projects/PROJ/repos/my-repo/browse/src/main.go?limit=2000'
```

## Attachments

Attachment **bytes do not live under `/rest/`** in Jira or Confluence, and the
download URL is not something you should construct by hand. Fetch the metadata
first, take the URL out of it, then fetch that URL.

Jira — the attachment id comes from the issue's `fields.attachment` array:

```sh
# 1. metadata; the "content" field holds the full download URL
proxz get https://jira.corp/rest/api/2/attachment/10000

# 2. fetch that URL verbatim
proxz get 'https://jira.corp/secure/attachment/10000/screenshot.png' > screenshot.png
```

Confluence — list a page's attachments, then follow `_links.download`, which is
relative and must be appended to the site's base URL:

```sh
# 1. list attachments on page 12345 (add ?filename=x.pdf to narrow)
proxz get https://wiki.corp/rest/api/content/12345/child/attachment

# 2. base URL + the _links.download value
proxz get 'https://wiki.corp/download/attachments/12345/design.pdf?version=1&modificationDate=1700000000000' > design.pdf
```

Keep the whole query string on Confluence download links — `version` and
`modificationDate` are part of how the file is addressed. Redirect to a file
rather than letting binary content into your output.

Bitbucket has no equivalent step: repository files come straight from `raw`,
described above.

## Pagination

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
| `no configured site matches https://host` | That host is not set up. Run `proxz sites` and use one of the base URLs listed. |
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
