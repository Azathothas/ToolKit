# {{ID}}: {{title}}

<!-- THE FORM AN ENTRY IS WRITTEN FROM. Copy the whole of it into
     TODO/{{category}}.md and add its row to TODO/INDEX.md in the same change.
     Fill every {{PLACEHOLDER}} and delete every comment like this one.
     Authored per ../docs/methodology/authoring.md, and NOT filed until the
     operator approves it.
     ⛔ The title is how this entry will always be referred to. If a
     measurement later disproves the premise, the title STAYS and the
     correction is written underneath. -->

**Source** {{where the idea came from. Provenance, not a path a reader must be
able to open: "the operator", "issue 4", "found while measuring WSL-08".}}
**Category** {{category}}, **Priority** {{P0 | P1 | P2 | P3}}, **Effort** {{S |
M | L | XL}}, **Status** {{open | partial | blocked | done}}

---

## Problem

{{What is wrong, in terms of what a user or a script actually sees. Not the
implementation. The symptom.}}

## Premise

{{What is believed, and how it was checked. A premise that was READ rather than
MEASURED says so, in those words.}}

⚠ {{If this entry describes what the code does, the code is on disk. Measure
before building.}}

## Approach

{{What to do, with the seam named at file and line. The one existing path it
extends rather than forking.}}

⛔ {{What it must not do: the ceiling it must not design, the abstraction it
must not build.}}

## Decision

<!-- Delete this section if there is no fork. Where one exists, write it as a
     decision with a recommendation attached, so the operator rules on one
     question instead of reading an essay. -->

{{The fork. The recommendation. The reason the alternative lost. Blank until
ruled; a ruled entry carries the date and the ruling.}}

## Consumers

<!-- Delete this section only when the change cannot reach a fetched file.
     ⛔ It is here because this repository's files are fetched by URL from
     outside this tree, so nothing here fails when a contract breaks. -->

{{Which rows of ../docs/consumers.md this touches, and whether the change is
breaking by that file's definition. "None: this file has no consumers" is a
complete answer and is still written down.}}

## Prove

⛔ **The acceptance, and it is a command.**

```bash
{{the command}}
```

{{What counts as passing: the exit code, and the specific output. Read the code
from the process that produced it, unpiped.}}

Three rules this has to satisfy:

- it waits on the condition, never on a guessed duration;
- it does not assert a scheduling outcome it does not control;
- a comparative claim names the benchmark that produces the number, and if no
  such benchmark exists, writing it is part of this entry.

---

## Closing

<!-- Filled when the entry closes, in place, in the same change as the work. -->

**Closed {{ISO 8601 UTC}}.** {{What was done.}}

```text
{{the acceptance command's ACTUAL output, pasted}}
```

{{⛔ If a measurement disproved the premise above, the correction goes HERE,
underneath, never as an edit to the premise. Say what was believed, what was
measured, and what that changes.}}

{{⚠ If this is blocked rather than done, it stays open, with the blocker named
and what would unblock it.}}
