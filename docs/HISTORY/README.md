# docs/HISTORY

⛔ **Nothing here is read to do work.** It is where the *story* of a change goes
when the live page keeps only what is true now.

⭐ **If you are working, close this and read the live page instead.** Every file
in here is superseded wording, a fix that has shipped, or a measurement taken
against a tree that has moved. Acting on any of it is acting on something that
was true once.

---

## Why it exists rather than being deleted

⚠ **A superseded rule is moved, never dropped.** A future session that wonders
why a rule is what it is can then find out instead of re-deriving it wrongly,
and re-deriving it wrongly is how a rule that cost something gets removed by
somebody who never paid.

⛔ **And it is moved rather than left in place.** A document written by
accretion, where the paragraph says one thing and a box below it says the
opposite, has a documented failure mode: a reader takes the first answer and
acts on the retired rule.
[`../conventions/prose.md`](../conventions/prose.md) is the rule that sends
wording here, and it is the rule this directory implements.

---

## What goes here, and what does not

| the text is | where it goes |
| --- | --- |
| the story of a fix: what broke, on what date, what the sentence used to say | ⭐ here |
| a rule that has been rewritten, kept so its reasoning survives | ⭐ here |
| a measurement whose conditions no longer exist | ⭐ here |
| a fact, a limit or a constraint a future session needs | ⛔ the live document. Not here. |
| a mistake that is worth grepping yourself against | ⛔ [`../conventions/forbidden-patterns.md`](../conventions/forbidden-patterns.md). That table is deliberately a list of incidents, and it stays live because it is read before a gate is called green. |
| what shipped, when, and where the evidence is | ⛔ [`../../CHANGELOG.md`](../../CHANGELOG.md), which points at the record |
| what one session did | ⛔ [`../../TODO/PROGRESS.md`](../../TODO/PROGRESS.md) and the entry it closed |

⛔ **A page here is exempt from the one-fact-one-home check**, by name, in both
halves of `check-one-home`. That is the point of the directory: it holds
sentences the live pages used to carry.

---

## The pages

| file | what it holds |
| --- | --- |
| [`wsl-toolkit.md`](wsl-toolkit.md) | the defects `wsl-toolkit.ps1` shipped and closed, and the shapes its behaviour used to have. ⚠ The tool was `wsl-ephemeral.ps1` until 2026-08-30; that page says so at the top. |
| [`consumers.md`](consumers.md) | how each consumer pin came to move, and what was measured while moving it. The live page keeps the pin STATE, which is the part a consumer's owner acts on. |
| [`scripts.md`](scripts.md) | what `check-markers` and `check-one-home` counted the day each was first armed |
| [`dated-passages.md`](dated-passages.md) | every dated paragraph and measurement table the live pages carried until `DOC-08`, verbatim |

⭐ **`DOC-06` closed on 2026-08-30 and the purge is applied**, so the four files
it named carry constraints and no longer carry diaries.

⚠ **`DOC-06` drew a line that let a measurement STAY on a live page when a reader
who did not know it would undo the rule. `DOC-08` replaced it on 2026-09-27**:
a live page carries no date at all, states the constraint in the present tense,
and cites the entry that holds the measurement. The `ste` check refuses a date
in the prose of a live page.

⭐ **Prior art.** The shape is `pkgforge-dev/docker-archlinux`'s `HISTORY/`,
recorded in
[`../reference-sweeps/findings.md`](../reference-sweeps/findings.md). It keeps
the directory at the repository root; this one is under `docs/` so the whole of
`docs/` is the thing a session is routed around rather than into.
