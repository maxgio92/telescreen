# Enroll the agents

The screen alone shows files. The feed, the drafting, and the posting
come from three enrolled components: minitrue (produces), speakwrite
(drafts), thinkpol (posts). They run on systemd user units and require
the claude CLI (thinkpol excepted; it is plain Go).

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
| `~/.config/systemd/user/` | the units; ExecStart points at the installing binary |
| `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/` | the state dirs, created 0700 |

The installer also enables the units: a timer for minitrue (every 10
minutes) and path units for speakwrite (`intents/*.intent`) and
thinkpol (`intents/*.publish`).

## Identity

minitrue needs to know who you are. Create `~/.config/minitrue.env`
with plain `KEY=value` lines:

```
SLACK_USER_ID=U0000000000   # your Slack user id
GH_LOGIN=your-gh-login      # gh @me must resolve to it
LINEAR_ASSIGNEE=me
REPO=owner/repo             # the GitHub repo to scope PR watches to
```

Two more env files exist, both optional at first:

- `~/.config/speakwrite.env`: the drafting agent's binary, prompt,
  allowlist, timeout. Absent means a plain claude setup.
- `~/.config/thinkpol.env`: posting credentials (SLACK_TOKEN,
  LINEAR_API_KEY). Secrets; `chmod 600` it. GitHub posting uses your
  authenticated `gh` and needs no entry.

Every key, default, and knob lives in the
[configuration guide](../guides/configuration.md).

## From source

`make install` builds the binary into `~/.local/bin` and runs its
installer, for development or by choice.

## Next

[Your first record](first-record.md): the whole loop, on the demo seed.
