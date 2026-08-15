# Configuration reference

Lookup page for everything configurable through files: the dictation
action map in `config.yaml` and the per-role env files. Everything else
is a systemd setting or a pull request; the how-to lives in the
[configuration guide](../guides/configuration.md).

## config.yaml

Location: `<user config dir>/recdep/config.yaml`
(`~/.config/recdep/config.yaml` on Linux).

Parse rules:

- The YAML is parsed strictly. An unknown key is an error, shown once
  in the status line, and the built-ins stand.
- A non-empty `actions` list replaces the built-in action map entirely.
- An empty or absent `actions` list keeps the built-ins.
- `action` is required on every rule; a rule without it is an error.

The file holds one key, `actions`: a list of rules. A rule is a set of
matchers plus outputs, evaluated top-down against each record; the
first rule whose matchers all hold wins, and an omitted matcher matches
anything. When no rule matches, the action is `respond` with no
guidance.

### Rule fields

| Field | Type | Required | Description |
|---|---|---|---|
| `source` | string, matcher | no | Equality against the record's `[<source>]` tag on the first line, such as `github`, `slack`, `linear`. |
| `name_contains` | string, matcher | no | Substring test against the record's filename (`<UTC>-<source>-<slug>.md`), so it can match the slug. Example: `-review-requested-`. |
| `who_suffix` | string, matcher | no | Suffix test against the record's `<who>`, the author on the first line. Example: `[bot]`. |
| `author` | string, matcher | no | Equality against the whole `<who>`. Example: `alice`. |
| `url_prefix` | string, matcher | no | Plain string prefix test against the record's URL line, scheme and host included, no globs. The prefix scopes by URL shape: `https://github.com/acme/` matches a GitHub org, `https://github.com/acme/widgets/` a repo, `https://acme.enterprise.slack.com/` a Slack workspace, `https://acme.enterprise.slack.com/archives/C012345/` a channel. |
| `action` | string, output | yes | The verb written into the intent's `action` line; the drafting clerk's skill maps verbs to draft types ([speakwrite skill](../../speakwrite/SKILL.md), read from your installed copy at `~/.claude/skills/speakwrite/SKILL.md`). The shipped skill knows `review`, `vet-findings`, `pr-reply`, `slack-reply`, `linear-comment`, `respond`; any other verb works once that file says what to draft for it. |
| `guidance` | string, output | no | Default stance text prepended to the dictated stance in the intent's guidance section: the rule sets the register, the dictation refines or overrides it. Example: `professional register`. |

### Built-in action map

Applied when the file is absent or the `actions` list is empty:

| source | name contains | who suffix | action |
|---|---|---|---|
| github-review-requested | | | review |
| github | -review-requested- | | review |
| github | | [bot] | vet-findings |
| github | | | pr-reply |
| slack | | | slack-reply |
| linear | | | linear-comment |
| (anything else) | | | respond |

## minitrue.env

Location: `~/.config/minitrue.env`, plain `KEY=value` lines.

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

A timeout above 900 also needs `TimeoutStartSec` raised in the unit,
or systemd kills the run first.

## speakwrite.env

Location: `~/.config/speakwrite.env`, plain `KEY=value` lines.

| Key | Meaning | Default |
|---|---|---|
| SPEAKWRITE_AGENT | the agent binary | `claude` |
| SPEAKWRITE_PROMPT | the headless prompt | `/speakwrite draft` |
| SPEAKWRITE_ALLOWED_TOOLS | the agent's tool allowlist | the subcommand's default |
| SPEAKWRITE_TIMEOUT | seconds before the subcommand kills the run | `600` |

## thinkpol.env

Location: `~/.config/thinkpol.env`, loaded by the service unit. It
holds secrets, so `chmod 600` it.

| Key | Meaning | Needed for |
|---|---|---|
| SLACK_TOKEN | a user token with `chat:write`; posts as you | the slack-thread publisher |
| LINEAR_API_KEY | a Linear API key | the linear-issue publisher |
| SLACK_API_BASE | replaces the Slack Web API root (default `https://slack.com/api`) | testing |
| LINEAR_API_BASE | replaces the Linear API root (default `https://api.linear.app`) | testing |

The github-pr publisher uses your authenticated `gh` and needs no
entry here. A missing token fails the post gracefully: the draft
survives, the approval is consumed, `publish.log` names the reason.

## Other settings

| Setting | Where | Default |
|---|---|---|
| state root | `XDG_STATE_HOME` | `~/.local/state/recdep` |
| dictation editor | `$VISUAL`, else `$EDITOR`; values may carry flags (`code -w`) | `vi` |
| producer cadence, path-unit trigger bounds | systemd: `systemctl --user edit minitrue.timer` | 10 minutes |
