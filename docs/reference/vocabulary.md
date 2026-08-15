# Vocabulary

Lookup page for the domain terms. Every page uses exactly these words,
one term per concept. The 1984 proper names (minitrue, recdep,
telescreen, speakwrite, thinkpol, memoryhole; the drawers tube, desk,
upsub, files) are mapped to their roles in the
[README glossary](../../README.md#the-names) and in
[the names](../design/names.md).

## Terms

| Term | Meaning |
|---|---|
| event | Something that happened upstream: a review request, a thread reply, a ticket assignment. |
| record | One file in the queue describing one event. |
| queue | The whole recdep directory tree of records. |
| drawer | One state directory of the queue: tube, desk, upsub, or files. |
| source | The provider a record came from: github, slack, or linear. |
| producer | The program that polls sources and files records. minitrue is the shipped producer. |
| stance | The text you type when you dictate. |
| intent | The file recording a request for a reaction: an entry line, an action line, a guidance section. |
| action | The verb in the intent selecting the reaction type. |
| guidance | Stance text carried by the intent: the rule default plus what you typed. |
| reaction | What speakwrite writes for a record: a reply, a review, or a comment. Use the specific word when the context is specific; reaction is the umbrella. |
| draft | A reaction not yet approved. |
| approval | The recorded double-key consent to post a draft. |
| actor | The program that posts approved drafts. thinkpol is the shipped actor. |
| publisher | The actor's per-provider posting backend: github-pr, slack-thread, or linear-issue. |
| marker | An appended section in a record: dictated, draft, published, or discarded. |
| stale | A record whose event resolved without you. |
| rule, action map | The config rules picking a record's action and guidance. |
| agent | An LLM process following a skill. |
| skill | The instruction file an agent follows. |

## Verbs

| Verb | Meaning |
|---|---|
| file | Write a record into the queue, or move one to the files drawer. |
| take | Move a record from tube to desk. |
| up | Move a record to upsub: you acted, the other side owes the next move. |
| dictate | Write a stance for a record; the intent carries it to speakwrite. |
| approve | Record the double-key consent to post a draft. |
| discard | Reject a draft; a pending approval is revoked. |
| delete | Remove a record permanently (the memory hole). |
