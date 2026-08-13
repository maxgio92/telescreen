# telescreen

A terminal dashboard for a personal notification queue: Slack thread replies,
GitHub PR activity (reviews, mentions, review requests), and Linear ticket
updates, collected by a headless producer and triaged in a bubbletea TUI.

In 1984 the telescreen watches you; this one flips the direction. A screen
in your terminal through which you watch everything said about and asked of
you.

## How it works

A producer polls Slack, GitHub, and Linear and writes one markdown file per
hit into a filesystem queue. This program is the consumer: it renders the
queue and moves entries between states with single keys. Files are the only
interface; the TUI makes no network calls.

The queue format is a producer-agnostic contract, specified in
[RECDEP.md](RECDEP.md). The reference producer, minitrue, is a headless
Claude Code skill run by a systemd user timer, but any program that writes
conforming files works: an agent with whatever model you like, a
deterministic poller, a webhook receiver.

State root: `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`

- `inbox/`: unread hits
- `todo/`: read, still needs action
- `waiting/`: acted on, the other side's move is pending
- `archive/`: closed, nothing more expected

Entry format (`<UTC>-<source>-<slug>.md`, sorts by time):

```
[<source>] <who>: <one-line summary>
<link>
seen <produce-run-time>

<preview>
```

## Install

```
make install
```

`make build` compiles the dashboard to `~/.local/bin/telescreen`.
`make minitrue` installs the reference producer from [minitrue/](minitrue/):
the wrapper script to `~/.local/bin/minitrue`, the skill directory symlinked
into `~/.claude/skills/minitrue`, and the systemd user units linked and
enabled (`minitrue.timer`, every 10 minutes). It also creates the four state
directories (`inbox/`, `todo/`, `waiting/`, `archive/`) under the state root.
Identity lives in
`~/.config/minitrue.env` (SLACK_USER_ID, GH_LOGIN, LINEAR_ASSIGNEE, REPO).

## Usage

```
telescreen          # dashboard
telescreen -once    # print per-state counts and exit
```

Keys:

- `tab`/`shift+tab`, `1`/`2`/`3`/`4`: switch view (inbox, todo, waiting, archive)
- `j`/`k`, arrows: navigate
- mouse: wheel scrolls the list, click selects a row, click a tab switches view
- `o`, `enter`: open the entry's URL
- `r`: mark read (inbox to todo)
- `w`: mark waiting (todo to waiting)
- `a`: archive (todo or waiting to archive)
- `u`: undo, one state back (archive to waiting, waiting to todo, todo to inbox)
- `y`: copy the URL (wl-copy, fallback xclip)
- `q`: quit

The TUI watches the state directories with fsnotify, so new producer entries
appear live.
