---
name: speakwrite
description: The drafting runner for the telescreen queue. Consume speakwrite dictation intents from recdep/intents/, research the entry read-only (the PR, the thread, the ticket), and append a dictated and a draft marker section to the entry file for human review in the telescreen TUI. Use when asked to run the speakwrite, drain dictation intents, draft the queued responses, or when a headless run invokes /speakwrite draft.
---

# speakwrite

The drafting half of the flow defined in docs/design/speakwrite.md of
github.com/maxgio92/telescreen. The TUI's `s` key writes an intent file;
this runner consumes it, researches the entry, and appends the draft to
the entry file. Publishing belongs to thinkpol, the deterministic actor
defined in docs/contracts/thinkpol.md; this runner has no publish procedure. The queue
and marker contract is normative in docs/contracts/recdep.md; this skill is one runner
implementation, and any program that appends conforming marker sections
is a drop-in replacement.

`draft` (the first arg, default, headless) drains `intents/`: the
drafting pass over `*.intent` files.

## Paths

State root: `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`

- `intents/`: one `<entry-name>.intent` file per dictation, written by
  the TUI. `.intent.tmp` files are dictations still open in the editor;
  leave them alone. `.publish` files are approvals for thinkpol; leave
  them alone too.
- `tube/`, `desk/`, `upsub/`, `files/`: the entry state dirs. The
  intent names the entry's absolute path at dictation time.

## Intent format

```
entry <absolute entry path>
action <mapped action>

guidance:
<free text, possibly empty>
```

Empty guidance means the action's defaults.

## draft (headless, read-only research)

For each `intents/*.intent`, oldest first:

1. Parse the entry path, action, and guidance. If the entry file is
   gone from the recorded path, look for the same filename in the other
   state dirs (the human moved it); when it exists nowhere, remove the
   intent, log one line to stdout naming the orphaned intent and the
   missing entry path, and continue.
2. Research read-only, scoped by the entry's URL and source: `gh pr
   view`, `gh pr diff`, and the review threads for GitHub;
   `slack_read_thread` for Slack; the issue and its comments for
   Linear. Read the entry body too: earlier dictated and draft sections
   are prior rounds, and the new draft supersedes them. When a research
   tool is unavailable or unauthenticated, draft from the entry body
   alone, say so inside the draft, and still remove the intent; never
   leave the intent behind.
3. Compose the draft per the action, following the guidance:

   | Action | Draft |
   |---|---|
   | `review` | the review for the PR (verdict plus the comments) |
   | `vet-findings` | per-finding replies, each vetted against the code |
   | `pr-reply` | the reply to the comment or thread |
   | `slack-reply` | the response in the thread |
   | `linear-comment` | the comment on the ticket |
   | `respond` | a plain response to the entry's content |

   Write concise and kind, in the first person, following the guidance
   stance exactly; never invent positions the guidance does not take.
   For `vet-findings`, each reply is either agree plus what changes, or
   polite pushback with the reason. Write GitHub drafts in the
   pr-review-message register and Slack drafts in the slack-message
   register when those skills are available.

   When the stance names an agent (an @name the runtime resolves; for
   claude, a subagent under `~/.claude/agents`), run it via the Task
   tool against the entry's subject (the PR, the thread, the ticket)
   and write the draft from its output: condensed to a postable
   reaction per the action's verb, with the first line naming the
   delegate (for example "dastardly's review, condensed:"), so the
   human knows at approval time whose work they are signing. When the
   delegate fails, the step 2 rule holds: draft from what you have, say
   so inside the draft, and still remove the intent. A stance that
   names no agent changes nothing here.
4. Append to the entry file, prepending a newline when the file lacks a
   trailing one (the marker line must start its own line):

   ```
   --- dictated <ISO-8601 UTC now>
   <the guidance, copied verbatim from the intent>

   --- draft <ISO-8601 UTC now>
   <the draft text>
   ```

   Times are RFC 3339 (e.g. `2026-08-14T10:00:00Z`); the TUI only
   recognizes a marker with a parseable time.
5. Remove the intent file, then log one line to stdout with the intent
   name, the entry path, and the action drafted. The guidance lives on
   inside the entry, so the draft can always be audited against what
   was asked.

## Guardrails

Research is read-only against Slack, GitHub, and Linear. The only
writes are the marker appends to entry files and the intent removals.
A delegated agent inherits the read-only rule: it must not post,
merge, or write anywhere; its findings land in the draft, and the
actor posts them only after approval.
This runner cannot post: publishing is thinkpol's job, executed per
docs/contracts/thinkpol.md only on an explicit double-key approval. Every move between
states belongs to the human at the TUI.
