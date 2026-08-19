# Configuration reference

Lookup page for everything configurable through files: the pipeline
choices in `telescreen.yaml` and the per-role env files. Everything
else is a systemd setting or a pull request; the how-to lives in the
[Configuration guide](../guides/configuration.md).

## telescreen.yaml

Location: `<user config dir>/telescreen.yaml`
(`~/.config/telescreen.yaml` on Linux).

One file for the whole pipeline, keyed by component: a `minitrue`
key, a `speakwrite` key, and an optional `thinkpol` key. The actor is
deterministic and its secrets stay in `thinkpol.env`; its key carries
publisher routing only.

Parse rules:

- The YAML is parsed strictly. An unknown key, top level or nested, is
  an error.
- A field set here wins; a field left unset falls back to the role's
  env file key, then the process environment, then the built-in
  default.
- When `telescreen.yaml` is absent and the retired
  `<user config dir>/recdep/config.yaml` exists, the old file loads
  and its `actions` list maps to `speakwrite.actions`. When both
  exist, `telescreen.yaml` wins. Move the rules over and delete the
  old file.

### minitrue

| Field | Type | Description |
|---|---|---|
| `agent` | string | The agent binary. |
| `args` | string | The argument template, split on whitespace; an element that is exactly `{prompt}` or `{tools}` becomes that value as one argument, every other element is verbatim; a template without `{tools}` leaves the allowlist unused. |
| `instructions` | string | Path (`~` expands) whose file content becomes the prompt; wins over `MINITRUE_PROMPT`. A path that is missing or unreadable fails the run naming the path. |
| `allowed_tools` | string | The agent's tool allowlist. |
| `timeout` | int | Seconds before the subcommand kills the run; must be positive when set. |

### speakwrite

| Field | Type | Description |
|---|---|---|
| `agent` | string | The agent binary. |
| `args` | string | The argument template; semantics as in the minitrue table. |
| `instructions` | string | Path (`~` expands) whose file content becomes the prompt; wins over `SPEAKWRITE_PROMPT`. A path that is missing or unreadable fails the run naming the path. |
| `allowed_tools` | string | The agent's tool allowlist. |
| `timeout` | int | Seconds before the subcommand kills the run; must be positive when set. |
| `actions` | list of rules | The dictation action map, per the [Rule fields](#rule-fields) table. A non-empty list replaces the built-in map entirely; empty or absent keeps the built-ins; `action` is required on every rule. |

The `SPEAKWRITE_*` env keys are each field's fallback, as the
`MINITRUE_*` keys are for minitrue.

A rule is a set of matchers plus outputs, evaluated top-down against
each record; the first rule whose matchers all hold wins, and an
omitted matcher matches anything. When no rule matches, the action is
`respond` with no guidance.

### Rule fields

| Field | Type | Required | Description |
|---|---|---|---|
| `source` | string, matcher | no | Equality against the record's `[<source>]` tag on the first line, such as `github`, `slack`, `linear`. |
| `name_contains` | string, matcher | no | Substring test against the record's filename (`<UTC>-<source>-<slug>.md`), so it can match the slug. Example: `-review-requested-`. |
| `who_suffix` | string, matcher | no | Suffix test against the record's `<who>`, the author on the first line. Example: `[bot]`. |
| `author` | string, matcher | no | Equality against the whole `<who>`. Example: `alice`. |
| `url_prefix` | string, matcher | no | Plain string prefix test against the record's URL line, scheme and host included, no globs. The prefix scopes by URL shape: `https://github.com/acme/` matches a GitHub org, `https://github.com/acme/widgets/` a repo, `https://acme.enterprise.slack.com/` a Slack workspace, `https://acme.enterprise.slack.com/archives/C012345/` a channel. |
| `meta` | map of string to string, matcher | no | Equality against the record's metadata lines ([Queue contract](../contracts/recdep.md#metadata)): the rule matches when every pair's value equals the record's value for that key, last duplicate wins; a record without the key never matches, no globs. A key must be lowercase `[a-z_]+` and the reserved keys (`stale`, `seen`, `path`, `url`) are rejected at load, as is an empty value. Example: `{channel: "#incidents"}`. |
| `action` | string, output | yes | The verb written into the intent's `action` line; the speakwrite skill maps verbs to draft types ([speakwrite skill](https://github.com/maxgio92/telescreen/blob/main/speakwrite/SKILL.md), read from your installed copy at `~/.claude/skills/speakwrite/SKILL.md`). The shipped skill knows `review`, `vet-findings`, `pr-reply`, `slack-reply`, `linear-comment`, `respond`; any other verb works once that file says what to draft for it. The action selects what speakwrite writes; how an approved draft is posted is chosen separately by the actor's publisher table matching the record URL ([Actor contract](../contracts/thinkpol.md#the-publisher-table)). |
| `guidance` | string, output | no | Default stance text prepended to the dictated stance in the intent's guidance section: the rule sets the register, the dictation refines or overrides it. Example: `professional register`. |

### thinkpol

One field, `publishers`: a list of routing rules for the actor's
publisher table. Rules are consulted top-down and the first match
wins; when no rule matches, the built-in URL matching runs, skipping
publishers a bare `enabled: false` rule disabled.

| Field | Type | Required | Description |
|---|---|---|---|
| `publisher` | string | yes | The backend the rule routes to: `github-pr`, `slack-thread`, `linear-issue`, or `exec`. |
| `url_prefix` | string, matcher | no | Plain string prefix test against the record URL, as in the action-map rule field; absent matches every URL. |
| `enabled` | bool | no | Defaults to true. `false` with no `url_prefix` disables the named publisher entirely, built-in matching included; a `false` rule never routes. |
| `command` | string | for exec | The exec publisher's argv template, split on whitespace; an element that is exactly `{url}` becomes the record URL as one argument, no shell. The draft arrives on stdin. Exit 0 is success; the first non-empty stdout line becomes the published permalink when it parses as a URL, else the record URL stands in. Non-zero exit fails the post with stderr's tail. The command inherits the unit's environment, `thinkpol.env` included. Forbidden on the other publishers. |

### Built-in action map

Applied when the `actions` list is empty or absent:

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

Location: `~/.config/minitrue.env`, plain `KEY=value` lines. The env
file is the home of identity and the fallback layer for the agent
keys: a `telescreen.yaml` field wins over its `MINITRUE_*` twin.

| Key | Meaning | Default |
|---|---|---|
| SLACK_USER_ID | your Slack user id, the person being watched | required |
| GH_LOGIN | your GitHub login; `gh` must resolve `@me` to it | required |
| LINEAR_ASSIGNEE | the Linear assignee to watch | `me` |
| REPO | the GitHub repo to scope PR watches to | required |
| BOT_LOGINS | bot logins to skip, besides `[bot]` suffixes | empty |
| MINITRUE_AGENT | the agent binary the subcommand runs | `claude` |
| MINITRUE_ARGS | the agent's argument template, split on whitespace; an element that is exactly `{prompt}` or `{tools}` becomes that value as one argument, every other element is verbatim; a template without `{tools}` leaves the allowlist unused | `-p {prompt} --allowedTools {tools}` |
| MINITRUE_PROMPT | the headless prompt | `/minitrue produce` |
| MINITRUE_ALLOWED_TOOLS | the agent's tool allowlist | the subcommand's default |
| MINITRUE_TIMEOUT | seconds before the subcommand kills the run | `600` |

A timeout above 900 also needs `TimeoutStartSec` raised in the unit,
or systemd kills the run first.

## speakwrite.env

Location: `~/.config/speakwrite.env`, plain `KEY=value` lines. The
fallback layer for the agent keys: a `telescreen.yaml` field wins over
its `SPEAKWRITE_*` twin.

| Key | Meaning | Default |
|---|---|---|
| SPEAKWRITE_AGENT | the agent binary | `claude` |
| SPEAKWRITE_ARGS | the agent's argument template, split on whitespace; an element that is exactly `{prompt}` or `{tools}` becomes that value as one argument, every other element is verbatim; a template without `{tools}` leaves the allowlist unused | `-p {prompt} --allowedTools {tools}` |
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
key here. A missing token fails the post gracefully: the draft
survives, the approval is consumed, `publish.log` names the reason.

## Other settings

| Setting | Where | Default |
|---|---|---|
| state root | `XDG_STATE_HOME` | `~/.local/state/recdep` |
| dictation editor | `$VISUAL`, else `$EDITOR`; values may carry flags (`code -w`) | `vi` |
| producer cadence, path-unit trigger bounds | systemd: `systemctl --user edit minitrue.timer` | 10 minutes |
