---
name: minitrue
description: Watch one person's Slack threads, GitHub PRs (opened, mentioned, and review-requested), and assigned Linear tickets. Producer/consumer over a file queue with a desk state for seen-but-unactioned items. Use when asked to watch my notifications, set up a personal notification loop, drain the watch queue, mark a watch item filed, or keep an eye on replies, reviews, mentions, or ticket activity. minitrue produces headless on a timer; the telescreen TUI (pw) is the consumer.
---

# minitrue

Single producer, single consumer, filesystem queue. The producer (minitrue)
polls and enqueues; the consumer (the telescreen TUI) drains to the terminal.
Only the producer touches Slack, GitHub, or Linear.

`produce` (the first arg, default, headless/timer) enqueues; the `pw` TUI
dashboard consumes.

The queue format is a producer-agnostic contract, documented normatively in
docs/contracts/recdep.md of github.com/maxgio92/telescreen. This skill is one producer
implementation (agentic, via a headless Claude run); any program that writes
conforming entry files works as a drop-in replacement.

## Paths

State root: `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`

- `since`: ISO-8601 UTC instant the producer last polled through. Producer owns
  it; consumer never touches it.
- `tube/`: one file per unseen hit. Presence means unseen.
- `desk/`: seen but not yet acted on. The dashboard's `d` key moves entries here.
- `upsub/`: acted on, the other side's move is pending. The dashboard's `u`
  key moves desk entries here.
- `files/`: closed, nothing more expected. Entries land here via the dashboard's `f` key.

## Identity (config)

Read `~/.config/minitrue.env` if present, else these defaults:

```
SLACK_USER_ID=U0000000000   # your Slack user id
GH_LOGIN=your-gh-login      # your GitHub login; gh @me must resolve to it
LINEAR_ASSIGNEE=me
REPO=owner/repo             # GitHub repo to scope PR searches to
BOT_LOGINS=                 # space-separated bot logins to skip, besides [bot] suffixes
```

`gh @me` must resolve to `GH_LOGIN`; Linear `me` to the same person.

## produce (headless, the only API caller)

1. Read `since`. If absent, write `now`, create `tube/`, `desk/`,
   `upsub/`, and `files/`, and stop (baseline; no history replayed).
2. Run the four watches for activity strictly after `since`, from someone other
   than the watched person. For each hit, write
   `tube/<UTC>-<source>-<slug>.md` with this body:
   ```
   [<source>] <who>: <one-line summary>
   <link>
   seen <produce-run-time>
   <key> <value>

   <preview>
   ```
   `<UTC>` is `YYYYMMDDTHHMMSSZ` so files sort by time. The `<key> <value>`
   metadata lines carry the source's structured facts, one per line, between
   the seen line and the blank line: for github hits `org` and `repo` (the
   owner and name from `$REPO`); for slack hits `channel` (the `#` name) for
   channel threads or `dm` (comma-separated participants) for DMs; for linear
   hits `project` and `ticket` (the KEY, e.g. `FUL-1`). Write the keys you
   know; skip a line when the fact is unavailable. `<preview>` is the
   triggering content quoted verbatim after a blank line: the Slack reply
   text, the GitHub review or comment body, or the Linear comment. Cap it at
   roughly 15 lines or 1000 characters and append `[...]` when truncated.
   The dashboard renders it in the detail pane as is.
3. Write `since = now` (the run's start).

Watches:

- A. Slack threads: `slack_search_public_and_private`
  `from:<@$SLACK_USER_ID> is:thread`, covering public and private channels
  alike. For each thread root he authored, `slack_read_thread`, enqueue
  replies by others with ts after `since`. Slack `after:` is
  date-granular, so filter ts precisely.
- F. Slack DMs: `slack_search_public_and_private` `to:<@$SLACK_USER_ID>`
  (and `is:dm` where the search syntax needs it), covering direct and
  group DMs. Enqueue messages from others with ts after `since`; keep the
  `[slack]` source tag and let the slug carry the dm hint. DMs are
  sensitive content: keep the preview within the usual cap and quote
  nothing beyond the triggering message.
- B. PRs opened: `gh search prs --repo $REPO --author @me --json number,title,url,updatedAt`.
  For each updated after `since`, read `gh pr view <n> --json reviews,comments`
  and `gh api repos/$REPO/pulls/<n>/comments`; enqueue reviews and comments by
  anyone other than `$GH_LOGIN` created after `since`.
- C. PRs mentioning him: `gh search prs --repo $REPO --mentions @me --json number,title,url,updatedAt`.
  Enqueue PRs newly mentioning him or with mention-activity after `since`.
- E. PRs review-requested: poll `gh api "/notifications?all=true&per_page=50"`
  and select entries with `.reason == "review_requested"` in `$REPO`. Use the
  notifications API, not `gh search prs --review-requested`: the search index
  lags and silently drops recent requests, while a notification fires the moment
  someone requests his review, directly or through a team he belongs to. For
  each hit resolve the PR (`.subject.url`) and enqueue those seen after `since`;
  skip bot authors (login ending in `[bot]`, `dependabot`, or any login in
  `$BOT_LOGINS`, the org's bot accounts). Dedupe across polls: append each
  enqueued PR number to `$STATE/seen-review-requests` (one per line) and skip any
  PR already listed, so a notification that keeps reappearing enqueues once, not
  every run. Caveat: a request whose
  notification is already marked read will not reappear, so this catches new
  requests going forward rather than back-filling history; for a one-time
  backlog check, list open PRs and read each `reviewRequests` directly.
- D. Linear assigned: `list_issues assignee $LINEAR_ASSIGNEE orderBy updatedAt`.
  Enqueue tickets created or assigned after `since`, and new comments after
  `since` (`list_comments issueId=...` per assigned ticket updated after
  `since`) by someone other than the watched person.

Revalidate (after the watches, same run): for each entry in `tube/`,
`desk/`, and `upsub/` whose URL points at a GitHub PR in `$REPO` and whose
body does not already contain a `stale` line, check the PR with `gh`:

- PR merged: append `stale merged <now>`. PR closed without merging: append
  `stale closed <now>`.
- Review-requested entry with a review by `$GH_LOGIN` submitted after the
  entry's `seen` time: append `stale already-reviewed <now>`. An entry is
  review-requested when its PR number appears in
  `$STATE/seen-review-requests`; use that file as the discriminator, not
  the summary text.

`<now>` is the produce run time, ISO-8601 UTC. Keep it cheap: one
`gh pr view <n> --json state,mergedAt,reviews` per distinct PR, reused across
entries for the same PR. One marker per entry, ever; never touch `files/`.
Slack and Linear entries are out of scope for now.

Headless caveat: GitHub uses the `gh` CLI (token auth, always works). Slack and
Linear are MCP servers; if a headless run lacks their auth, run those watches
best-effort and enqueue a `[minitrue] degraded` entry naming the gap
rather than failing silently.

## dashboard (the consumer surface, no API, no auth)

The TUI at `~/src/github.com/maxgio92/telescreen/` (private repo
github.com/maxgio92/telescreen) is the consumer. `pw`
launches it (building `~/.local/bin/telescreen` on first use; `pw` stays as a
short alias). Five views (tube, desk, upsub, files, memoryhole) switch with tab or
1-5; `d` moves tube to desk (take), `u` moves desk to upsub (sent up,
their move), `f` moves desk or upsub to files, `z` moves one state
back (files to upsub, upsub to desk, desk to tube). A fifth virtual
view, memoryhole (key 5), is always empty, and `x` in files, pressed twice
on the same entry, permanently deletes its file. `o` opens the
entry's URL, `y` copies it, `q` quits. `s` on a tube, desk, or upsub
entry dictates a speakwrite intent: it opens a pre-filled intent file in the
editor and submits it into `intents/` on save, and the row's status column
shows `dictated` while the intent is pending. It watches the state dirs with
fsnotify, so new producer entries appear live. `pw -once` prints per-state
counts and exits.

The dashboard only reads, shows, and renames files between the state dirs. It
never calls Slack, GitHub, or Linear, and never advances `since`. Files means
closed, nothing more expected; `upsub/` holds entries where the other side owes the
next move; `desk/` is the live to-do list. When an agent performs an entry's
action in-session (posts the review, sends the reply), it moves that entry's
file from `desk/` to `files/` (or `upsub/` when a reply is expected) in the
same turn.

## Driving it

- Producer: a systemd user timer runs `claude -p "/minitrue produce"`
  every ~30 min (durable across sessions).
- Consumer: the `pw` shell function launches the TUI dashboard.

## Guardrails

Read-only against Slack, GitHub, and Linear. The producer writes only the local
queue; the consumer only moves files. Never merge, comment, or post anywhere
without an explicit go-ahead in a live turn.
