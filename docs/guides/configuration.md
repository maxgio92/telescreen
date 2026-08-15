# Configuration

For operators who want to change how the screen behaves: this page
tells you which file to edit for which wish, with one worked example
per task. Every key and field is defined in the
[configuration reference](../reference/configuration.md).

## Which layer to change

Three layers, from a file edit to a pull request:

| Layer | What lives there | How to change it |
|---|---|---|
| configuration | the action map, identity, secrets, agent binary, prompts, allowlists, timeouts, cadence | edit the files below; no rebuild |
| enrollment | which program plays each role: producer, drafting clerk, actor | enroll your own unit in place of a shipped one; the [recdep.md](../contracts/recdep.md) contract is the interface |
| the binary | the drawer names and record grammar, the keys, the double-key approval, the publisher match rules (which URL goes to GitHub, Slack, Linear) | a pull request to this repo |

Choosing an implementation is never a config key: you enroll a unit.
The last row is deliberately fixed: the grammar is what makes every
component replaceable, so it moves by PR, not per user.

## Add a rule to the action map

Edit `~/.config/recdep/config.yaml`. A rule pairs matchers with an
action verb and optional guidance; rules match top-down, first match
wins. A non-empty `actions` list replaces the built-ins entirely, so
bring every rule you still want. Full field definitions and the
built-in map: [config.yaml reference](../reference/configuration.md#configyaml).

Example: two Slack workspaces, two registers.

```yaml
actions:
  - url_prefix: https://acme.enterprise.slack.com/   # the work workspace
    action: slack-reply
    guidance: professional register
  - url_prefix: https://friends.slack.com/           # the personal workspace
    action: slack-reply
    guidance: casual register, first names
  - source: slack            # any other workspace
    action: slack-reply
  - source: github           # keep the built-in GitHub routing
    name_contains: -review-requested-
    action: review
  - source: github
    who_suffix: "[bot]"
    action: vet-findings
  - source: github
    action: pr-reply
  - source: linear
    action: linear-comment
```

The rule's guidance is prepended to the stance you dictate: the rule
sets the register, your dictation refines or overrides it.

## Change the agent or the prompt

The agent binary and its headless prompt are env-file keys: set
`MINITRUE_AGENT` and `MINITRUE_PROMPT` in `~/.config/minitrue.env`
for the producer, the `SPEAKWRITE_*` twins in
`~/.config/speakwrite.env` for the drafting clerk. Use an absolute
path when the binary is not on the unit's PATH, and carry your
environment's MCP tool identifiers in the allowlist key. All keys and
defaults: [env file reference](../reference/configuration.md#minitrueenv).

The prompts themselves are skills. `telescreen install` writes seeds
under `~/.claude/skills/`; the agents read the installed files at run
time, so editing `~/.claude/skills/speakwrite/SKILL.md` changes how
drafts are written without touching Go. Re-installs keep your edits
(`--force` restores the shipped versions).

New action verbs live in the same file: the clerk drafts whatever the
skill says a verb means, so a custom `summarize` action works as soon
as your speakwrite skill defines it.

## Set credentials

The actor's tokens live in `~/.config/thinkpol.env`; `chmod 600` it.
The github-pr publisher uses your authenticated `gh` and needs no
token there. Keys and failure behavior:
[thinkpol.env reference](../reference/configuration.md#thinkpolenv).

## Change the cadence

The producer cadence and the path units' trigger bounds are systemd
settings: `systemctl --user edit minitrue.timer`, not a config file.
