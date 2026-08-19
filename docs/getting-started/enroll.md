# Enroll the agents

The screen alone shows files. Records, draft reactions, and posts
come from three enrolled components: minitrue files records,
speakwrite writes drafts, thinkpol posts approved drafts. They run on
systemd user units and default to the claude CLI (thinkpol excepted;
it is plain Go). Another agent CLI works too:
[Use another agent](../guides/use-another-agent.md).

## Enroll

```
telescreen install                 # minitrue, speakwrite, thinkpol
telescreen install thinkpol        # one component
telescreen install --dry-run       # print the plan without writing
telescreen install --force         # restore the shipped skills over your edits
```

## What it writes where

| Path | Contents |
|---|---|
| `~/.claude/skills/` | the agent skills; seeds you may edit, re-installs keep your edits, `--force` restores the shipped versions |
| `~/.config/telescreen.yaml` | the pipeline config; install seeds a missing component key with `agent: claude` and never touches existing keys, `--force` included |
| `~/.config/systemd/user/` | the units; ExecStart points at the installing binary |
| `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/` | the state dirs, created 0700 |

The installer also enables the units: a timer for minitrue (every 10
minutes) and path units for speakwrite (`intents/*.intent`) and
thinkpol (`intents/*.publish`).

Editing the seeded skills is how you extend what the producer
watches: [Add a watch](../guides/add-a-watch.md). An already-installed
skill keeps the shipped text it was seeded with until you merge newer
shipped changes into your copy or restore it with `--force`.

## Identity

minitrue needs to know who you are. Create `~/.config/minitrue.env`
with plain `KEY=value` lines:

```
SLACK_USER_ID=U0000000000   # your Slack user id
GH_LOGIN=your-gh-login      # gh @me must resolve to it
LINEAR_ASSIGNEE=me
REPO=owner/repo             # the GitHub repo to scope PR watches to
```

Two more files matter, both optional at first:

- `~/.config/telescreen.yaml`: the agent choices per component
  (binary, args, instructions, allowlist, timeout, action map). The
  seeded file is a plain claude setup.
- `~/.config/thinkpol.env`: posting credentials (SLACK_TOKEN,
  LINEAR_API_KEY). Secrets; `chmod 600` it. GitHub posting uses your
  authenticated `gh` and needs no key here.

Every key and default lives in the
[Configuration reference](../reference/configuration.md); the
[Configuration guide](../guides/configuration.md) covers the tasks.

## From source

`make install` builds the binary into `~/.local/bin` and runs its
installer, for development or by choice.

## Next

[Your first record](first-record.md): the whole loop, on the demo seed.
