# speakwrite: how drafts are written

In the book, the speakwrite turns dictation into text that goes back
into the record. Here: the human dictates a stance on a record, the
speakwrite agent writes the draft reaction into the record file, and
the human approves it with an explicit second step. speakwrite never
posts anything on its own; the actor is thinkpol
([Actor contract](../contracts/thinkpol.md)).

## Flow

1. Dictate. In telescreen, `s` on a record suspends into `$EDITOR` on a
   pre-filled intent: the record path, the mapped action, and an empty
   guidance section. The human writes the stance in plain words ("agree
   with the Validate finding; push back on the driver-mode one, it is
   intentional; ignore the style nit"). Saving submits the intent;
   saving with empty guidance means the action's defaults; aborting the
   editor cancels.
2. Draft. The speakwrite agent picks up the intent, does read-only
   research (the PR, the thread, the diff), and appends a dictated
   marker and a draft marker to the record. The intent file is removed;
   the guidance lives on inside the record, so the draft can always be
   audited against what was asked.
3. Review. The detail pane shows the draft because it is part of the
   body. The row's status column shows `draft`. `s` again re-dictates
   with the previous guidance pre-filled; the new draft supersedes the
   old.
4. Approve. `p` shows the target in the status line; a second `p`
   writes an approval into the intent directory. thinkpol executes it
   per the [Actor contract](../contracts/thinkpol.md): it posts the
   draft through the publisher table, appends a published marker with
   the permalink, and moves the record to upsub (unless it already sits
   in upsub or files). The double keypress is the recorded consent; the
   TUI itself still never calls the network.

## Key bindings

| Key | Name | Works in | Effect |
|---|---|---|---|
| `s` | dictate | tube, desk, upsub | Open the intent in `$EDITOR`; save submits, empty guidance means defaults, abort cancels. Row status column turns `dictated` |
| `s` | re-dictate | record with a draft or pending intent | Reopen with the previous guidance; the new draft supersedes |
| `p` `p` | approve | record with a `draft` status | First press names the target in the status line, second press records the approval; thinkpol posts and moves the record ([Actor contract](../contracts/thinkpol.md)) |
| `D` | discard | record with a `draft` status | Append a discarded marker; the draft stays in the record but stops rendering as actionable |

`p` is double-keyed like `x` because posting is outward-facing the way
deletion is destructive. `D` is shifted because it discards work; the
lowercase keys are safe moves.

## Files

The intent format and the four marker kinds are normative in the
[Queue contract](../contracts/recdep.md). The design-relevant part is
how the TUI derives the status column from the last marker: `dictated`
after dictated, `draft` after draft, nothing after published (the
record moves drawer instead) or discarded. Everything is append-only, the TUI
included: the discarded marker is the one line the consumer may append,
so the record keeps the full history and the rename-only rule bends
exactly once.

## Action selection

A record's source and shape pick the action through the rules in
`telescreen.yaml`; the built-in action map and the verb-to-draft mapping
live in the [Configuration reference](../reference/configuration.md).

## Guardrails

- The agent's research is read-only against Slack, GitHub, and Linear.
  Its only writes are appends to record files. The post is thinkpol's,
  and it requires a recorded double-key approval
  ([Actor contract](../contracts/thinkpol.md)).
- Every step leaves a marker in the record: the queue is the audit
  trail.
- A failed post leaves the record untouched (the draft tag survives so
  the human can re-approve) and removes the approval; nothing retries
  silently. The failure is logged.

## Out of scope for v1

- Interactive tmux sessions for full PR reviews (a live session serves
  those better; the dashboard can grow a launcher later).
- Agent-decided actions and tiered direct posting.

## Design rationale: the five decisions

| Decision | Choice | Why |
|---|---|---|
| Execution surface | Headless agent (subcommand plus a systemd path unit on `recdep/intents/`, same pattern as minitrue) | telescreen stays offline and file-only; the agent is swappable like the producer |
| Action selection | Rule-mapped: the record's source and shape pick the action; the human adds guidance at dictation time | The map knows the verb, only the human knows the stance |
| Result flow | Draft appended to the record, rendered in the detail pane, tagged in the list row | The record stays in the queue; the TUI needs only a tag |
| Human gate | Draft-then-approve: the agent never posts; posting requires an explicit, double-keyed approval | Nothing outward-facing happens without live consent |
| Naming | speakwrite | Dictation in, corrected record out, filed back into recdep |
