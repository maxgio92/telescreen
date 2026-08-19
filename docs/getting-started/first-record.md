# Your first record

A walkthrough of the whole loop on the demo record. It exercises every
step except the final post: demo#42 does not exist, so the publish for
this record would fail. The demo is for looking, not posting.

## Seed and open

```
telescreen demo
```

One record lands in the tube: a review request from julia on demo#42,
about the chocolate ration.

## Triage

- The tube view shows the record, its context column carrying the repo
  (`demo`); the detail pane shows its full body:
  the content line, the preview, then the labeled tail: the path, the
  URL, the metadata lines (`org example`, `repo demo`), and the seen
  stamp.
- Press `t`: take it to the desk. The move is a file rename; check with
  `ls ~/.local/state/recdep/desk/` if you like.
- `b` moves it back a drawer, `f` files it, `u` sends it to upsub. Try
  them; every move is reversible except the memory hole.

## Dictate

With the record selected in tube, desk, or upsub, press `s`. Your
editor opens on a pre-filled intent: the record path, the mapped action
(`review`, since the filename says review-requested), and an empty
guidance section. Write your stance in plain words:

```
guidance:
push back: the announcement contradicts the numbers.
```

Save to submit; abort the editor to cancel. The row gains `[dictated]`.
For a short stance, `r` opens a quick-reply popup inside the TUI that
submits the same intent with `ctrl+s`, no editor round trip.

## Draft

The speakwrite agent (if enrolled) picks up the intent, researches the
record read-only, and appends a dictated marker and a draft marker to
the record file. The row turns `[draft]` and the detail pane shows the
draft, because the draft is part of the file. Press `s` again to
re-dictate; the new draft supersedes the old.

## Approve

Read the whole draft first: `enter` opens the reader, a full-screen
scrollable view of the record, draft included; `q` closes it. `p p`
works inside the reader too. Press `p`: the status line names the
target. Press `p` again: the
approval is written to disk, the recorded consent for one outward
write. For a real record, thinkpol now posts the draft, stamps the
record with the comment URL, and moves it to upsub. For the demo
record the post fails (demo#42 does not exist), the draft survives,
and `publish.log` names the reason. Working as intended.

`D` discards a draft instead, and revokes a pending approval.

## Next

[Enroll the agents](enroll.md) for real records, then tune the action
map and the env files with the
[Configuration guide](../guides/configuration.md).
