---
name: proxz
description: Read and update self-hosted Jira, Confluence and Bitbucket Data Center through the proxz CLI. Use whenever a task touches an on-premise Atlassian instance - issue details, JQL searches, page content, repo files, branches, pull requests, or writing comments, transitions and page edits - including when the user names only an issue key, page or repo.
---

# Reading and writing Atlassian Data Center data with proxz

`proxz` performs authenticated requests against Jira, Confluence and Bitbucket
Data Center. It holds the tokens itself, so you never see or need credentials.
Use it instead of `curl`, `wget` or any HTTP library for these hosts: a direct
request has no token and will fail.

```sh
proxz get <url>            # response body goes to stdout
proxz sites                # configured sites and their base URLs
proxz methods              # HTTP methods this build permits
```

Pass whole URLs — proxz resolves the site and refuses unknown hosts. Quote
anything with a query string. Run `proxz sites` for the base URLs; the tables
below list paths only.

```sh
proxz get https://jira.corp/rest/api/2/issue/PROJ-123
proxz get 'https://wiki.corp/rest/api/content/12345?expand=body.storage'
```

These APIs are verbose — a single issue is tens of kilobytes. Narrow the
request rather than fetching everything: Jira takes `fields`, Confluence takes
`expand`, list endpoints take a page size. Pipe to `jq` to reshape.

```sh
proxz get 'https://jira.corp/rest/api/2/issue/PROJ-123?fields=summary,status,assignee'
```

## Endpoints

Jira:

| Goal | Path |
| --- | --- |
| One issue | `/rest/api/2/issue/PROJ-123` |
| JQL search | `/rest/api/2/search?jql=<url-encoded>&maxResults=50` |
| Comments | `/rest/api/2/issue/PROJ-123/comment` |
| Available transitions | `/rest/api/2/issue/PROJ-123/transitions` |
| Attachment metadata | `/rest/api/2/attachment/10000` |

Confluence:

| Goal | Path |
| --- | --- |
| Page by id, with body | `/rest/api/content/12345?expand=body.storage,version` |
| Page by space and title | `/rest/api/content?spaceKey=SPACE&title=Page+Title&expand=body.storage` |
| Child pages | `/rest/api/content/12345/child/page` |
| CQL search | `/rest/api/content/search?cql=<url-encoded>` |
| Attachments on a page | `/rest/api/content/12345/child/attachment` |

Bitbucket:

| Goal | Path |
| --- | --- |
| Repos in a project | `/rest/api/1.0/projects/PROJ/repos` |
| Pull requests | `/rest/api/1.0/projects/PROJ/repos/my-repo/pull-requests?state=OPEN` |
| One pull request | `/rest/api/1.0/projects/PROJ/repos/my-repo/pull-requests/42` |
| PR diff | `/rest/api/1.0/projects/PROJ/repos/my-repo/pull-requests/42/diff` |
| Branches | `/rest/api/1.0/projects/PROJ/repos/my-repo/branches` |
| File contents | `/rest/api/1.0/projects/PROJ/repos/my-repo/raw/src/main.go?at=refs/heads/main` |
| Directory listing | `/rest/api/1.0/projects/PROJ/repos/my-repo/files/src` |

Prefer `raw` for file contents: it returns the bytes whole. `browse` returns
the same content as a JSON `lines` array but is paginated, so a long file is
silently truncated unless you pass `limit` and check `isLastPage`.

Pagination parameters differ, and mixing them up returns a default page rather
than an error: Jira uses `startAt`/`maxResults`, Confluence and Bitbucket use
`start`/`limit`. All three report whether more remains.

## Attachments

Attachment bytes are not under `/rest/`, and the download URL is not something
to construct by hand. Fetch the metadata, then the URL it gives you: Jira's
attachment metadata has a `content` field holding the full URL; Confluence's
`child/attachment` listing has `_links.download`, which is relative to the base
URL and whose query string is part of the address. Redirect to a file rather
than letting binary content into your output.

```sh
proxz get 'https://wiki.corp/download/attachments/12345/design.pdf?version=1&modificationDate=1700000000000' > design.pdf
```

## Writing

Only if `proxz methods` lists more than `GET`. If it lists `GET` alone, this
build cannot write: do the read-only part, report what change is needed, and
leave it to the user.

Write the body to a file and pass `--body-file`; it is sent as
`application/json`. Prefer this over shell quoting, which is where these calls
usually break. `--body-file -` reads stdin, and no flag means no body.

```sh
proxz post --body-file comment.json https://jira.corp/rest/api/2/issue/PROJ-123/comment
proxz put --body-file page.json https://wiki.corp/rest/api/content/12345
```

| Goal | Request |
| --- | --- |
| Comment on an issue | POST `/rest/api/2/issue/PROJ-123/comment` — `{"body":"..."}` |
| Transition an issue | POST `/rest/api/2/issue/PROJ-123/transitions` — `{"transition":{"id":"31"}}` |
| Edit issue fields | PUT `/rest/api/2/issue/PROJ-123` — `{"fields":{"assignee":{"name":"charlie"}}}` |
| Update a page | PUT `/rest/api/content/12345` — see below |
| Comment on a PR | POST `/rest/api/1.0/projects/PROJ/repos/my-repo/pull-requests/42/comments` — `{"text":"..."}` |

Transition ids come from the workflow, so they differ between projects. Read
`/transitions` on the issue and use an id from that response.

A Confluence update replaces the page, so the PUT must carry `id`, `type`,
`title`, `space.key`, the full `body.storage`, and `version.number` set to the
current number plus one. GET it with `?expand=body.storage,version` first; a
stale version number gives a 409.

```json
{"id":"12345","type":"page","title":"Release checklist","space":{"key":"PLAT"},
 "body":{"storage":{"value":"<p>...</p>","representation":"storage"}},
 "version":{"number":7}}
```

Writes are not reversible here. Make the change the task asks for, one request
at a time.

## Errors

| Message | Meaning |
| --- | --- |
| `unknown command "post"` | This build is read-only. Report the change instead of attempting it another way. |
| `path ... is outside the allowed prefixes` | A browser URL is not an API path: Jira's `/browse/PROJ-123` is `/rest/api/2/issue/PROJ-123`, Confluence's `/display/SPACE/Title` is `/rest/api/content?spaceKey=SPACE&title=Title`. |
| `no configured site matches https://host` | Not set up. Run `proxz sites` and use a listed base URL. |
| `no sites configured` | Ask the user to run `proxz login <site> <url>`; do not attempt it yourself. |
| `returned 401`/`403` | The token lacks access or has expired. Ask the user to re-run `proxz login`. |
| `returned 409` on a write | Someone changed it since you read it. Re-read and rebuild; do not resend the same body. |

## Rules

- **Never** search for, read, print or ask for API tokens. proxz handles
  authentication.
- **Never** reach these hosts another way — `curl`, `wget`, `requests`, a
  browser tool — whether or not proxz just failed.
