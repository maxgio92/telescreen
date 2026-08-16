# Configuration

For operators who want to change how the screen behaves: this page
tells you which file to edit for which wish, with one worked example
per task. Every key and field is defined in the
[Configuration reference](../reference/configuration.md).

## Which layer to change

Three layers, from a file edit to a pull request:

| Layer | What lives there | How to change it |
|---|---|---|
| configuration | the per-component choices in `telescreen.yaml` (agent, args, instructions, allowlist, timeout, action map); identity and secrets in the env files; cadence | edit the files below; no rebuild |
| enrollment | which program plays each role: producer, speakwrite agent, actor | enroll your own unit in place of a shipped one; the [Queue contract](../contracts/recdep.md) is the interface |
| the binary | the drawer names and record grammar, the keys, the double-key approval, the publisher match rules (which URL goes to GitHub, Slack, Linear) | a pull request to this repo |

Choosing an implementation is never a config key: you enroll a unit.
The last row is deliberately fixed: the grammar is what makes every
component replaceable, so it moves by PR, not per user.

## One pane of glass

`~/.config/telescreen.yaml` holds the choices per component: the
`minitrue` and `speakwrite` keys each carry the agent, its args
template, an instructions file, the allowlist, and the timeout;
`speakwrite` also carries the action map. `telescreen install` seeds
the file and never overwrites your edits: a re-install only appends a
component key you deleted. The env files stay for identity and
secrets, and their agent keys work as the fallback layer when a field
is unset in the YAML.

A complete file, every key in use:

```yaml
minitrue:
  agent: codex                       # any agent CLI; claude is the default
  args: exec {prompt}                # its argv shape; {prompt} lands as one argument
  instructions: ~/notes/producer.md  # file content becomes the prompt
  timeout: 900                       # seconds before the run is killed
speakwrite:
  agent: claude                      # claude reads the installed skill, so no
  allowed_tools: mcp__github mcp__slack   # instructions field is needed here
  actions:                           # replaces the built-in action map entirely
    - url_prefix: https://github.com/acme/
      action: review
      guidance: professional register
    - source: slack
      action: slack-reply
```

Every field is optional except `action` inside a rule; an absent field
falls back to the env-file key, then the default. Field semantics:
[Configuration reference](../reference/configuration.md#telescreenyaml).

## Add a rule to the action map

Edit `~/.config/telescreen.yaml` under `speakwrite.actions`. A
non-empty `actions` list replaces the built-ins entirely, so bring
every rule you still want. The rule fields, the match order, and the
built-in map are defined in the
[Configuration reference](../reference/configuration.md#telescreenyaml).

Example: two Slack workspaces, two registers.

```yaml
speakwrite:
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

## Change the agent or the prompt

The agent binary and its instructions are `telescreen.yaml` fields:
set `agent` and `instructions` under `minitrue` for the producer,
under `speakwrite` for the speakwrite agent. `instructions` names a
file whose content becomes the prompt. Use an absolute path when the
binary is not on the unit's PATH, and carry your environment's MCP
tool identifiers in `allowed_tools`. All fields, the env fallback
keys, and defaults:
[Configuration reference](../reference/configuration.md#telescreenyaml).
A CLI with different flags than claude also needs the `args` template;
the worked example lives in [Use another agent](use-another-agent.md).

The prompts themselves are skills. `telescreen install` writes seeds
under `~/.claude/skills/`; the agents read the installed files at run
time, so editing `~/.claude/skills/speakwrite/SKILL.md` changes how
drafts are written without touching Go. Re-installs keep your edits
(`--force` restores the shipped versions).

New action verbs live in the same file: speakwrite drafts whatever the
skill says a verb means, so a custom `summarize` action works as soon
as your speakwrite skill defines it.

## Set credentials

The actor's tokens live in `~/.config/thinkpol.env`; `chmod 600` it.
The github-pr publisher uses your authenticated `gh` and needs no
token there. Keys and failure behavior:
[Configuration reference](../reference/configuration.md#thinkpolenv).

## Change the cadence

The producer cadence and the path units' trigger bounds are systemd
settings: `systemctl --user edit minitrue.timer`, not a config file.
