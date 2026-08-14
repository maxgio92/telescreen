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
| act | thinkpol | consumes `.publish`, posts, appends published, renames to upsub | gh, authenticated; no claude |

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
  skill), the dictation action map (a data table in the TUI), and
  thinkpol's publishers (a dispatch table from URL shape to posting
  command). Adding a source touches those three tables and nothing
  else.
- Technology-agnostic. Every seam is a file format, so every component
  is swappable per role: the producer can be an agent, a deterministic
  poller, or a webhook receiver; the drafting layer can be any agent
  runtime that reads intents and appends sections; the actor can be
  this Go binary, a shell script, or anything that honors the approval
  semantics. The contract files (RECDEP.md, SPEAKWRITE.md, this one)
  are the only coupling.

## Procedure

Triggered by a systemd path unit on `recdep/intents/*.publish`
(trigger-bounded like the others). For each approval, oldest first:

1. Parse the `entry <absolute path>` line. Resolve the entry: the
   recorded path, else search the four drawers for the basename, else
   remove the approval and log the orphan.
2. Extract the last draft section. No draft, or a discarded marker
   after it: remove the approval, log why, touch nothing.
3. Dispatch on the URL. v1 ships one publisher: a github.com pull
   request URL posts the draft via `gh pr comment <n> --repo <o/r>
   --body-file -`. Any other URL: remove the approval, log why. The
   dispatch table is where Slack and Linear publishers land later.
4. On success: append `--- published <ISO-8601 time> <comment URL>` to
   the entry (contract newline discipline), rename the entry to
   `upsub/` unless it already sits in `upsub/` or `files/`, remove the
   approval, log one line.
5. On failure: leave the entry untouched (the draft tag survives, the
   human can re-approve), remove the approval so nothing retries
   silently, log the error.

Logs append to `recdep/publish.log`, one line per approval, so a
disappeared approval is always explained somewhere.

## What changes elsewhere

- speakwrite loses its publish procedure and its `.publish` glob; its
  path unit narrows back to `*.intent`. Its guardrail becomes literal:
  it cannot post.
- RECDEP.md reassigns the publish write and the single upsub rename
  from "the runner" to the actor.
- The TUI is untouched: `p p` writes the same approval file, `D` still
  revokes it.
