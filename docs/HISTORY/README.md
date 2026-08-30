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
| [`wsl-ephemeral.md`](wsl-ephemeral.md) | the defects `wsl-ephemeral.ps1` shipped and closed, and the shapes its behaviour used to have |

⚠ **One page, and the directory is not finished.** `scripts/README.md`,
[`../consumers.md`](../consumers.md), [`../conventions/docs.md`](../conventions/docs.md)
and [`../conventions/prose.md`](../conventions/prose.md) still carry the story of
their own fixes on the live page. `DOC-06` in [`../../TODO/docs.md`](../../TODO/docs.md)
is that work, it is open, and it names the four files.

⭐ **Prior art.** The shape is `pkgforge-dev/docker-archlinux`'s `HISTORY/`,
recorded in
[`../reference-sweeps/findings.md`](../reference-sweeps/findings.md). It keeps
the directory at the repository root; this one is under `docs/` so the whole of
`docs/` is the thing a session is routed around rather than into.
