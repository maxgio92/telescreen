# thinkpol: the actor contract

Read this to enroll or write an actor.

In the book the Thought Police do not deliberate; they enforce
decisions already taken. thinkpol is the actor role, defined by the
`.publish` contract rather than by any particular executable. This repo
enrolls a deterministic reference actor (ordinary Go: it never composes
a word, posts the draft verbatim); an agentic actor is equally welcome:
your own agent, your persona and tone, its MCP credentials, acting on
the approved draft. The enrolled actor declares its semantics: verbatim
(the draft is the post) or interpretive (the agent may adapt the text,
and must then record what it actually posted in the published marker's
section body, per the [Queue contract](recdep.md)). Exactly one actor
is enrolled at a time; two consumers of the same approvals would race
into double-posting.

## Why a separate binary

Publishing needs zero judgment: parse the approval, extract the draft,
post it, stamp the record. Keeping it inside the speakwrite agent made
the least deterministic component the only one with outward reach, and
its "never post without approval" rule was a sentence in a prompt.
As a separate binary it is structure: the agent has nothing to post
with, and the one component that can post is ordinary Go, unit-tested
line by line.

## Responsibilities, and what each one requires

The system is five single-responsibility components joined only by
files. Each is replaceable without touching the others.

| Role | Component | Interface it honors | Requires |
|---|---|---|---|
| produce | minitrue | writes records into `tube/` ([Queue contract](recdep.md)) | claude CLI + gh; Slack/Linear MCP, degrading without |
| queue | recdep | plain files and renames ([Queue contract](recdep.md)) | a filesystem |
| view | telescreen | reads records, renames between drawers | nothing else |
| draft | speakwrite | consumes `.intent`, appends dictated/draft markers | claude CLI + gh for research; Slack/Linear MCP optional |
| act | thinkpol | consumes `.publish`, posts, appends published, renames to upsub | per-publisher credentials (see the publisher table); no claude |

The model is optional at every role except draft. A deterministic
producer plus telescreen plus thinkpol is a complete, LLM-free system;
speakwrite is the one place judgment lives, and dictation is how the
human injects it.

## Agnosticism

Two axes, both resolved at the edges rather than in the core:

- Source-agnostic. A record carries a source tag, a URL, content, and
  markers; nothing else about Slack, GitHub, or Linear leaks into the
  queue, the view, or the flow. The three places that know sources are
  declared tables, not logic: the producer's watches (prose in its
  skill), the action map (a data table in the TUI), and the publisher
  table (`internal/publish`, a dispatch table from URL shape to posting
  call; the TUI's p gate and the actor both read it). Adding a source
  touches those three tables and nothing else.
- Technology-agnostic. Every seam is a file format, so every component
  is swappable per role: the producer can be an agent, a deterministic
  poller, or a webhook receiver; the draft role can be filled by any
  agent runtime that reads intents and appends markers; the actor can
  be this Go binary, a shell script, or anything that honors the
  approval semantics. The contract files (the
  [Queue contract](recdep.md), [Design: speakwrite](../design/speakwrite.md),
  this one) are the only coupling.

### Configuration

Environment details live outside the code, split by shape.
`~/.config/telescreen.yaml` holds the per-component choices, the agent
binary, instructions, MCP tool allowlists, and the action map
included. Env files (`~/.config/minitrue.env`,
`~/.config/speakwrite.env`, `~/.config/thinkpol.env`) hold identity
and secrets, and act as the fallback layer for the agent keys.
Systemd enrollment chooses implementations: swapping a component means
enabling a different unit. The subcommand defaults reproduce a plain
claude setup, so an absent config is a working one.

## Procedure

Triggered by a systemd path unit on `recdep/intents/*.publish`
(trigger-bounded like the others). For each approval, oldest first:

1. Parse the `entry <absolute path>` line. Resolve the record: the
   recorded path, else search the four drawers for the basename, else
   remove the approval and log the orphan.
2. Extract the last draft marker. No draft, or a discarded marker
   after it: remove the approval, log why, touch nothing.
3. Dispatch on the URL through the publisher table below. A URL no
   publisher matches: remove the approval, log why.
4. On success: append `--- published <ISO-8601 time> <comment URL>` to
   the record (contract newline discipline), rename the record to
   `upsub/` unless it already sits in `upsub/` or `files/`, remove the
   approval, log one line.
5. On failure: leave the record untouched (the draft tag survives, the
   human can re-approve), remove the approval so nothing retries
   silently, log the error.

Logs append to `recdep/publish.log`, one line per approval, so a
disappeared approval is always explained somewhere.

## The publisher table

`internal/publish` declares the table; the TUI's p gate and the actor
both read it, so the view offers exactly what the actor can do. The
table is extensible by configuration: `thinkpol.publishers` in
`telescreen.yaml` routes URL prefixes to a named publisher, disables
one, or defines an exec backend (a command getting the record URL as
an argument and the draft on stdin) for custom targets. Rules apply in
order, first match wins; the built-in matching below is the fallback
when no rule matches. The obligations are unchanged: whatever takes
the URL posts the draft, returns the permalink, and honors the
procedure above.

| Publisher | URL shape | Posts via | Requires |
|---|---|---|---|
| github-pr | `github.com/<owner>/<repo>/pull/<n>` | `gh pr comment <n> --repo <o/r> --body-file -` | gh, authenticated |
| slack-thread | `https://<workspace>.slack.com/archives/<CHANNEL>/p<digits>` | Slack Web API `chat.postMessage` | `SLACK_TOKEN`, a user token with `chat:write` |
| linear-issue | `https://linear.app/<workspace>/issue/<KEY>-<n>` | Linear GraphQL `commentCreate` (issue id resolved by the `issue` query, which accepts the `KEY-<n>` identifier) | `LINEAR_API_KEY` |

The Slack thread timestamp comes from the URL's `thread_ts` query when
present, else from the `p<digits>` segment itself (a dot before the
last six digits), so a bare message URL replies to that message as the
thread root. `SLACK_TOKEN` is a user token on purpose: it posts as the
human, which matches drafts written in first person.

The service unit loads the tokens from `~/.config/thinkpol.env`
(`EnvironmentFile=-`, so a missing file is fine, gh alone still works).
The file holds secrets; `chmod 600` it.
