# Configuration

Three mechanisms, split by what is being decided:

| Mechanism | Decides | Examples |
|---|---|---|
| env files (`~/.config/*.env`) | parameters and secrets of a chosen implementation | identity handles, tokens, agent binary, tool allowlists, timeouts |
| YAML (`~/.config/recdep/config.yaml`) | structured, human-edited tables | the dictation action map |
| systemd enrollment | which implementation runs each role | `telescreen install minitrue`, or your own producer's unit |

Choosing an implementation is never a config key: you enroll a unit.
Everything below configures the implementations this repo ships.

## What you can change, and where it lives

Three layers, from a file edit to a pull request:

| Layer | What lives there | How to change it |
|---|---|---|
| configuration | the action map, identity, secrets, agent binary, prompts, allowlists, timeouts, cadence | edit the files below; no rebuild |
| enrollment | which program plays each role: producer, drafting clerk, actor | enroll your own unit in place of a shipped one; the [recdep.md](../contracts/recdep.md) contract is the interface |
| the binary | the drawer names and record grammar, the keys, the double-key approval, the publisher match rules (which URL goes to GitHub, Slack, Linear) | a pull request to this repo |

The skills are configuration too, despite living under `~/.claude/`:
the agents read the installed files at run time, so editing
`~/.claude/skills/speakwrite/SKILL.md` changes how drafts are written
without touching Go. Anything the table's last row names is
deliberately fixed: the grammar is what makes every component
replaceable, so it moves by PR, not per user.

The skills that `telescreen install` writes under `~/.claude/skills/`
are seeds: edit them freely, the agents read the installed files at
run time and re-installs keep your edits (`--force` restores the
shipped versions). No rebuild is ever needed to tweak a prompt.

## config.yaml: the action map

`~/.config/recdep/config.yaml` overrides the dictation action map, the
table that picks the speakwrite action when you press `s` on a record.

An action is a verb: when you dictate, the screen writes it into the
intent file as `action <verb>`, and the drafting clerk composes the
draft that verb asks for. The shipped speakwrite skill knows `review`,
`vet-findings`, `pr-reply`, `slack-reply`, `linear-comment`, and
`respond` (its SKILL.md carries the verb-to-draft table); any other
verb works as soon as your installed skill says what to draft for it.

The map is a list of rules under one key, `actions`. Each rule:

| Field | Matches when | Example |
|---|---|---|
| `source` | it equals the record's source tag, the `[<source>]` opening the first line | `github`, `slack`, `linear` |
| `name_contains` | the record's filename contains it | `-review-requested-` |
| `who_suffix` | the record's author (the `<who>` on the first line) ends with it | `[bot]` |
| `action` | never; this is the verdict, the verb a matching record dictates | `review` |

Rules match top-down and the first match wins; every field except
`action` is optional, and an omitted field matches anything, so a rule
with only `action` catches everything and belongs last. When no rule
matches, the action is `respond`. A non-empty `actions` list replaces
the built-in table entirely (bring every rule you still want); an
empty or absent list keeps the built-ins.

```yaml
actions:
  - source: github            # exact match on the record's source tag
    name_contains: -review-requested-   # substring of the filename
    action: review
  - source: slack
    action: slack-reply
  - who_suffix: "[bot]"       # suffix of the author
    action: vet-findings
```

The built-in table (applied when the file is absent or the list empty):

| source | name contains | who suffix | action |
|---|---|---|---|
| github-review-requested | | | review |
| github | -review-requested- | | review |
| github | | [bot] | vet-findings |
| github | | | pr-reply |
| slack | | | slack-reply |
| linear | | | linear-comment |
| (anything else) | | | respond |

Action names are free-form verbs: the drafting agent interprets them,
so a custom action like `summarize` works as soon as your speakwrite
prompt knows what to do with it. The YAML is parsed strictly: an
unknown key is a startup error shown once in the status line, and the
built-ins stand.

## minitrue.env: the producer

`~/.config/minitrue.env`, plain `KEY=value` lines.

| Key | Meaning | Default |
|---|---|---|
| SLACK_USER_ID | your Slack user id, the person being watched | required |
| GH_LOGIN | your GitHub login; `gh` must resolve `@me` to it | required |
| LINEAR_ASSIGNEE | the Linear assignee to watch | `me` |
| REPO | the GitHub repo to scope PR watches to | required |
| BOT_LOGINS | bot logins to skip, besides `[bot]` suffixes | empty |
| MINITRUE_AGENT | the agent binary the subcommand runs | `claude` |
| MINITRUE_PROMPT | the headless prompt | `/minitrue produce` |
| MINITRUE_ALLOWED_TOOLS | the agent's tool allowlist | the subcommand's default |
| MINITRUE_TIMEOUT | seconds before the subcommand kills the run | `600` |

Swapping the LLM agent is `MINITRUE_AGENT` (an absolute path when it
is not on the unit's PATH) plus a prompt of your own; the allowlist
carries your environment's MCP tool identifiers. A timeout above 900
also needs `TimeoutStartSec` raised in the unit, or systemd kills the
run first.

## speakwrite.env: the drafting clerk

`~/.config/speakwrite.env`, same pattern:

| Key | Meaning | Default |
|---|---|---|
| SPEAKWRITE_AGENT | the agent binary | `claude` |
| SPEAKWRITE_PROMPT | the headless prompt | `/speakwrite draft` |
| SPEAKWRITE_ALLOWED_TOOLS | the agent's tool allowlist | the subcommand's default |
| SPEAKWRITE_TIMEOUT | seconds before the subcommand kills the run | `600` |

## thinkpol.env: the actor's credentials

`~/.config/thinkpol.env`, loaded by the service unit; it holds
secrets, so `chmod 600` it.

| Key | Meaning | Needed for |
|---|---|---|
| SLACK_TOKEN | a user token with `chat:write`; posts as you | the slack-thread publisher |
| LINEAR_API_KEY | a Linear API key | the linear-issue publisher |

The github-pr publisher uses your authenticated `gh` and needs no
entry here. A missing token fails the post gracefully: the draft
survives, the approval is consumed, `publish.log` names the reason.

Two testing knobs, read from the environment when set:

- `SLACK_API_BASE` replaces the Slack Web API root (default `https://slack.com/api`).
- `LINEAR_API_BASE` replaces the Linear API root (default `https://api.linear.app`).

## Everything else

- The state root honors `XDG_STATE_HOME` (default
  `~/.local/state/recdep`).
- Dictation opens `$VISUAL`, else `$EDITOR`, else `vi`; values may
  carry flags (`code -w`).
- Producer cadence (10 minutes) and the path units' trigger bounds are
  systemd settings: override with `systemctl --user edit
  minitrue.timer` rather than a config file.
