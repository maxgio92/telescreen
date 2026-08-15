# Swap the actor

The actor is the one component that writes upstream. It is defined by
the `.publish` contract, not by any particular executable: this repo
enrolls a deterministic reference actor (`telescreen thinkpol`, plain
Go, posts the draft verbatim), and you can enroll your own instead.
The normative contract is the
[Actor contract](../contracts/thinkpol.md); this page is the short
version.

## What an actor must do

Consume `recdep/intents/*.publish` approvals, oldest first. For each:

1. Resolve the record from the approval's `entry <absolute path>` line.
2. Extract the last draft marker; no draft, or a discarded marker
   after it, means remove the approval and touch nothing.
3. Post the draft to the record's URL.
4. On success: append the `--- published <time> <URL>` marker, move the
   record to `upsub/` (unless it already sits in `upsub/` or `files/`),
   remove the approval.
5. On failure: leave the record untouched, remove the approval so
   nothing retries silently, log the reason.

Declare your semantics: verbatim (the draft is the post) or
interpretive (you may adapt the text, and must then record what you
actually posted in the published marker's section body).

## Enroll it

Swapping is enrollment, not configuration: disable the shipped path
unit and enable your own on `recdep/intents/*.publish`.

```
systemctl --user disable --now thinkpol.path
systemctl --user enable --now your-actor.path
```

Exactly one actor at a time: two consumers of the same approvals would
race into double-posting.

## Credentials

The shipped actor reads `~/.config/thinkpol.env` (SLACK_TOKEN,
LINEAR_API_KEY; GitHub goes through your authenticated `gh`). Your
actor brings its own; the contract does not care how it
authenticates, only what it writes to the queue.
