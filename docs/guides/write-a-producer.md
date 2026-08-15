# Write a producer

Anything that writes conforming records into `tube/` is a producer: an
LLM agent, a deterministic poller in Go or shell, a webhook receiver.
The screen does not know or care which. The normative contract is the
[Queue contract](../contracts/recdep.md); this page is the short
version.

## The minimum

Write one file per event into
`${XDG_STATE_HOME:-$HOME/.local/state}/recdep/tube/`, named
`<UTC>-<source>-<slug>.md` (`<UTC>` is `YYYYMMDDTHHMMSSZ`, so lexical
order is time order), mode 0600, with this body:

```
[<source>] <who>: <one-line summary>
<link>
seen <ISO-8601 instant>

<preview>
```

A minimal producer can emit only line 1 and still render.

## The obligations

Distilled from the [Queue contract](../contracts/recdep.md), which is
the authority when they disagree:

1. Write each hit exactly once; dedupe across polls is your job.
2. Track your own cursor in the `since` file (an ISO-8601 UTC instant
   next to the drawers); enqueue strictly after it, then advance it.
   The consumer never touches it.
3. Skip activity authored by the watched person; the queue is for what
   others did.
4. On a partial outage, file a degraded record naming the gap rather
   than failing silently.
5. Records are append-only once written. One sanctioned addition: a
   revalidation pass may append a single
   `stale <reason> <ISO-8601 time>` marker line to records in `tube/`,
   `desk/`, or `upsub/`. Never mark twice, never touch `files/`, never
   delete.

Write only into `tube/`; every other drawer belongs to the consumer.

## Check your work

```
telescreen verify
```

lints the queue against the contract grammar and exits 1 on findings.

## Enroll it

Replace the shipped producer by enabling your own unit instead of the
minitrue timer: enrollment, not configuration. The
[Configuration guide](configuration.md) explains the split.
