# recdep: the queue contract

Read this to write a producer or a consumer.

telescreen consumes a filesystem queue. Anything that writes conforming
records is a valid producer: an LLM agent, a deterministic poller in Go
or shell, a webhook receiver. This document is the normative contract
between the two sides. Terms are defined in the
[Vocabulary](../reference/vocabulary.md).

## Roles

- Producer (reference implementation: minitrue, a headless agent run on
  a systemd timer): polls upstream sources, writes records into
  `tube/`, owns `since`.
- Consumer (telescreen, this repo): renders the queue and renames
  records between drawers. Never touches the network, never writes
  record content, never advances `since`.

## Layout

State root: `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`

| Path | Contents |
|---|---|
| `since` | An ISO-8601 UTC instant, the moment the producer last polled through. Producer-owned; the consumer never reads or writes it. |
| `tube/` | Landed, unseen records. Presence means unseen. |
| `desk/` | Seen, still needs action. |
| `upsub/` | Acted on, the other side's move is pending. |
| `files/` | Closed, nothing more expected. |
| `intents/` | Intents and approvals, written only by the consumer (the TUI). |

An intent (`<record-name>.intent`) records one dictation: an
`entry <absolute record path>` line, an `action <mapped action>` line,
then a `guidance:` section with free text, possibly empty. The
speakwrite agent consumes each intent and removes the file.

An approval (`<record-name>.publish`) is a single
`entry <absolute record path>` line, written only after an explicit
double-key approval on a drafted record: the recorded consent for the
one outward write. The actor (thinkpol) consumes each approval, posts
the draft, and removes the file. The consumer may also remove a pending
approval when the human discards the draft: the revocation of that
consent.

The producer writes only into `tube/` and creates missing directories
at startup. The consumer moves files between the four drawers with
plain renames and creates missing directories, `intents/` included, at
startup. Files, not sockets or databases, are the whole interface.

The queue is private to the user: components create directories 0700
and new files 0600 (recommended for existing ones too; `telescreen
verify` warns otherwise but never chmods). Encryption at rest is a
deployment choice (full-disk encryption, or a gocryptfs/fscrypt mount
of the state root) rather than a component concern, because append-only
plaintext is what keeps the queue auditable by `cat`.

## Records

One file per hit, named `<UTC>-<source>-<slug>.md`:

- `<UTC>` is `YYYYMMDDTHHMMSSZ` (UTC), so lexical order is time order.
- `<source>` is a short tag such as `slack`, `github`, `linear`.
- `<slug>` is a short kebab-case hint for humans reading `ls`.

Body:

```
[<source>] <who>: <one-line summary>
<link>
seen <produce-run-time>
<key> <value>

<preview>
```

- Line 1: `[<source>]` tag, the author, a colon, and a one-line summary.
- Line 2: the canonical URL of the triggering event.
- Line 3: `seen <ISO-8601 instant>`, when the producer observed it.
- Metadata (optional): zero or more `<key> <value>` lines between the
  seen line and the blank line, written with the record.
- Preview (optional): after one blank line, the triggering content
  quoted verbatim (a Slack reply, a review comment body, a Linear
  comment). Cap it at roughly 15 lines or 1000 characters and append
  `[...]` when truncated.

The consumer parses lines 1 and 2 for the list view and shows the full
body in the detail pane. Unknown or missing pieces degrade to empty
fields, so a minimal producer can emit only line 1 and still render.

### Metadata

A metadata line is one structured provider fact: a lowercase `[a-z_]+`
key, a single space, and a non-empty value running to the end of the
line. The recommended keys per source:

| Source | Keys |
|---|---|
| `github` | `org` (the owner), `repo` (the name) |
| `slack` | `channel` (the `#` name), or `dm` (comma-separated participants) |
| `linear` | `project`, `ticket` (the KEY, e.g. `FUL-1`) |

Unknown keys are allowed; consumers ignore what they do not know.
`stale`, `seen`, `path`, and `url` are reserved: `stale` for the stale
marker below, the other three because the detail view renders labeled
lines with those names and a metadata twin would be indistinguishable
from the real one. Records without metadata stay valid.

Metadata sits after the seen line and before the blank line, never at
the end of the file: the stale marker and the speakwrite markers are
appended to the end (see below), so the append-only stamping can never
land inside or corrupt the metadata region. On a record with no
preview, an appended stale line follows the metadata directly; it ends
the region rather than joining it.

## Producer obligations

1. Write each hit exactly once. Dedupe across polls is the producer's
   job (for example, a seen-list file next to `since`).
2. File activity strictly after `since`, then advance `since` to the
   poll's start time.
3. Skip activity authored by the watched person; the queue is for what
   others did.
4. On a partial outage (an auth-less source, a failed poll), file a
   degraded record naming the gap rather than failing silently.
5. Records are append-only once written, with one sanctioned addition:
   a revalidation pass may append a single marker line
   `stale <reason> <ISO-8601 time>` (kebab-case reason, e.g. `merged`,
   `closed`, `already-reviewed`) to records in `tube/`, `desk/`, or
   `upsub/`. The marker must start on its own line: prepend a newline
   when the file lacks a trailing one. Never mark a record twice; never
   touch `files/`. Never delete records; drawers beyond `tube/` belong
   to the consumer. The producer marks, the human files.

## Markers

A marker is an appended section in a record. Each marker starts with
`--- ` on its own line, the same discipline as the stale marker:
prepend a newline when the file lacks a trailing one.

```
--- dictated <ISO-8601 time>
<the guidance, copied from the intent>

--- draft <ISO-8601 time>
<the draft text>

--- published <ISO-8601 time> <URL>

--- discarded <ISO-8601 time>
```

| Marker | Appended by | Meaning |
|---|---|---|
| `dictated` | the speakwrite agent | The guidance from the consumed intent, kept so the draft can be audited against what was asked. |
| `draft` | the speakwrite agent | The draft text. A new draft supersedes earlier ones. |
| `published` | the actor | The post happened; the URL is the resulting permalink. May carry a section body: the text actually posted, required whenever the enrolled actor adapted the draft rather than posting it verbatim, so the record never lies about what went out. A verbatim actor omits it (the draft is the post). |
| `discarded` | the consumer | The draft is rejected. This append is the one consumer write to record content besides renames. |

These four kinds are the only recognized markers: a `--- ` line with
any other kind is section text (a quoted diff, for example), not a
marker. Markers accumulate append-only; the last marker wins for
presentation.

The published marker records the actor's publish write, the one
outward-facing action in the whole system. The publish procedure, the
actor's single rename to `upsub/`, and the failure behavior (record
untouched, approval removed, nothing retries silently) are normative in
the [Actor contract](thinkpol.md); every other move between drawers
belongs to the consumer. The speakwrite agent's writes are the dictated
and draft markers and the intent removals; it never posts, renames, or
touches approvals.

## Consumer obligations

1. Read and rename only, with two exceptions: the delete in point 4 and
   the discarded marker append above. Never otherwise edit record
   content. Stale markers are rendered, not written, by the consumer.
2. Never call the upstream sources; the queue is the only input.
3. Treat a failed read as a race with a concurrent move and retry on
   the next refresh.
4. One destructive action exists: delete, which removes a record from
   `files/` only, behind a double keypress on the same record. The
   removal is permanent; nothing returns from the memory hole.
