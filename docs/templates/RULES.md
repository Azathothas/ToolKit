# Rules

<!-- TEMPLATE. Copy to the record directory, fill every {{PLACEHOLDER}}, delete
     this comment.

     This is the part of the record that does NOT change from session to
     session. What changes lives in PROGRESS.md.

     ⭐ Every rule here says WHAT IT COST TO LEARN. A rule with no incident
     behind it is a preference, and a preference stated as a rule is what makes
     an agent stop believing the rules that matter. If you cannot say what a
     rule cost, it probably belongs in the conventions rather than here. -->

How this repository is worked on. {{The record directory}} is the authoritative
record and this file is the part of it that does not change between sessions.

Read [`PROGRESS.md`](PROGRESS.md) first: what the last session did, the
measured baseline, and the work order.

---

## 1. Starting a session

Specified in
[`docs/methodology/sessions.md`](../docs/methodology/sessions.md). This
project's specifics:

1. Read [`PROGRESS.md`](PROGRESS.md).
2. Record the start instant on its state line, in ISO 8601 UTC. Everything at
   the end that measures the session reads it from there.

```bash
date -u +%Y-%m-%dT%H:%M:%SZ
```

3. Rewrite `PROGRESS.md` to say what **this** session is going to do, before
   doing it. Name the items and the files.
4. ⛔ **Re-measure the baseline rather than trusting the recorded one.**

```bash
{{the one command that runs every gate and prints one verdict}}
```

5. Run the probe. A different machine or a moved tool changes what this session
   can prove.

```bash
sh scripts/doctor/doctor.sh
```

---

## 2. Ending a session

Specified in
[`docs/methodology/sessions.md`](../docs/methodology/sessions.md). In order:

1. Finish or checkpoint the current task. ⛔ A half-finished change is recorded
   as partial, never left silent.
2. Update `PROGRESS.md`, and the items the session touched.
3. Update the documentation the work changed, in the same change.
4. Run the gate, all three parts.
   [`docs/methodology/gate.md`](../docs/methodology/gate.md).
5. Commit. {{And push, if the policy in section 4 permits it.}}
6. ⭐ Print the **summary table** in chat, and save it beside the record.
7. ⭐ Print the **next prompt** in chat, in a fenced block, never into a file.
   It is a **resume** prompt if anything at all was left unfinished.
8. Tear down anything created on a remote system.

---

## 3. The kickoff prompt

⭐ **It is generic and it stays generic.** Everything that changes from session
to session lives in `PROGRESS.md`, which is tracked, versioned, and read first
anyway.

A prompt that restates the work order is a second copy of it that goes stale
the moment an item closes, and it costs the next session's budget to read
something it is about to read again.

So it carries only what a reader cannot get from the repository:

- one line on what this project is;
- ⭐ what to read, in order, **each with a one-line summary of what that file
  is**. A bare path gets skimmed; a path that says why it matters gets read.
- whether the session is attended, and what to do when blocked;
- anything the operator must supply this time;
- any warning carried over.

⛔ It carries **no** item ids, no counts, no check results and no work order.

---

## 4. Git

{{The push policy, stated in one sentence and meant. The default is: commit
freely and locally, never push. Publishing is the operator's.}}

Everything else is in
[`docs/conventions/git.md`](../docs/conventions/git.md). The two that are
absolute:

- ⛔ **No tool is credited in a commit.** No co-author trailer, no
  generated-with line, no tool name in the body.
- ⛔ **Every other repository is read-only.** Never an issue, a pull request, a
  comment, a review, a fork or a star anywhere else, under any framing.
  [`docs/security/remote-ops.md`](../docs/security/remote-ops.md).

{{What it cost to learn: {{the incident, if there was one}}.}}

---

## 5. The tools this project has

⚠ **Reach for the purpose-built tool before the general one.** A general tool
used where a specific one exists produces answers that are plausible and wrong,
which is the hardest kind to catch.

| question | tool |
| --- | --- |
| what host is this, what is installed | `sh scripts/doctor/doctor.sh` |
| {{is the tree green}} | {{command}} |
| {{does the record agree with itself}} | {{command}} |
| {{an item closed, so the counts must move}} | {{command}}. ⛔ Never retype a count. |
| {{do the docs still resolve}} | {{command}} |
| {{what has this session done, measured}} | {{command}} |
| {{commit and push}} | {{command}}, and nothing else |

⛔ **An exit code is read from the process that produced it, unpiped.** Piping a
check into anything reports the pipeline's status, so a guard that failed reads
as green.

### The check contract

Every check in this project:

- a header comment saying **what defect it exists to catch**;
- exit **0** pass, **1** fail, **2** could not run;
- a json switch;
- no dependence on the directory it is run from.

⚠ **A check that measures an open defect must not fail the build for that
defect alone.** Record the count and judge it only past a stated ceiling. ⭐ The
other half of that rule is that the exemption comes off when the item closes.

---

## 6. The rules that bite most often

<!-- ⭐ This is the section that earns this file. Each entry: the rule, then
     what it cost. Grow it every time something bites. -->

### The record is part of the change

⛔ `PROGRESS.md`, {{the index}} and the item are edited in the **same change**
as the work, never after it. A session that fixes something and leaves the
record saying it is open has not finished the change; it has made the next
session read a lie first.

⭐ **This is enforced rather than remembered:** {{the gate that runs the record
check}}.

{{What it cost: {{the incident}}.}}

### Claims need evidence

⛔ A comparative claim without a committed benchmark does not ship. A flag that
does not move a number does not ship.

### A disproved premise gets a correction, not an edit

⛔ An item whose premise a measurement disproves keeps its title, and the
correction is written **underneath**. Never a silent edit of the premise.

### No deferral

⛔ Nothing closes as "won't fix", "upstream's problem" or "out of scope". A
blocked item stays open with the blocker named and what would unblock it.

⚠ Leaving a residual bound is allowed **only** when it is measured, named with
a file and a line, and carried as its own open item.

### {{Add a rule here every time something bites}}

{{The rule. Then: what it cost to learn.}}

---

## 7. Settled decisions, not to be relitigated

<!-- ⭐ Rewrite in place when one changes. Move the superseded wording to a
     history file with the date and the reason; do NOT stack a dated box under
     the old text. An agent that reads the first paragraph of such a box and
     stops acts on the retired rule, and that has happened.
     docs/conventions/prose.md. -->

- **{{The decision}}.** {{The ruling, and the date. Why the alternative lost.}}
