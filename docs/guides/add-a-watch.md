# Add a watch

Read this to make the shipped producer watch something new: another
Slack channel, a second repo, activity the built-in watches skip. You
edit prose, not code.

## Where the watches live

The watches are lettered prose instructions in the producer's prompt.
For claude, that is the installed skill at
`~/.claude/skills/minitrue/SKILL.md`: a user file, yours to edit;
re-installs keep your edits, and `telescreen install --force` restores
the shipped text. For another agent, it is the file named by
`minitrue.instructions` in `telescreen.yaml`
([Configuration reference](../reference/configuration.md#minitrue)).

## Example: watch a Slack channel

Add one entry to the skill's watch list, in the same shape as the
existing ones: filter strictly after `since`, file one record per
event, skip what the watched person authored.

```
- G. Slack channel #build-status: `slack_search_public_and_private`
  `in:#build-status`. Enqueue messages by others with ts after
  `since`, one record per message; Slack `after:` is date-granular,
  so filter ts precisely. Keep the `[slack]` source tag and let the
  slug carry the channel name.
```

Stay on the tools the shipped watches already use; a new tool also
needs an entry in `minitrue.allowed_tools`
([Configuration reference](../reference/configuration.md#minitrue)).

The next timer run reads the edited skill; no re-enroll, no restart.

## Scope the reaction (optional)

Records from the new channel arrive with the built-in `slack-reply`
action. To give them their own guidance, add an action-map rule keyed
on the channel's archives URL:

```yaml
speakwrite:
  actions:
    - url_prefix: https://acme.enterprise.slack.com/archives/C0BUILD0000/
      action: slack-reply
      guidance: build chatter; short, informal
```

A non-empty `actions` list replaces the built-in map entirely, so
carry over the built-ins you still want; every field is in the
[Configuration reference](../reference/configuration.md#rule-fields).

## When this page is the wrong one

A source outside Slack, GitHub, and Linear is a new producer: the
shipped skill's tools reach only those three.
[Write a producer](write-a-producer.md) covers filing records from
anything.
