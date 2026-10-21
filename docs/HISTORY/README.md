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

## ⛔ Claims this repository published and withdrew

Read this before trusting a sentence elsewhere in the tree. Each row is a claim a
page or a record made, what is true instead, and the entry that corrected it.

| the claim | where it was | what is true | corrected by |
| --- | --- | --- | --- |
| every cited path in every document resolves, and the gate asserts it | the record's review of 2026-09-18 | the gate's cited-path rule had an empty body from 2026-09-09; links were checked, cited paths were not | `TOOL-30` |
| the full gate runs in about 30 seconds | `check-gate.sh` and its twin; `docs/AGENTS.md` said under a minute | 131 to 245 seconds on the host that measured it | `TOOL-31` |
| findings 12, 13 and 15 are fixed | the record's finding 95, and a comment in `base.go` | four guest errors were still named by their first line | `WSL-152` |
| ephemeral removes the container and the job directory | `run --help` | it removed the container, and the host record stayed until `gc` | `WSL-121` |
| the release ran 8 jobs; CI makes six runs | the 7.0.0 session table; `TODO/RULES.md` | 7 release jobs; 8 CI runs | `DOC-13` |
| `TOOL-14` removed `check-twins` | the record's finding 3 | it kept it, and CI runs it | the record's rewrite, `DOC-10` |
| the adapter contract stops an agent's binary being found before its wrapper | the record's finding 49 | its own example probe passes with no wrapper | `WSL-145` |

## The pages

| file | what it holds |
| --- | --- |
| [`wsl-toolkit.md`](wsl-toolkit.md) | the defects `wsl-toolkit.ps1` shipped and closed, and the shapes its behaviour used to have. ⚠ The tool was `wsl-ephemeral.ps1` until 2026-08-30; that page says so at the top. |
| [`consumers.md`](consumers.md) | how each consumer pin came to move, and what was measured while moving it. The live page keeps the pin STATE, which is the part a consumer's owner acts on. |
| [`scripts.md`](scripts.md) | what `check-markers` and `check-one-home` counted the day each was first armed |
| [`dated-passages.md`](dated-passages.md) | every dated paragraph and measurement table the live pages carried until `DOC-08`, verbatim |
| [`record.md`](record.md) | `TODO/PROGRESS.md` as it stood until it was rewritten to the model, with the index's ordering argument and four session tables, verbatim. A `finding N` or a `ruling N` cited in code or an entry is a numbered item there. `DOC-10` |

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
