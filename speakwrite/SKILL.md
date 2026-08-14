---
name: speakwrite
description: The drafting and publishing runner for the telescreen queue. Consume speakwrite dictation intents from recdep/intents/, research the entry read-only (the PR, the thread, the ticket), and append a dictated and a draft marker section to the entry file for human review in the telescreen TUI. Consume publish approvals (.publish files, written by the TUI after a double-key approval) and post the approved draft as a GitHub PR comment. Use when asked to run the speakwrite, drain dictation intents, draft the queued responses, publish approved drafts, or when a headless run invokes /speakwrite draft.
---

# speakwrite

The runner half of the flow defined in SPEAKWRITE.md of
github.com/maxgio92/telescreen. The TUI's `s` key writes an intent file;
this runner consumes it, researches the entry, and appends the draft to
the entry file. The TUI's `p` `p` writes a publish approval; this runner
consumes it and posts the draft. The queue and marker contract is
normative in RECDEP.md; this skill is one runner implementation, and any
program that appends conforming marker sections is a drop-in
replacement.

`draft` (the first arg, default, headless) drains `intents/`: the
drafting pass over `*.intent` files, then the publish pass over
`*.publish` files.

## Paths

State root: `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`

- `intents/`: one `<entry-name>.intent` file per dictation and one
  `<entry-name>.publish` file per publish approval, written by the TUI.
  `.intent.tmp` files are dictations still open in the editor; leave
  them alone.
- `inbox/`, `todo/`, `waiting/`, `archive/`: the entry state dirs. The
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

## publish (headless, the one outward write)

Approval format, written only by the TUI after a double-key approval:

```
entry <absolute entry path>
```

For each `intents/*.publish`, oldest first:

1. Parse the entry path and resolve the entry the way the drafting pass
   does: the recorded path, else the same filename in the other state
   dirs, else remove the approval, log one line naming the orphaned
   approval and the missing entry path, and continue.
2. Extract the last draft section (the text after the last `--- draft`
   marker line, up to the next marker or the end) and the entry URL
   (line 2 of the body). The TUI gates on github.com; verify anyway,
   and on a non-GitHub URL remove the approval and log why.
3. Post the draft text as a PR comment. Derive the number and repo from
   the URL (`https://github.com/<owner>/<repo>/pull/<number>`), write
   the draft text to a temporary file, then:

   ```
   gh pr comment <number> --repo <owner>/<repo> --body-file <file>
   ```

   This is the single sanctioned outward write, and only because the
   `.publish` approval exists. Log the comment URL `gh` returns.
4. On success: append `--- published <ISO-8601 UTC now> <comment URL>`
   to the entry (the same newline discipline as the draft append), move
   the entry file to `waiting/` unless it is already in `waiting/` or
   `archive/`, remove the approval, and log one line with the approval
   name, the entry path, and the comment URL.
5. On failure: leave the entry untouched (the `[draft]` tag survives so
   the human can re-approve), remove the approval so nothing retries
   silently, and log the error.

## Guardrails

Research is read-only against Slack, GitHub, and Linear. The writes
are the marker appends to entry files, the intent and approval
removals, the publish rename to `waiting/`, and the one approved PR
comment per `.publish` file. Nothing posts anywhere without the
explicit double-key approval defined in SPEAKWRITE.md; every other
move between states belongs to the human at the TUI.
