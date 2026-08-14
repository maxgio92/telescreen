# telescreen

A screen for monitoring your daily job. Yes, Smith, I am looking at you.

The telescreen in the book received and transmitted simultaneously, and
there was no way of shutting it off. This one differs on a single point
of doctrine: it works for you. Slack thread replies, GitHub reviews,
mentions and review requests, Linear tickets; everything said about you
and asked of you, watched, filed, and displayed. There is always an eye
on your work. Now it is yours.

## The apparatus

The Ministry of Truth (`minitrue/`, a headless agent on a systemd user
timer) manufactures the records: it polls Slack, GitHub, and Linear and
files one markdown record per event. The Records Department (`recdep`,
a directory of plain files) holds them in four drawers: `inbox/` for
the unseen, `ack/` for your turn, `waiting/` for their turn, and
`archive/` for the closed. This program is the screen itself: it renders
the records and moves them between drawers on single keys. It never
touches the network. Files are the only interface, and the file is the
only truth.

The drawer layout is a contract, [RECDEP.md](RECDEP.md), and any
producer that writes conforming records is a loyal citizen: an agent
with whatever model you like, a deterministic poller, a webhook
receiver. The Party is not particular about who writes history, only
about the format.

Record format (`<UTC>-<source>-<slug>.md`, chronology by filename):

```
[<source>] <who>: <one-line summary>
<link>
seen <produce-run-time>

<preview>
```

Records rot. When a PR merges behind your back or you already filed the
review, the Ministry's revalidation pass stamps the record
(`stale <reason> <time>`); the screen dims it and sinks it below the
fresh ones. The producer stamps, you archive. Nobody rewrites history
here, which admittedly is where we diverge from the source material.

And at the end of the row of drawers there is a slit in the wall. The
fifth view is the memory hole: permanently empty, as intended. Press `x`
on an archived record and the screen will challenge you by name; press
it again and the record rides the warm draft to the incinerators.
Nothing returns. The past was erased, the erasure was forgotten.

A drafting layer, the speakwrite (`speakwrite/`, a headless clerk
behind a systemd path unit on `recdep/intents/`): press `s` on a record
to dictate your stance, and the clerk researches the matter and drafts
the response into the record for your explicit approval. Press `p`
twice on the draft and the clerk posts it upstream (GitHub for now),
stamps the record with the comment URL, and files it under waiting;
press `D` to discard the draft into the record's history instead.
Doctrine in [SPEAKWRITE.md](SPEAKWRITE.md).

## Requisition

```
make install
```

`make build` compiles the screen to `~/.local/bin/telescreen`.
`make minitrue` enrolls the Ministry: the wrapper to
`~/.local/bin/minitrue`, the skill symlinked into
`~/.claude/skills/minitrue`, the systemd user units linked and enabled
(`minitrue.timer`, every 10 minutes). Your identity papers live in
`~/.config/minitrue.env` (SLACK_USER_ID, GH_LOGIN, LINEAR_ASSIGNEE,
REPO). `make speakwrite` enrolls the drafting clerk the same way: the
wrapper to `~/.local/bin/speakwrite`, the skill symlinked into
`~/.claude/skills/speakwrite`, and `speakwrite.path` watching
`recdep/intents/` so a saved dictation or a publish approval wakes the
runner.

## Operation

```
telescreen          # the screen
telescreen -once    # print per-drawer counts and exit
```

Keys:

- `tab`/`shift+tab`, `1`-`5`: switch view (inbox, ack, waiting,
  archive, memoryhole)
- `j`/`k`, arrows, mouse wheel: navigate; click selects a row or a tab
- `o`, `enter`: open the record's URL
- `y`: copy the URL (wl-copy, fallback xclip)
- `r`: acknowledge (inbox to ack, seen and mine)
- `w`: their move now (ack to waiting)
- `a`: file it (ack or waiting to archive)
- `u`: unfile, one drawer back (the Party admits no mistakes; you may)
- `s`: dictate into the speakwrite (inbox, ack, waiting): edit a
  pre-filled intent in `$VISUAL`/`$EDITOR`; saving submits it to
  `intents/`, emptying the file or aborting cancels
- `p` `p`: publish a draft (GitHub records only): the first press names
  the target, the second files the approval; the clerk does the posting,
  the screen never touches the network
- `D`: discard a draft: the record keeps the history, the `[draft]` tag
  goes away
- `x` `x`: the memory hole (archive only, and the screen will bark
  first)
- `q`: switch off the telescreen, a luxury Smith never had

New records appear on the screen the moment the Ministry files them
(fsnotify). The detail pane shows the record in full: content, preview,
the file's absolute path (one `cat` away, or one agent handle), the
link, and when it was seen.

Under no circumstances does this screen watch you back. That would be
doubleplusungood.
