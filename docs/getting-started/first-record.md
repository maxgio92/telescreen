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

- The tube view shows the record; the detail pane shows its full body:
  the content line, the URL, the seen stamp, the preview.
- Press `t`: take it to the desk. The move is a file rename; check with
  `ls ~/.local/state/recdep/desk/` if you like.
- `b` moves it back a drawer, `f` files it, `u` sends it to upsub. Try
  them; every move is reversible except the memory hole.

## Dictate

With the record selected in tube, desk, or upsub, press `s`. Your
editor opens on a pre-filled intent: the entry path, the mapped action
(`review`, since the filename says review-requested), and an empty
guidance section. Write your stance in plain words:

```
guidance:
push back: the announcement contradicts the numbers.
```

Save to submit; abort the editor to cancel. The row gains `[dictated]`.

## Draft

The speakwrite runner (if enrolled) picks up the intent, researches the
entry read-only, and appends a dictated section and a draft section to
the record file. The row turns `[draft]` and the detail pane shows the
draft, because the draft is part of the file. Press `s` again to
re-dictate; the new draft supersedes the old.

## Approve

Press `p`: the status line names the target. Press `p` again: the
approval is written to disk, the recorded consent for one outward
write. For a real record, thinkpol now posts the draft, stamps the
record with the comment URL, and moves it to upsub. For the demo
record the post fails (demo#42 does not exist), the draft survives,
and `publish.log` names the reason. Working as intended.

`D` discards a draft instead, and revokes a pending approval.

## Next

Enroll the real feed ([enroll.md](enroll.md)) and tune the action map
and the env files ([configuration](../guides/configuration.md)).
