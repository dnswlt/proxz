# proxz agent skill

A drop-in [agent skill](https://docs.github.com/en/copilot/concepts/agents/about-agent-skills)
that teaches Copilot to read Jira, Confluence and Bitbucket Data Center through
`proxz` instead of reaching for credentials or `curl`.

Agent Skills are an open format, so the same directory works in Copilot, Claude
Code and anything else that implements the spec.

## Install

Copy the `proxz/` directory into one of these, keeping the folder name:

| Destination | Scope |
| --- | --- |
| `.github/skills/proxz/` | this repository, Copilot |
| `.claude/skills/proxz/` | this repository, Claude Code |
| `.agents/skills/proxz/` | this repository, any compliant agent |
| `~/.copilot/skills/proxz/` | all your projects, Copilot |
| `~/.agents/skills/proxz/` | all your projects, any compliant agent |

For example:

```sh
mkdir -p /path/to/your/repo/.github/skills
cp -r skills/proxz /path/to/your/repo/.github/skills/
```

Copilot loads the skill only when a task looks relevant, based on the
`description` in the frontmatter. Nothing else is needed.

## Prerequisite

`proxz` must be on `PATH` and configured — see the [main README](../README.md).
The skill deliberately tells the agent *not* to run `proxz login` itself, since
that would mean handling a token.

## Stopping the confirmation prompts

A bare `/rest/api/...` argument reads as an absolute filesystem path, so some
harnesses stop and ask. The skill therefore passes whole URLs, which are
unambiguous.

If prompts persist, allowlist the command: Copilot CLI takes
`--allow-tool 'shell(proxz)'`, VS Code has an auto-approve list. proxz cannot
write or reach an unconfigured host, so approving it once is safe.

## Alternative: always-on instructions

If you would rather have the rules apply to every request instead of only
relevant ones, copy the "Rules" section of [`proxz/SKILL.md`](proxz/SKILL.md)
into `.github/copilot-instructions.md`. Those few lines — never hunt for
tokens, never fall back to `curl` — are short and always true, which is what
instructions are for.

Copy only that section, not the whole file. The rest is mostly REST endpoint
tables, and they are dead weight on every task that has nothing to do with
Atlassian. Leaving them in the skill means Copilot loads them only when a task
actually calls for them.
