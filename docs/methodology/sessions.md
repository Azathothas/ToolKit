# sessions.md

What a session owes at its start and at its end, and how one is resumed when it
did not finish.

⭐ **The governing principle: the conversation is not the source of truth. The
tree, the record and the running system are.** A summary, yours or the
harness's, can claim anything. None of it is real until an artefact confirms it.
Trust artefacts, verify claims.

---

## Starting

1. ⭐ **Read the record first.** It is the one file that always carries what
   changed since last time: the baseline, what the last session did, what is in
   progress, and the work order. Everything session-specific lives there.
2. **Run the probe.** [`../../scripts/doctor/`](../../scripts/doctor/). A
   different machine, a different shell, or a tool that moved changes what this
   session can prove. Run it even on a machine you think you know.
3. **Run the capability check** in [`gate.md`](gate.md). What can this harness
   actually do. A gate the plan needs and this session cannot run is surfaced
   now, not discovered at the end.
4. **Re-measure the baseline** rather than trusting the recorded one. Run the
   checks and read what they say today.
5. **Read what the task routes you to**, per the project's own router. Not
   everything, and not less than the routing table names.
6. **Restate the plan and its decisions in a few bullets** before touching
   anything. This is not ceremony: it reloads the design into working memory
   and catches a misreading before it becomes a wrong build.
7. **Record the start instant, in ISO 8601 UTC.** Everything at the end that
   measures the session reads it from there.

```bash
date -u +%Y-%m-%dT%H:%M:%SZ
```

---

## Ending

In this order.

1. **Finish or checkpoint the current task.** A half-finished change is
   recorded as partial, with what is done and what is not, never left silent.
2. **Run the gate.** All three parts. [`gate.md`](gate.md).
3. **Update the record in the same change as the work.** ⛔ The record is part
   of the change, not a report about it. A session that fixes something and
   leaves the record saying it is open has not finished; it has made the next
   session read a lie first.
4. **Update the documentation the work changed**, in the same change.
5. **Write the handoff**, in stage mode, or close the entry with its evidence,
   in todo mode.
6. **Print the summary table.** Below.
7. **Print the next prompt.** Below.
8. **Tear down** anything this session created on a remote system.
   [`../security/remote-ops.md`](../security/remote-ops.md).

---

## The summary table

⛔ **No session ends without one, and it does not depend on the session having
gone well.** A session that ran out of budget after one task still owes it.

It is **for the operator**, and it goes in chat. Prose is not a summary; a wall
of paragraphs is what this rule exists to stop. One markdown table, before and
after, ⛔ **every cell grounded in something you can point at.**

| row | from |
| --- | --- |
| Elapsed | the recorded start instant to now |
| Commits | the git log over this session's range |
| Work | how many assigned items **completed, deferred, failed**. Counted, not described. |
| Changes | files touched, lines added and removed |
| Size | the tree's line count, and the delta |
| Checks | the gate's result, and what it was at the start |
| Cost | if the work spends money or bandwidth, the number, split by what it was spent on |
| Health | debts cleared and introduced, tree clean or dirty, deployed version |

⛔ **It has to be able to say that nothing moved.** A summary that can only
report improvement is fabricated progress with a table around it. "Nothing
moved, and here is what was measured to establish that" is a complete and
honest answer. What is not acceptable is silence, a number with no before, or a
number with no conditions.

⛔ **If you did not measure something, write that you did not.** Never a number
you did not take.

⭐ **Save it as well as printing it**, beside the record, so it survives the
chat scrolling away. The next session reads it as the fastest orientation into
what the last one actually did.

---

## The next prompt

Printed in chat, in a fenced block, ⛔ **never written into a file.**

**Which prompt depends on one thing: did this session finish what it was
given?**

| state | print |
| --- | --- |
| finished | the kickoff for the next unit of work |
| not finished, for any reason | ⭐ a **resume** prompt that says what is left and why |
| some done, some not | ⭐ the **resume** prompt. A kickoff printed over unfinished work is how a debt gets silently inherited. |

"Ran out of budget" is a reason. Omitting the prompt is not an option.

### What a kickoff carries, and what it must not

⭐ **It carries a pointer to the record, never a copy of it.**

This is the one place where two defensible practices collide, and the
resolution matters. A prompt that restates the work order is a second copy of
it that goes stale the moment an item closes, and it costs the next session's
budget to read something it is about to read again. But a bare list of paths is
a list that gets skimmed.

Both are true, and they do not actually conflict, because they are about
different things:

- **The reading list is stable**, so it goes in the prompt, ⭐ **with a one-line
  summary of what each file is and why it matters.** A path that says why it
  matters is what gets read.
- **The work order is not stable**, so it stays in the record, which is
  tracked, versioned, and read first anyway.

So the prompt carries only what a reader cannot get from the repository:

- one line on what the project is, because a fresh session has to know that
  before it opens anything;
- what to read, in order, each with its one-line summary;
- whether the session is attended, and what to do when blocked;
- anything the operator has to supply this time;
- any warning carried over from last time.

⛔ It carries **no** item ids, no counts, no check results, no version numbers
and no work order. Those are the record's, which is where they are already
correct.

⚠ Re-rank the reading list for the actual work and add what that work needs.
The list is not boilerplate to copy. It is the session's reading order, and a
wrong order is a wrong session.

### The rework prompt

When the operator's validation fails, they get a prompt listing each issue as
**what I did, what happened, what I expected**, and instructing: reproduce each
one first, fix the root cause with no workarounds, add a regression test per
issue, re-run the full gate, update the handoff.

---

## Freezing cleanly

⭐ **The best resumption is one the previous session set up.** When a session is
ending, over budget, or interrupted:

1. **Get the tree coherent.** Commit the work in progress, or deliberately
   stash it. ⛔ Never leave a half-edit across the boundary: a broken half-edit
   is worse than nothing, because the next session cannot tell it from
   finished work.
2. **Leave the checks green**, or have the record say exactly which are red and
   why.
3. **Write the partial record** with an honest status and a pointer to the
   resume point, the next unstarted task.
4. **Print the resume prompt.**

---

## Resuming

A unit of work may be picked up by a session with none of the previous one's
context: a fresh session, a summarised history, or a different agent entirely.
The methodology assumes this and never depends on the conversation surviving.

⛔ **Reconstruct from artefacts. Reconcile any mismatch before continuing.**

```bash
git log --oneline -15
```

```bash
git status --short
```

```bash
git diff --stat
```

Then read the record, read the latest handoff end to end, and check the running
system's actual state. Reconcile the three against each other:

| signal | what it means | what to do |
| --- | --- | --- |
| tree clean, checks green, deployment matches the record | a clean stopping point | resume at the next unstarted task |
| the tree has uncommitted work | an edit was mid-flight | read the diff and understand it. Finish it or revert it. ⛔ Never build on work in progress you do not understand. |
| the deployment is **ahead of** the tree | something was deployed and never committed | recover it now. A deployment with no commit cannot be resumed from. |
| the deployment is **behind** the tree | committed, not yet deployed | fine. Note it as pending. |
| the checks are red | the last edit broke something | fix to green first. A session that builds on red compounds the break. |
| a file the summary claims was written is not on disk | the write never landed | redo it. The claim was not real. |

⚠ **Warm and cold resumption differ.** With the conversation still present, you
still verify the durable state, because a summarised turn can drop that a write
failed or a deployment did not land. With the conversation gone, trust nothing
from any prior narrative and rebuild the model entirely from the tree, the
record and the running system.

⚠ **Re-run the capability check for this session.** The session that froze may
have had capabilities this one lacks, or the reverse.

---

## Do not idle

⚠ **Do not end a turn to wait for something.** The conversation idles, the
harness times out, and the session dies mid-operation with state half-changed.

Hold in the foreground with a loop that ticks and prints progress, and keep the
tick short enough that nothing looks hung. A background job and a hold loop are
not alternatives; use both. See
[`../conventions/shell.md`](../conventions/shell.md) section 10 for the shape.
