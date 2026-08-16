# Use another agent

For anyone who wants the shipped producer and speakwrite subcommands
to run an agent CLI other than claude.

Three layers, three amounts of work:

1. The contracts need nothing. The [Queue contract](../contracts/recdep.md)
   and the [Actor contract](../contracts/thinkpol.md) are plain files;
   any process that honors them participates.
2. The subcommands need two env keys. `<PREFIX>_AGENT` names the
   binary; `<PREFIX>_ARGS` is its argument template, split on
   whitespace, where an element that is exactly `{prompt}` or `{tools}`
   becomes that value as one argument and every other element is
   verbatim. The default is the claude shape,
   `-p {prompt} --allowedTools {tools}`. Omit `{tools}` when the agent
   has no allowlist concept.
3. The instructions need `<PREFIX>_PROMPT`. The shipped skills install
   to `~/.claude/skills/`, which only claude reads, so a `/minitrue
   produce` prompt means nothing to another agent. Put the full
   instructions in the prompt instead.

## Worked example: codex as the producer

`~/.config/minitrue.env`:

```
SLACK_USER_ID=U0000000000
GH_LOGIN=your-gh-login
LINEAR_ASSIGNEE=me
REPO=owner/repo
MINITRUE_AGENT=codex
MINITRUE_ARGS=exec {prompt}
MINITRUE_PROMPT="You are the producer for a personal notification queue. Poll GitHub for PRs where $GH_LOGIN is mentioned or review-requested in $REPO, Slack for replies to $SLACK_USER_ID, and Linear for tickets assigned to $LINEAR_ASSIGNEE. For each new event, file one record into ~/.local/state/recdep/tube/ in the record format defined by the telescreen queue contract (docs/contracts/recdep.md); skip events already recorded anywhere in the queue. Touch nothing else."
```

The `$GH_LOGIN` style names stay literal in the prompt: no expansion
happens. The agent sees their values because every key in the env
file is exported into its environment, and resolves them from there;
inline the literal values instead if your agent does not.

The record format the prompt must honor is the
[Queue contract](../contracts/recdep.md); point your instructions at
your local copy of it rather than restating the grammar.

The same recipe covers speakwrite with the `SPEAKWRITE_` keys in
`~/.config/speakwrite.env`.

Timeouts (`<PREFIX>_TIMEOUT`) and the logs (`produce.log`,
`draft.log` under the state root) behave identically whatever the
agent.
