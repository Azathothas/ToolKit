# docs.md

The document set, what each one owns, and the rules that keep it trustworthy.

[`prose.md`](prose.md) is how they are written. This is which ones exist and
what makes them true.

---

## The set this repository actually has

⛔ **Every row names a file that exists.** This table used to list thirteen
documents, seven of which have never existed here, and one of those seven was
named as the authority a conflict between two documents is settled against. A
file nobody selected is a file a future session reads, believes, and follows
into a rule that was never meant to apply, and a file that does not exist at all
is worse: there is nothing to read and the rule pointing at it cannot be
followed.

| file | owns |
| --- | --- |
| [`../../AGENTS.md`](../../AGENTS.md) | the door. It says to read the router in full and states the absolutes, because a session may be handed it and nothing else. |
| ⭐ [`../AGENTS.md`](../AGENTS.md) | the router, read end to end. Where you are, the absolutes, the start of a session, what to read for which task, and which tool already exists. |
| [`../../README.md`](../../README.md) | what this is, for a competent stranger, and the map of everything else |
| ⭐ [`../../TODO/PROGRESS.md`](../../TODO/PROGRESS.md) | the record. What changed since last time and what is next. Nothing else carries a work order. |
| [`../../TODO/RULES.md`](../../TODO/RULES.md) | the half of the record that does not change between sessions: the standing facts, and the rules that are this repository's own |
| [`../../TODO/INDEX.md`](../../TODO/INDEX.md) | every entry, one line each, with the counts a check holds |
| [`../../TODO/ENTRY.md`](../../TODO/ENTRY.md) | the form an entry is written from |
| [`../../CHANGELOG.md`](../../CHANGELOG.md) | what shipped, when, and where the evidence is |
| ⭐ [`../consumers.md`](../consumers.md) | the technical reference for the thing that makes this repository different: who fetches from it and what breaks them. **When a document conflicts with it about a consumer, it wins and the other is the defect.** |
| a tool's `.md`, beside the tool | what that tool does, in full, for a reader who has opened nothing else |

⚠ **Two roles this set deliberately leaves empty.** An operator-facing runbook
and a threat model are both worth having and neither has content yet, so neither
exists. [`../../TODO/PROGRESS.md`](../../TODO/PROGRESS.md) carries that as an
open question rather than shipping an empty skeleton for each.

⛔ **There is no architecture document, and nothing here should claim one.**
This repository is a set of independent tools, each with its own page; there is
no shared schema, state machine or layer rule for such a page to describe. The
per-tool page is the technical reference for its tool.

---

## The invariants

### One fact, one home

Every fact lives in exactly one document. A version string, a constant, a rate
limit, a schema: one place.

⛔ **A value in two documents with no check between them drifts**, and the copy
a reader trusts is the wrong one. If a number must appear twice, derive it from
the source, or have a check assert the two agree.

⚠ The trap is that a value which never changes cannot expose a missing check.
It sits correct for a year and drifts the first time it moves.

### The technical reference wins

⛔ **Every fact has a document that owns it, and when two disagree the owner is
right and the other is the defect.** Fix it in the same change and say so in the
record rather than leaving the reader to work out which is live.

⚠ **Which document owns a fact is answered by the table above**, not by which
one a reader found first. For a tool's behaviour it is that tool's own page; for
who fetches a file and what breaks them it is
[`../consumers.md`](../consumers.md); for what changed last session it is the
record.

### Documentation ships with the code it describes

⛔ Doc and code drifting apart is a forbidden pattern. The moment code changes a
documented behaviour, the document changes with it. In the same commit, not
later.

### Every claim is verified before it is written

Writing the documentation is the audit. Being forced to say precisely what
something does, and then checking whether that is true, is where a surprising
share of real defects are found.

⚠ The most confident sentence in a file is regularly the only false one. A test
file header asserting it ran "exactly as production uses it" hid the gap that
shipped a server error for six units of work.

### Prefer a shape a check can assert

Where a document names a file, a constant, a route or an identifier, prefer a
form a check can verify against the tree, so a rename fails a gate instead of
rotting quietly.

⭐ The strongest version of this is a catalogue where each entry declares which
files read it, and a check opens those files and looks. That is a document that
reviews itself.

⚠ **A document that cannot be checked is a document that drifts.** That is not
an argument against writing prose. It is an argument for making the mechanical
parts mechanical, so the reading is spent on the parts that need it.

### Say what is not true

Reserve a place for the truths that are tempting to hide. This is slower than
it looks. This has a known gap. This estimate excludes something unmeasurable.

⛔ A limit hidden is a defect filed against the user later.

---

## Lessons, and where they go instead of a lessons file

⛔ **There is no `lessons.md` here, and that is a decision rather than an
omission.** A running log of what worked and what bit is real institutional
memory, and this repository already keeps it in three places that are each
better at one part of the job:

| a lesson that is | goes to | because |
| --- | --- | --- |
| grep-able, and cost something | [`forbidden-patterns.md`](forbidden-patterns.md) | a reader greps themselves against it before calling a gate green, which a log does not get read for |
| mechanical | ⭐ a check, and a row pointing at it | a rule enforced by a script is a rule nobody has to remember |
| a measurement, or a rejected approach | the entry that produced it, in `TODO/` | it keeps the conditions and the acceptance command beside it, which a log strips |
| about somebody else's project | [`../reference-sweeps/usable.md`](../reference-sweeps/usable.md) | that page exists to say which findings this repository can act on |

⚠ **The cost of not having one** is that there is no single page to read for
orientation. ⭐ The record's own summary is what covers that instead, and it is
rewritten every session rather than appended to.

---

## The changelog

**What shipped, when, and where the evidence is.** One entry per shipped unit
of work, pointing at the record that carries the detail.

⭐ It is also the destination for what a documentation pass removes. When a
document loses the *story* of a fix, what broke and what the sentence used to
say, the story comes here. So this file is expected to grow, and its length is
not a defect.

| the text is | where it goes |
| --- | --- |
| a fact, limit or constraint a future session needs | ⛔ the document. Not here. |
| a measurement with its conditions | ⛔ the document, as a table. Not here. |
| the story of a fix, or a superseded claim kept for provenance | ⭐ here |
| the full detail of one session's work | ⛔ the handoff. Here goes a pointer to it. |

Four rules, and [`scripts/common/check-changelog.sh`](../../scripts/common/check-changelog.sh)
holds all four. ⭐ They were stated here and enforced by nothing for as long as
this document existed, which is the shape a rule takes on its way to becoming
a preference:

1. ⛔ **Newest first, always.** A new entry goes at the top of its section,
   never appended to the bottom.
2. ⛔ **Every heading carries a date.** Consider a full ISO 8601 UTC stamp:
   several entries sharing one date cannot be ordered from what was written
   down.
3. ⛔ **Every entry names its record**, the handoff or plan or commit carrying
   the evidence. An entry with no record is a claim.
4. ⛔ **Every entry says whether it deployed.** "No version bump and no deploy"
   is a complete and common answer. Silence is not.

And two things an entry must not do:

- ⛔ **Do not tidy the file while shipping something else.** Reordering old
  entries in the commit that adds a new one makes both unreviewable. Tidying is
  its own commit.
- ⛔ **Do not delete an entry.** A superseded one is amended in place with a
  dated note. Amend, never silently delete.

⚠ A check can hold the order, the dates and the pointers. **It cannot check
that an entry is true.** That stays with the claim audit,
[`../methodology/reviews.md`](../methodology/reviews.md) lens 3.
