# Use another agent

For anyone who wants the shipped producer and speakwrite subcommands
to run an agent CLI other than claude.

Three layers, three amounts of work:

1. The contracts need nothing. The [Queue contract](../contracts/recdep.md)
   and the [Actor contract](../contracts/thinkpol.md) are plain files;
   any process that honors them participates.
2. The subcommands need two `telescreen.yaml` fields. `agent` names
   the binary; `args` is its argument template, split on whitespace,
   where an element that is exactly `{prompt}` or `{tools}` becomes
   that value as one argument and every other element is verbatim. The
   default is the claude shape,
   `-p {prompt} --allowedTools {tools}`. Omit `{tools}` when the agent
   has no allowlist concept.
3. The instructions need `instructions`: a file whose content becomes
   the prompt. The shipped skills install to `~/.claude/skills/`,
   which only claude reads, so a `/minitrue produce` prompt means
   nothing to another agent. Put the full instructions in that file
   instead.

## Worked example: codex as the producer

`~/.config/telescreen.yaml`:

```yaml
minitrue:
  agent: codex
  args: exec {prompt}
  instructions: ~/prompts/produce.md
```

`~/prompts/produce.md`:

```
You are the producer for a personal notification queue. Poll GitHub
for PRs where $GH_LOGIN is mentioned or review-requested in $REPO,
Slack for replies to $SLACK_USER_ID, and Linear for tickets assigned
to $LINEAR_ASSIGNEE. For each new event, file one record into
~/.local/state/recdep/tube/ in the record format defined by the
telescreen queue contract (docs/contracts/recdep.md); skip events
already recorded anywhere in the queue. Touch nothing else.
```

`~/.config/minitrue.env` shrinks to identity:

```
SLACK_USER_ID=U0000000000
GH_LOGIN=your-gh-login
LINEAR_ASSIGNEE=me
REPO=owner/repo
```

The `$GH_LOGIN` style names stay literal in the instructions file: no
expansion happens. The agent sees their values because every key in
the env file is exported into its environment, and resolves them from
there; inline the literal values instead if your agent does not.

The record format the instructions must honor is the
[Queue contract](../contracts/recdep.md); point your instructions at
your local copy of it rather than restating the grammar.

The same recipe covers speakwrite under the `speakwrite` key.

Timeouts (`timeout`) and the logs (`produce.log`, `draft.log` under
the state root) behave identically whatever the agent.
