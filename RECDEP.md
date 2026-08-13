# recdep: the queue contract

telescreen consumes a filesystem queue. Anything that writes conforming
files is a valid producer: an LLM agent, a deterministic poller in Go or
shell, a webhook receiver. This document is the normative contract between
the two sides.

## Roles

- Producer (reference implementation: minitrue, a headless agent run on a
  systemd timer): polls upstream sources, writes entry files into `inbox/`,
  owns `since`.
- Consumer (telescreen, this repo): renders the queue and renames entries
  between state directories. Never touches the network, never writes entry
  content, never advances `since`.

## Layout

State root: `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`

- `since`: an ISO-8601 UTC instant, the moment the producer last polled
  through. Producer-owned. The consumer never reads or writes it.
- `inbox/`: unread entries. Presence means unread.
- `todo/`: read, still needs action.
- `waiting/`: acted on, the other side's move is pending.
- `archive/`: closed, nothing more expected.

The producer writes only into `inbox/` and creates missing directories at
startup. The consumer moves files between the four directories with plain
renames and creates missing directories at startup. Files, not sockets or
databases, are the whole interface.

## Entry files

One file per hit, named `<UTC>-<source>-<slug>.md`:

- `<UTC>` is `YYYYMMDDTHHMMSSZ` (UTC), so lexical order is time order.
- `<source>` is a short tag such as `slack`, `github`, `linear`.
- `<slug>` is a short kebab-case hint for humans reading `ls`.

Body:

```
[<source>] <who>: <one-line summary>
<link>
seen <produce-run-time>

<preview>
```

- Line 1: `[<source>]` tag, the actor, a colon, and a one-line summary.
- Line 2: the canonical URL of the triggering event.
- Line 3: `seen <ISO-8601 instant>`, when the producer observed it.
- Preview (optional): after one blank line, the triggering content quoted
  verbatim (a Slack reply, a review comment body, a Linear comment). Cap it
  at roughly 15 lines or 1000 characters and append `[...]` when truncated.

The consumer parses lines 1 and 2 for the list view and shows the full body
in the detail pane. Unknown or missing pieces degrade to empty fields, so a
minimal producer can emit only line 1 and still render.

## Producer obligations

1. Write each hit exactly once. Dedupe across polls is the producer's job
   (for example, a seen-list file next to `since`).
2. Enqueue activity strictly after `since`, then advance `since` to the
   poll's start time.
3. Skip activity authored by the watched person; the queue is for what
   others did.
4. On a partial outage (an auth-less source, a failed poll), enqueue a
   degraded entry naming the gap rather than failing silently.
5. Never modify or delete entries once written; states beyond `inbox/`
   belong to the consumer.

## Consumer obligations

1. Read and rename only; never edit entry content.
2. Never call the upstream sources; the queue is the only input.
3. Treat a failed read as a race with a concurrent move and retry on the
   next refresh.
