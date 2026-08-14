# thinkpol: the acting layer

Status: implemented (`cmd/thinkpol`, behind the `thinkpol/` path unit).

In the book the Thought Police do not deliberate; they enforce decisions
already taken. That is exactly this component: a small deterministic
binary that executes recorded approvals. It never composes a word and
never judges a draft; the human decided (the `.publish` approval), the
speakwrite drafted, thinkpol acts.

## Why a separate binary

Publishing needs zero judgment: parse the approval, extract the draft,
post it, stamp the record. Keeping it inside the speakwrite agent made
the least deterministic component the only one with outward reach, and
its "never post without approval" rule was a sentence in a prompt.
After the split it is structure: the agent has nothing to post with,
and the one component that can post is ordinary Go, unit-tested line by
line.

## Responsibilities, and what each one requires

The system is five single-responsibility components joined only by
files. Each is replaceable without touching the others.

| Role | Component | Interface it honors | Requires |
|---|---|---|---|
| produce | minitrue | writes records into `tube/` (RECDEP.md) | claude CLI + gh; Slack/Linear MCP, degrading without |
| store | recdep | plain files and renames (RECDEP.md) | a filesystem |
| view | telescreen | reads records, renames between drawers | nothing else |
| draft | speakwrite | consumes `.intent`, appends dictated/draft sections | claude CLI + gh for research; Slack/Linear MCP optional |
| act | thinkpol | consumes `.publish`, posts, appends published, renames to upsub | per-publisher credentials (see the publisher table); no claude |

The model is optional at every layer except drafting. A deterministic
producer plus telescreen plus thinkpol is a complete, LLM-free system;
the speakwrite is the one place judgment lives, and dictation is how
the human injects it.

## Agnosticism

Two axes, both resolved at the edges rather than in the core:

- Source-agnostic. A record carries a source tag, a URL, content, and
  markers; nothing else about Slack, GitHub, or Linear leaks into the
  store, the view, or the flow. The three places that know sources are
  declared tables, not logic: the producer's watches (prose in its
  skill), the dictation action map (a data table in the TUI), and the
  publisher table (`internal/publish`, a dispatch table from URL shape
  to posting call; the TUI's p gate and the actor both read it). Adding
  a source touches those three tables and nothing else.
- Technology-agnostic. Every seam is a file format, so every component
  is swappable per role: the producer can be an agent, a deterministic
  poller, or a webhook receiver; the drafting layer can be any agent
  runtime that reads intents and appends sections; the actor can be
  this Go binary, a shell script, or anything that honors the approval
  semantics. The actor needs no model at all; the model is optional
  everywhere except drafting. The contract files (RECDEP.md,
  SPEAKWRITE.md, this one) are the only coupling.

### Known impurities

One place falls short of the principle today, named here so it reads
as debt rather than design:

1. The two agent wrappers hardcode this machine's MCP tool identifiers
   in their allowlists. The identifiers are an environment detail, not
   a contract term; they belong in the identity config next to the
   Slack and GitHub handles, with the wrapper defaults as fallback.
   Until then the wrappers are a reference implementation for exactly
   one setup.

## Procedure

Triggered by a systemd path unit on `recdep/intents/*.publish`
(trigger-bounded like the others). For each approval, oldest first:

1. Parse the `entry <absolute path>` line. Resolve the entry: the
   recorded path, else search the four drawers for the basename, else
   remove the approval and log the orphan.
2. Extract the last draft section. No draft, or a discarded marker
   after it: remove the approval, log why, touch nothing.
3. Dispatch on the URL through the publisher table below. A URL no
   publisher matches: remove the approval, log why.
4. On success: append `--- published <ISO-8601 time> <comment URL>` to
   the entry (contract newline discipline), rename the entry to
   `upsub/` unless it already sits in `upsub/` or `files/`, remove the
   approval, log one line.
5. On failure: leave the entry untouched (the draft tag survives, the
   human can re-approve), remove the approval so nothing retries
   silently, log the error.

Logs append to `recdep/publish.log`, one line per approval, so a
disappeared approval is always explained somewhere.

## The publisher table

`internal/publish` declares the table; the TUI's p gate and the actor
both read it, so the view offers exactly what the actor can do.

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

## What changes elsewhere

- speakwrite loses its publish procedure and its `.publish` glob; its
  path unit narrows back to `*.intent`. Its guardrail becomes literal:
  it cannot post.
- RECDEP.md reassigns the publish write and the single upsub rename
  from "the runner" to the actor.
- The TUI is untouched: `p p` writes the same approval file, `D` still
  revokes it.
