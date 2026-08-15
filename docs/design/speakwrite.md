# speakwrite: the drafting layer

In the book, records clerks dictate corrections into the speakwrite and the
machine produces the text that goes back into the record. Here: the human
dictates a stance on a queue entry, an agent drafts the response into the
entry file, and the human publishes it with an explicit second step.
speakwrite never posts anything on its own; the posting binary is
thinkpol ([thinkpol.md](../contracts/thinkpol.md)).

## Flow

1. Dictate. In telescreen, `s` on an entry suspends into `$EDITOR` on a
   pre-filled intent: the entry path, the source-mapped action, and an
   empty guidance section. The human writes the stance in plain words
   ("agree with the Validate finding; push back on the driver-mode one,
   it is intentional; ignore the style nit"). Saving submits the intent;
   saving with empty guidance means the action's defaults; aborting the
   editor cancels.
2. Draft. The speakwrite runner picks up the intent, does read-only
   research (the PR, the thread, the diff), and appends a dictated
   section and a draft section to the entry file. The intent file is
   removed; the guidance lives on inside the entry, so the draft can
   always be audited against what was asked.
3. Review. The detail pane shows the draft because it is part of the
   body. The row carries a `[draft]` tag. `s` again re-dictates with the
   previous guidance pre-filled; the new draft supersedes the old.
4. Publish. `p` shows the target in the status line; a second `p` writes
   a publish approval into the intent directory. thinkpol executes it
   per [thinkpol.md](../contracts/thinkpol.md): it posts the draft through the publisher table
   (`internal/publish`), appends a published line with the permalink,
   and moves the entry to upsub (unless it already sits in upsub or
   files). The double keypress is the recorded consent; the TUI itself
   still never calls the network.

## Key bindings

| Key | Name | Works in | Effect |
|---|---|---|---|
| `s` | dictate | tube, desk, upsub | Open the intent in `$EDITOR`; save submits, empty guidance means defaults, abort cancels. Row gains `[dictated]` |
| `s` | re-dictate | entry with a draft or pending intent | Reopen with the previous guidance; the new draft supersedes |
| `p` `p` | publish | entry with `[draft]` | First press names the target in the status line, second press approves publication; thinkpol posts and moves the entry ([thinkpol.md](../contracts/thinkpol.md)) |
| `D` | discard | entry with `[draft]` | Append a discarded marker; the draft stays in the record but stops rendering as actionable |

`p` is double-keyed like `x` because publishing is outward-facing the way
incineration is destructive. `D` is shifted because it discards work; the
lowercase keys are safe moves.

## Files

Intent (`recdep/intents/<entry-name>.intent`), written by the TUI:

```
entry <absolute entry path>
action <mapped action, e.g. review-reply>

guidance:
<free text, possibly empty>
```

Entry additions, appended by the drafting runner (dictated, draft) and
by thinkpol (published), each marker on its own line, the same append
discipline as the stale marker:

```
--- dictated <ISO-8601 time>
<the guidance, copied from the intent>

--- draft <ISO-8601 time>
<the draft text>

--- published <ISO-8601 time> <URL>

--- discarded <ISO-8601 time>
```

The TUI derives tags from the last marker: `[dictated]` after dictated,
`[draft]` after draft, nothing after published (the entry moves state
instead) or discarded. Everything is append-only, the TUI included: the
discarded marker is the one line the consumer may append, so the record
keeps the full history and the rename-only rule bends exactly once.

## Source-action map

The default map ships in the speakwrite skill; a config file can override
it later.

| Entry shape | Action |
|---|---|
| review requested on a PR | draft the review (or the review reply round) |
| bot findings on an own PR | vet each finding against the code, draft per-finding replies |
| human comment or reply on an own PR | draft the thread reply |
| Slack reply in a watched thread | draft the response |
| Linear comment on an assigned ticket | draft the comment |

## Guardrails

- The runner's research is read-only against Slack, GitHub, and Linear.
  Its only writes are appends to entry files. The published post is
  thinkpol's, and it requires a recorded double-key approval
  ([thinkpol.md](../contracts/thinkpol.md)).
- Every step leaves a marker in the entry file: the record in recdep is
  the audit trail.
- Publishing failures leave the entry untouched (the draft tag survives
  so the human can re-approve) and remove the approval; nothing retries
  silently. The failure is logged.

## Out of scope for v1

- Interactive tmux sessions for full PR reviews (a live session serves
  those better; the dashboard can grow a launcher later).
- Agent-decided actions and tiered direct posting.

## Design rationale: the five decisions

| Decision | Choice | Why |
|---|---|---|
| Execution surface | Headless runner (subcommand plus a systemd path unit on `recdep/intents/`, same pattern as minitrue) | telescreen stays offline and file-only; the runner is swappable like the producer |
| Action selection | Source-mapped: the entry's source and shape pick the action; the human adds guidance at dictation time | The map knows the verb, only the human knows the stance |
| Result flow | Draft appended to the entry file, rendered in the detail pane, tagged in the list row | The record stays in recdep; the TUI needs only a tag |
| Human gate | Draft-then-publish: the runner never posts; publishing requires an explicit, double-keyed approval | Nothing outward-facing happens without live consent |
| Naming | speakwrite | Dictation in, corrected record out, filed back into recdep |
