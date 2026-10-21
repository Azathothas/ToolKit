# TODO: docs

Entries for this repository's documents and the rules they hold themselves to.

[`INDEX.md`](INDEX.md) is the list; [`PROGRESS.md`](PROGRESS.md) is the order.

---

## DOC-02. The tree broke its own character rule in 164 places

**Source** Issue 2, which asks for every document, the readme and the comments
in code to be checked and fixed. Found by arming the check in `TOOL-04`.
**Category** docs, **Priority** P2, **Effort** S, **Status** done

**Problem.** [`../docs/conventions/prose.md`](../docs/conventions/prose.md)
allows five characters outside ASCII and asks that they be used sparingly
enough to stay visible. The tree kept neither half, and `check-docs.sh`
reported it clean the whole time because it reads markdown alone.

**Premise.** ⭐ **Measured, not asserted.** First run of `check-markers.sh` on
this tree, 2026-08-29, before anything was changed:

```text
marker check failed, 167 problem(s)
```

164 of those were characters and 3 were files over the density ceiling. Every
character was in a comment banner in a script, in five shapes. ⚠ The counts are
of OFFENDING LINES, which is what the check reports; a line carrying three of
the same character is one row:

| codepoint | what it was | lines |
| --- | --- | --- |
| U+2500 | a box-drawing rule in a comment banner | 102 |
| U+00B7 | a middle dot used as a list separator | 35 |
| U+2022 | a bullet in a comment list | 21 |
| U+2713 | a check mark that is not the defined status glyph | 3 |
| U+2026 | an ellipsis | 3 |

**Approach.** A one-off transform over every tracked text file, mapping each
shape to what it was standing in for: the box-drawing rule to a hyphen, the
bullet to a hyphen, the ellipsis to three dots, the separator dot to a comma,
and the check mark to `✅`, which is the status glyph it was imitating.
`Azathothas/TEMPLATE` had already made the same change to its own copies of
these scripts, so the result agrees with upstream rather than diverging from it.

⛔ **The density half is not a transform.** `TODO/bsd.md` was at 33 markers per
100 non-blank lines. The markers removed there are the ones inside table cells,
where the cell's own words already carry the verdict: `⭐ **met.**` says nothing
`**met.**` does not. The first cell of a row keeps its marker, because that is
where one legitimately says which row to reach for.

**Consumers.** None. No behaviour, no path and no exit code changed, and
`check-twins.sh` compares the two halves of every pair on their answers.

**Prove.**

```bash
sh scripts/common/check-markers.sh
```

Exit 0, and the summary line names the densest file and its figure.

---

### Closing

**Closed 2026-08-29T15:18:27Z.** 4,633 replacements across 28 files, then 49
markers removed from table cells in `TODO/bsd.md`.

```text
markers ok: 76 files, 2435 markers, densest 29 per 100 non-blank lines
(TODO/SUMMARY.md), ceiling 30
```

⚠ **The record files were the last two over the ceiling** and they are rewritten
every session, so they came under it as part of writing this session's record
rather than as a separate edit.

⚠ **The figure above is the CLOSING run, not the first green one.** The record
is inside the set the check measures, so writing the record moves the number.
A reader re-running the command gets this, and gets a different total the moment
they write anything themselves. ⛔ The acceptance is the exit code; the totals
are context.

---

## DOC-03. Seventeen sentences had two homes

**Source** Issue 2, which says sections were copied verbatim from
`Azathothas/TEMPLATE` into a repository that is not a template. Found by arming
the check in `TOOL-04`.
**Category** docs, **Priority** P2, **Effort** S, **Status** done

**Problem.** A fact with two homes drifts, and the copy a reader trusts is
whichever they saw first. Nothing here compared two documents.

**Premise.** ⭐ **Measured on 2026-08-29**, before anything was changed: 17
sentences of twelve words or more appeared in more than one document. Seven of
the seventeen involved a skeleton under `docs/templates/`, which this repository
had copied across and never filled in.

**Approach.** For each, decide which document owns the fact and make the other
a pointer. ⛔ Not by deleting the second copy: a pointer that says where the
rule lives is what stops the same paragraph being re-added.

The seven involving the skeletons are answered by `DOC-04`, which removes them.
The remaining ten are edits: the idling rule to `shell.md` section 10, the
disproved-premise rule to `authoring.md`, the probe's one-line description to
`scripts/doctor/README.md`, the empty-review-pass rule to `reviews.md`, the 5.1
quoting measurement to `shell.md` section 8, the sweep's scope to
`reference-sweeps/findings.md`, and the `lsf` mechanism to the same file.

**Consumers.** None. Documentation only.

**Prove.**

```bash
sh scripts/common/check-one-home.sh
```

Exit 0.

---

### Closing

**Closed 2026-08-29T15:18:27Z.**

```text
one fact one home: 37 documents, no sentence of 12+ words in two of them
```

⚠ **It compares sentences, so this is not a claim that nothing is restated.** A
fact put in different words passes this check and is a finding for a review.

---

## DOC-04. The template skeletons go, and TODO/ becomes the shape the model names

**Source** Issue 2, whose title is that `docs/templates/*` still quote verbatim
double-brace placeholders. Ruled by the operator on 2026-08-29: adopt the proper
`TODO/` directory instead.
**Category** docs, **Priority** P1, **Effort** S, **Status** done

**Problem.** `docs/templates/` held nine fill-in skeletons inherited from
`Azathothas/TEMPLATE`. This repository is not a template and fills none of them.
Eight were skeletons for files it already has, or has decided it does not want.
⚠ The failure is not that they are untidy: a future session reads one, believes
it, and follows it into a rule that was never meant to apply here.

**Premise.** Read, on 2026-08-29. All nine carried double-brace placeholder
markers and guidance comments addressed to whoever was filling them in. Seven of
the seventeen two-home sentences in `DOC-03` involved one of them.
[`../docs/methodology/work-todo.md`](../docs/methodology/work-todo.md) names the
shape a todo directory has: a record, an index, the entries, and `RULES.md`.
This tree had no `RULES.md`.

⚠ **The sentence above describes the marker rather than showing one, on
purpose.** A page that demonstrates the thing a checker looks for makes the
checker fire on correct writing, which is the same class as a document about
escape sequences containing the byte it warns about.

**Approach.** Delete `docs/templates/` entire. Move the one skeleton this
repository authors from to [`ENTRY.md`](ENTRY.md), beside the entries it
produces, with its links rewritten for where it now lives. Write
[`RULES.md`](RULES.md) for real: the standing facts, and the rules that are this
repository's own rather than a convention's.

⛔ **Move the exemptions with the files.** `check-placeholders` exempted three
directories, two of which never existed here and one of which is now gone;
`check-docs` exempted a template directory from link resolution; `check-changelog`
carried a note about a skeleton. Each is removed rather than emptied. ⚠ An
exemption for a path that does not exist is dead configuration, and it grants
itself to whatever lands there next.

**Decision.** ⭐ **Ruled by the operator, 2026-08-29.** The question put was
whether to keep the entry form under `docs/templates/`, delete the directory
entirely, or de-template all nine. The answer was to adopt the proper `TODO/`
directory. ⚠ `HUMAN.md` and `SECURITY.md` are still not written: neither is part
of the shape the work model names, and an empty skeleton outlives the session
that wrote it.

**Consumers.** None. Nothing outside this tree fetches a document.

**Prove.**

```bash
sh scripts/common/check-placeholders.sh
```

Exit 0, and the summary names `TODO/ENTRY.md` as the only exemption.

---

### Closing

**Closed 2026-08-29T15:18:27Z.**

```text
no placeholders survived in 89 files (TODO/ENTRY.md is exempt)
```

⭐ **Both halves agree**, which is what the exemption change had to preserve:
the `.ps1` twin printed the same line and exited 0.

---

## DOC-05. `docs/AGENTS.md`, and what `README.md` is for

**Source** Issue 3: `docs/AGENTS.md` is missing and has been turned into an
unusable `docs/README.md`.
**Category** docs, **Priority** P1, **Effort** M, **Status** done

**Problem.** An agent working here had no single document to read in full. The
root `AGENTS.md` was a router that restated nothing, and `docs/README.md` was a
table of one-line summaries pointing at other files. Neither can be read end to
end and neither leaves a reader oriented, so the reading either does not happen
or happens twenty files at a time.

**Premise.** ⛔ **The issue's own framing was checked and is not literally
true.** It says `docs/AGENTS.md` should exist "as per the TEMPLATE".
`Azathothas/TEMPLATE` at `6eaf4b5` has no such file: it has `docs/README.md`,
which is exactly what was copied here, and a `docs/templates/AGENTS.md`
skeleton for a project's own root router. ⭐ The ask is right and the citation
is not, which is what re-deriving a claim is for.
[`../docs/security/remote-ops.md`](../docs/security/remote-ops.md) is the rule.

**Approach.** ⭐ **Ruled by the operator, 2026-08-29:** `README.md` is for
people, technical and concise, and takes the map; `docs/AGENTS.md` is for agents
and routes properly.

- `docs/README.md` becomes `docs/AGENTS.md` by rename, and is rewritten as the
  document read in full: where you are, the absolutes, the sequential start of
  session, the routing table, the tool inventory, what a session owes, and what
  to do when unsure.
- The root `AGENTS.md` becomes a door: read `docs/AGENTS.md` in full, read the
  record, and the seven absolutes, because a session may be handed that file and
  nothing else.
- `README.md` takes the document map and the tree, for a competent stranger.

⛔ **The two routers are the only files allowed to say the same thing**, and
`check-one-home.sh` holds exactly that exemption and no wider.

**Consumers.** None. ⚠ `Azathothas/bit-cli` links this repository's raw script
path rather than any document, and that path did not move.

**Prove.**

```bash
sh scripts/common/check-docs.sh
```

Exit 0, with no broken link and no page that nothing links to.

---

### Closing

**Closed 2026-08-29T15:18:27Z.**

```text
docs ok: 37 files, 379 relative links, 105 shell blocks. Links and prose clean.
```

⚠ **The orphan rule is what proves the rename landed.** `docs/README.md` is gone
and every one of its links was re-pointed; a page left behind would have been
reported as linked from nowhere, and `TODO/RULES.md` was, until the router
gained a row for it.

---

## DOC-06. the documents carry the story of their own fixes

**Source** the operator, 2026-08-30.
**Category** docs, **Priority** P1, **Effort** M, **Status** done

---

## Problem

⭐ **Stated by the operator in the words that matter:** an agent does not need to
know that xyz was fixed on abc. It will never hit the defect, because it is
fixed, and the sentence costs it context to read. Several pages here are a
reference and a diary at the same time.

## Premise

⚠ **The rule already existed and had no destination.**
[`../docs/conventions/prose.md`](../docs/conventions/prose.md) says superseded
wording moves to a history file rather than staying as a box on the live page,
and `check-one-home` already carried an exemption for a `docs/history/`
directory that did not exist.

## Approach

`docs/HISTORY/`, with a `README.md` that says what belongs there and what does
not, and one page per subject. The live page keeps the constraint and loses the
story. `check-one-home` exempts the directory in both halves, so a retired page
may hold sentences the live pages used to carry.

⛔ It must not delete. A superseded rule is moved so a future session can find
out why the rule is what it is instead of re-deriving it wrongly.
⛔ `forbidden-patterns.md` stays live. That table is deliberately a list of
incidents and it is read before a gate is called green.

## Prove

```bash
sh scripts/common/check-gate.sh --fast
```

Exit 0, with `check-one-home` and `check-docs` green over the new directory, and
no live page carrying a sentence about what something used to do.

---

## Closing

⚠ **PARTIAL, and this entry stays open.** The directory, its `README.md`, the
check exemption in both halves, and `HISTORY/wsl-ephemeral.md` shipped on
2026-08-30, and ten passages were moved off
`scripts/powershell-windows/wsl-ephemeral.md`.

⛔ **What was NOT done**, named rather than left to be discovered:

- `scripts/README.md` still carries "here is what that cost" sections and the
  164-characters-in-28-files measurement;
- `docs/consumers.md` still carries the whole pin-move narrative;
- `docs/conventions/docs.md` and `prose.md` still carry the story of rules that
  were stated and enforced by nothing;
- `prose.md` and `docs.md` were not amended to name `docs/HISTORY/` as the
  destination, so the rule still points at a generic "history file".
---

## Closing

⚠ **PARTIAL on 2026-08-30, and closed the same day.** The first attempt shipped
the directory, its `README.md`, the check exemption in both halves, and
`HISTORY/wsl-toolkit.md`, and ran out of budget with four files unpurged. This
records both halves rather than rewriting the first.

**What the second pass did**, which is exactly the list the partial closing
named:

| file | what moved, and where |
| --- | --- |
| `scripts/README.md` | the two armed-run counts, 164 characters in 28 files and 17 sentences with two homes, to [`../docs/HISTORY/scripts.md`](../docs/HISTORY/scripts.md) |
| `docs/consumers.md` | the whole pin-move narrative, to [`../docs/HISTORY/consumers.md`](../docs/HISTORY/consumers.md) |
| `docs/conventions/docs.md` | the "stated here and enforced by nothing" clause, deleted: the four rules are enforced now and the sentence described a state that no longer exists |
| `docs/conventions/prose.md` | amended to name `docs/HISTORY/` as the destination, which it previously called "a history file" with nowhere to point |
| `docs/conventions/docs.md` | gained a row sending retired wording to `docs/HISTORY/`, so the lessons table names it |

⭐ **THE LINE THIS DREW, because it is the part a later session will have to
apply rather than copy.** A measurement stays on a live page when a reader who
does not know it will undo the rule; it moves when it is a count from a tree that
has since been fixed.

- ⭐ **Stayed:** `Sort-Object -u` dropping two of four distinct values. A reader
  who does not know that will remove the twins rule as redundant.
- ⛔ **Moved:** "before it was armed, 164 characters across 28 files". The
  characters are gone, the check is armed, and there is nothing to act on.

```text
docs ok: 42 files, 454 relative links, 116 shell blocks. Links and prose clean.
one fact one home: 38 documents, no sentence of 12+ words in two of them
```

⚠ **`check-one-home` earned its keep during this pass**, which is the strongest
thing that can be said for a check. Two sentences written into
`scripts/windows/wsl-toolkit/README.md` were also in `scripts/README.md`, and it
named both files and both sentences. The tool's own page kept them and
`scripts/README.md` became a pointer.

---

## DOC-07. there are two AGENTS.md files

**Source** the operator, 2026-08-30, asking a second time.
**Category** docs, **Priority** P2, **Effort** S, **Status** done

---

## Problem

`AGENTS.md` at the root and `docs/AGENTS.md` both exist. The root one is a door
that restates the seven absolutes; the other is the router. The operator wants
one, and it is `docs/AGENTS.md`.

## Premise

⚠ **Measured while attempting it.** `check-one-home` exempts the two from each
other **by name**, because each states the absolutes in full. Removing the root
file is therefore three changes and not one: delete it, remove the exemption
from both halves of the check, and repoint every reference. `README.md`,
`docs/AGENTS.md`, `docs/conventions/docs.md` and `TODO/RULES.md` link to it.

⚠ **The cost of keeping it is not zero either.** A harness that opens `AGENTS.md`
on its own finds the door; with only `docs/AGENTS.md` it finds nothing, and
whether that matters is a question about the harness rather than about this
tree.

## Approach

Delete `AGENTS.md`. Move whatever a cold session genuinely needs into
`docs/AGENTS.md`, which already carries all of it. Remove the router exemption
from both halves of `check-one-home` and from its header comment. Repoint the
four references.

## Prove

```bash
sh scripts/common/check-gate.sh --fast
```

Exit 0, with `check-docs` reporting no dead link and `check-one-home` green with
no exemption in it.
⚠ Started and reverted on 2026-08-30: the check change was made and backed out
when the session pivoted, because a check exempting a file that no longer exists
and a file whose sentences are duplicated are two different broken states and
neither is worth leaving behind.

---

## Closing

**Closed 2026-08-30T09:05:00Z.** ⚠ **Ruled by the operator in this session, and
the question was put to them first**, because their own kickoff opened with
"read ./AGENTS.md & then ./docs/AGENTS.md", which is evidence they still use the
root file. They chose to delete it anyway.

Three changes, which is what the premise said it would be:

1. `AGENTS.md` deleted.
2. The router exemption removed from both halves of `check-one-home`, from the
   table AND from the header comment that explains it.
3. Five references repointed: `README.md` twice, `docs/AGENTS.md`'s own header,
   `docs/conventions/docs.md`'s document table, `scripts/README.md`'s
   explanation of the exemption, and `TODO/tooling.md`.

```text
docs ok: 42 files, 454 relative links, 116 shell blocks. Links and prose clean.
one fact one home: 38 documents, no sentence of 12+ words in two of them
```

⛔ **The exemption was deleted rather than emptied**, and that is the part worth
keeping. An exemption written for a path that no longer holds anything grants
itself to whatever lands there next, which is the `DOC-04` row in
`docs/conventions/forbidden-patterns.md`. Both halves now name one router.

⚠ **THE COST IS REAL AND IT IS NOT HYPOTHETICAL.** A harness that opens
`AGENTS.md` on its own now finds nothing, and `docs/AGENTS.md` says so in its own
first paragraph so a session that arrives some other way is told. A session
pointed at nothing has to be pointed at `docs/AGENTS.md` by whoever starts it.
That is now a line in the next-session prompt rather than a property of the tree.

## DOC-08. The live pages carry dated measurements and the story of their own work again

**Source** the operator, 2026-09-27: "all docs/skills etc are amended in place,
carry no dated/stale/narrative lore, and read as ASD STE100 technical manual".
**Category** docs, **Priority** P2, **Effort** L, **Status** done

---

## Problem

`DOC-06` moved the story of old fixes off the pages, and the class came back. A
reader of the tool's usage page meets "measured on 2026-09-14" rows, "the first
published nightly", "this page claimed ... before the tool did", and sentences
that describe what a release used to do. A reader who wants one fact reads a
diary to find it.

## Premise

Counted on 2026-09-27 with `git grep` for a `YYYY-MM-DD` date over the live
pages (`docs/` without `HISTORY/` and `reference-sweeps/`, `skills/`, the tool
pages, the READMEs and the examples): **93 lines across 18 pages**, and
`tools/windows/wsl-toolkit/wsl-toolkit.md` alone carries **51** of them. ⚠ **`DOC-06`'s own line cannot hold the stricter rule**: it kept a
measurement on a page when "a reader who does not know it will undo the rule",
which is most measurements.

## Approach

1. Each live page states behaviour, limits and commands in the present tense,
   in STE sentences, with no dates, no session history and no "used to".
2. ⭐ A dated measurement moves to the entry that produced it, and the page cites
   that entry by id. A constraint a reader needs stays, as a present-tense fact.
3. [`../docs/conventions/prose.md`](../docs/conventions/prose.md) and
   [`../docs/conventions/docs.md`](../docs/conventions/docs.md) are amended in
   place to state that rule.
4. A check refuses a date on a live page, so the class cannot come back a third
   time without a red gate.

⛔ **The record is not a live page.** `TODO/`, `CHANGELOG.md` and `docs/HISTORY/`
are evidence of what was believed on a date, and they keep their dates.

## Decision

**Ruled 2026-09-27, the operator's recommended option:** the entries hold the
measurements and the pages cite them. The alternative was one dated appendix per
page.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0, with the new check green over the live pages and red when a date is
planted in one.

---

## Closing

**Closed 2026-09-27T10:55:59Z.** The check's own answer over the live pages:

```text
sh scripts/common/check.sh ste --json
{"files":35,"longest_sentence":25,"problems":0,"schema":"check-ste/1","sentences":5261}
```

| | result |
| --- | --- |
| dates the new rule found on its first run | **47**, on 16 pages |
| dates the old span mask had hidden from every rule | **3** more, all on `docs/conventions/forbidden-patterns.md` |
| dates on the live pages now | **0** |
| live pages rewritten | **28**: every date, release-by-release story and past-tense anecdote out, and each measured claim citing its entry |
| passages moved | 90 blocks from 18 pages to `docs/HISTORY/dated-passages.md`, verbatim; the consumer page's release history to `docs/HISTORY/consumers.md` |

⭐ **The rule can fail.** A date planted in prose, in a table row and in a heading
is a finding, and one inside code is not. Two mutation rows went red: the rule
removed, and tables masked again. `prose.md` and `docs.md` state the rule in
place, and the gate runs it.

⛔ **The rewrite found a defect in the check itself.** Its code-span mask paired
backticks across the whole document, so one double-backtick span shifted every
later pair and masked the rest of the page as code. No rule had read the tail
of `forbidden-patterns.md`. A span does not cross a blank line now, and a
double-backtick span is read first; two more rows prove both.

⚠ **Not in scope, and said so on `prose.md`:** a comment in code keeps its
dates. It is the maintainer's record beside the code, not a page a reader
follows.

## DOC-09. The methodology is a month behind TEMPLATE

**Source** the operator, 2026-10-21, ruling 31
**Category** docs, **Priority** P2, **Effort** M, **Status** done

---

## Problem

`docs/methodology/` was taken from TEMPLATE at `3191d08`. TEMPLATE has since
added pages, among them `history.md` and `template-sync.md`, renamed `references.md`, and
changed five pages this repository also edited, `sessions.md` gaining a resume file
written at the start.

## Premise

Measured on 2026-10-21: each page diffed against TEMPLATE at `3191d08` and at `a970fe5`.

## Approach

TEMPLATE's own sync procedure: take the pages missing here, take a page unchanged
here, merge a page this repository edited, never restore a page the bootstrap dropped,
and pin the commit taken in `RULES.md`.

## Consumers

None.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** TEMPLATE's own procedure, three-way, with the adopted commit
as the base. `gate.md`, `initialize.md`, `reviews.md` and `sessions.md` are merged:
the first two cleanly, the rest with one conflict each, resolved toward this
repository's edit where it was the considered one. `work-todo.md` keeps this
repository's side. Section 10 of `docs/conventions/shell.md` is taken whole, unchanged
here since adoption. `history.md`, `lean-adoption.md`, `template-sync.md` and
`vendoring.md` are new, and `research.md` replaces `references.md` with this
repository's one edit carried over. The stage model, the model chooser and the ingest
page stay out: the bootstrap dropped them, and the procedure forbids restoring them.
`RULES.md` pins the commit taken.

```text
for c in docs ste markers one-home secrets placeholders skills: exit 0 each
pwsh -NoProfile -File scripts/common/check-gate.ps1     exit 0, 24 of 24
```

⚠ **Adapted, and said where.** TEMPLATE's text was changed where it named a file this
repository does not have, a flag its gate does not take, a date in a live page, or a
word the prose check refuses. `history.md` says this repository's checks exempt
`docs/HISTORY/` from three more rules than TEMPLATE's do. `vendoring.md`'s pointer to
the tracker section said 3.4 and is 3.3. ⛔ **Two differences are left as decisions**:
`research.md` asks for a corpus to be kept, and `.gitignore` keeps the clones out of
the tree while the derived write-ups are tracked, which is this repository's own rule;
and `research.md`'s `mine-repo.sh` stays TEMPLATE's helper, not this repository's.


## DOC-10. The record carries history, a findings list and every session's measurements

**Source** the operator, 2026-10-21: "clean this up and follow TODO properly from now on"
**Category** docs, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`PROGRESS.md` was 2,776 lines, and the work order a reader needs was 41 of
them. Its "Found, and not filed" list was the deferral the model forbids, and the index
and the summary carried history too.

## Premise

Measured on 2026-10-21; `TOOL-29` has the numbers.

## Approach

Rewrite `PROGRESS.md` to the model's parts, file every finding still open as an
entry, move the old text word for word to `docs/HISTORY/record.md`, move the rulings in
force to `RULES.md` section 8, and overwrite `SUMMARY.md`.

## Consumers

None.

## Prove

```bash
sh scripts/common/check.sh record
```

Exit 0.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** `PROGRESS.md` carries the model's parts and is 110 lines. The
old one, the index's ordering argument and four session tables are in
`docs/HISTORY/record.md` word for word, headings one level down and links rewritten
to resolve. Five read-only passes sorted the old record against the tree; every
finding still open is one of 58 entries, and the rulings in force are `RULES.md`
section 8 under the same numbers. `SUMMARY.md` carries the last session alone, and
the index argues the current order. 51 comments and pages citing the old record by
path point at its new home, except three in files the builders are changing, done
when those merge. `docs/HISTORY/README.md` lists the claims this repository withdrew.

```text
sh scripts/common/check.sh record     exit 0
  ok     record
wc -l TODO/PROGRESS.md               110
```


## DOC-11. Lessons the old record held have no live page

**Source** the classification of the old record, 2026-10-21
**Category** docs, **Priority** P3, **Effort** S, **Status** open

---

## Problem

Several rules the old record learned live nowhere a reader follows: an assertion
that needs a program the image may lack, `agent wait --until idle` on an agent already
idle, `$LASTEXITCODE` after a pipe into a native command, a StrictMode read that disarms
a cleanup, three review lenses, and a durable record that says "this session".

## Premise

Read on 2026-10-21 by the classification; each has its finding in `docs/HISTORY/record.md`.

## Approach

Each becomes a row or a sentence on the live page that owns its subject.

## Consumers

None.

## Prove

```bash
pwsh -NoProfile -File scripts/common/check-gate.ps1
```

Exit 0.

---

## Closing

Open.


## DOC-12. The muse-code guide starts an agent named `muse` and then prompts `work`

**Source** found while classifying the old record's finding 61
**Category** docs, **Priority** P3, **Effort** S, **Status** open

---

## Problem

A reader following the guide prompts an agent that does not exist.

## Premise

Read at `9784821`: `examples/muse-code/README.md:278` and :290-291.

## Approach

One name throughout.

## Consumers

None.

## Prove

```bash
sh scripts/common/check.sh docs
```

Exit 0.

---

## Closing

Open.


## DOC-13. The router and the rules count the release and CI wrong

**Source** found while measuring the old record's baseline, 2026-10-21
**Category** docs, **Priority** P3, **Effort** S, **Status** done

---

## Problem

`RULES.md` said CI makes six runs and that `main` requires three of six; it makes
eight. It and `docs/AGENTS.md` leave `text-tool`'s eight assets out of what a release
publishes.

## Premise

Measured on 2026-10-21: `gh run view` lists 8 jobs, and `gh release view` 22 assets.

## Approach

Both pages say what the API answers.

## Consumers

None.

## Prove

```bash
gh release view wsl-toolkit-v7.0.0 --json assets
```

22 assets, as the pages say.

---

## Closing

**Closed 2026-10-21T09:46:45Z.** `RULES.md` says CI makes eight runs and `main` requires three,
and it and `docs/AGENTS.md` count `text-tool`'s assets, 22 in all.

```text
gh run view on 9784821 --json jobs     8 jobs
gh release view wsl-toolkit-v7.0.0     22 assets: 2 wsl-toolkit, 4 text-tool, 4 herdr,
                                       SHA256SUMS, and a cosign bundle for each
```

## DOC-14. The skills tell an agent things the tool does not do

**Source** the review of the three skills, 2026-10-21, and the operator, 2026-10-21: "have the skill files been updated/cleaned"
**Category** docs, **Priority** P2, **Effort** M, **Status** open

---

## Problem

The review found claims the tool contradicts, passages that send a reader with only
the file into a wrong or destructive step, history in live text, and 7.0.0 behaviour
not mentioned. The worst: `base ensure` offered as the answer to most problems, though
it can restart and rebuild a base; `--instance base` never explained; an agents
snippet whose `adapters` key is refused where it is shown; a prompt, read and wait
order that races; and an exit-code table for `text-tool` that does not match it. The
installed copies predate 7.0.0.

## Premise

Measured on 2026-10-21: every command, flag and message checked against the installed 7.0.0 and the code.

## Approach

Apply each fix the review lists, add what 7.1.0 adds once it is merged, and
install the copies the operator's agents read, by the skills manager's own update.

## Consumers

None in this tree. An agent the operator runs reads the installed copies.

## Prove

```bash
sh scripts/common/check.sh skills
```

Exit 0, and the installed copies match the tree.

---

## Closing

Open.
