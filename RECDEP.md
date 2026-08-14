# recdep: the queue contract

telescreen consumes a filesystem queue. Anything that writes conforming
files is a valid producer: an LLM agent, a deterministic poller in Go or
shell, a webhook receiver. This document is the normative contract between
the two sides.

## Roles

- Producer (reference implementation: minitrue, a headless agent run on a
  systemd timer): polls upstream sources, writes entry files into `tube/`,
  owns `since`.
- Consumer (telescreen, this repo): renders the queue and renames entries
  between state directories. Never touches the network, never writes entry
  content, never advances `since`.

## Layout

State root: `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`

- `since`: an ISO-8601 UTC instant, the moment the producer last polled
  through. Producer-owned. The consumer never reads or writes it.
- `tube/`: landed, unseen entries. Presence means unseen.
- `desk/`: seen, still needs action.
- `upsub/`: acted on, the other side's move is pending.
- `files/`: closed, nothing more expected.
- `intents/`: speakwrite dictation intents, one file per intent named
  after the entry (`<entry-name>.intent`). Written only by the consumer
  (the TUI); the drafting runner consumes each intent and removes the
  file. Format: an `entry <absolute entry path>` line, an
  `action <mapped action>` line, then a `guidance:` section with free
  text, possibly empty. The directory also holds publish approvals
  (`<entry-name>.publish`): a single `entry <absolute entry path>` line,
  written only by the consumer after an explicit double-key approval on
  a drafted entry. The actor (thinkpol) consumes each approval, posts
  the draft, and removes the file; the approval is the recorded consent
  for the one outward write. The consumer may also remove a pending
  approval when the human discards the draft: the revocation of that
  consent.

The producer writes only into `tube/` and creates missing directories at
startup. The consumer moves files between the four directories with plain
renames and creates missing directories, `intents/` included, at startup.
Files, not sockets or databases, are the whole interface.

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
5. Entries are append-only once written, with one sanctioned addition: a
   revalidation pass may append a single marker line
   `stale <reason> <ISO-8601 time>` (kebab-case reason, e.g. `merged`,
   `closed`, `already-reviewed`) to entries in `tube/`, `desk/`, or
   `upsub/`. The marker must start on its own line: prepend a newline
   when the file lacks a trailing one. Never mark an entry twice; never
   touch `files/`. Never
   delete entries; states beyond `tube/` belong to the consumer. The
   producer marks, the human files.

## Entry marker sections

The speakwrite drafting runner records its work as marker sections
appended to entry files. Each marker starts with `--- ` on its own line,
the same discipline as the stale marker: prepend a newline when the file
lacks a trailing one.

```
--- dictated <ISO-8601 time>
<the guidance, copied from the intent>

--- draft <ISO-8601 time>
<the draft text>

--- published <ISO-8601 time> <URL>

--- discarded <ISO-8601 time>
```

The drafting runner appends dictated and draft. The actor (thinkpol)
appends published. The consumer appends only discarded: that append is
the one consumer write to entry content besides renames. These four kinds are the only recognized
markers: a `--- ` line with any other kind is section text (a quoted
diff, for example), not a marker. Markers accumulate append-only; the
last marker wins for presentation.

The published marker records the actor's publish write, the one
outward-facing action in the whole system: on a `.publish` approval the
actor (thinkpol) posts the last draft upstream through its publisher
table (THINKPOL.md), appends the published marker with the resulting
URL, moves the entry file to `upsub/` unless it already sits in
`upsub/` or `files/`, and removes the approval. That single rename is
the actor's only move between state directories; every other move
belongs to the consumer. On a failed post the actor leaves the entry
untouched and removes the approval, so the draft stays approvable and
nothing retries silently.
The drafting runner's writes are the dictated and draft sections and
the intent removals; it never posts, renames, or touches approvals.

## Consumer obligations

1. Read and rename only, with two exceptions: the incinerate removal in
   point 4 and the discarded marker append in the entry marker sections
   above. Never otherwise edit entry content. Stale markers are rendered,
   not written, by the consumer.
2. Never call the upstream sources; the queue is the only input.
3. Treat a failed read as a race with a concurrent move and retry on the
   next refresh.
4. One destructive action exists: incinerate, which removes an entry file
   from `files/` only, behind a double keypress on the same entry. The
   removal is permanent; nothing returns from the memory hole.
