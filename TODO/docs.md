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
