# docs

The map. Which document answers which question, so a session reads what its
task needs rather than everything.

⭐ **Read the row, then read the document.** Reading the row is not reading the
rule: these summaries exist to route, not to substitute.

---

## methodology: how work is planned, gated and handed over

| file | answers |
| --- | --- |
| [`initialize.md`](methodology/initialize.md) | how to start a project that does not exist yet. The mindset, the phases, the approval gates. |
| ⭐ [`gate.md`](methodology/gate.md) | what a unit of work passes before it is done. Three parts, none skippable. |
| ⭐ [`reviews.md`](methodology/reviews.md) | the three review lenses, and why one sweep written up three times is not three passes. |
| ⭐ [`sessions.md`](methodology/sessions.md) | what a session owes at its start and its end, how to resume one, how to freeze cleanly. |
| [`authoring.md`](methodology/authoring.md) | how a rough idea becomes an approved unit of work. Authoring and implementing are different sessions. |
| [`work-todo.md`](methodology/work-todo.md) | the todo model: an index, a record, entries that close in place. |
| [`references.md`](methodology/references.md) | how to study somebody else's project, including the step that always gets skipped. |

## conventions: how things are written here

| file | answers |
| --- | --- |
| [`prose.md`](conventions/prose.md) | how documents are written. The three markers, and why amendments are made in place. |
| [`docs.md`](conventions/docs.md) | the document set, one fact one home, and the changelog rules. |
| [`git.md`](conventions/git.md) | commit identity, what may reach a remote, what is never committed. |
| [`code.md`](conventions/code.md) | one read path one write path, build to last, and the testing tiers. |
| ⭐ [`forbidden-patterns.md`](conventions/forbidden-patterns.md) | the table to grep yourself against before declaring a gate green. |
| ⭐ [`shell.md`](conventions/shell.md) | quoting, heredocs, exit codes, streams, line endings, and the platform traps. |

⚠ The entry points that live outside `docs/`: [`AGENTS.md`](../AGENTS.md), the
router, and [`TODO/PROGRESS.md`](../TODO/PROGRESS.md), the record every session
reads first. [`reference-sweeps/findings.md`](reference-sweeps/findings.md) and
[`reference-sweeps/usable.md`](reference-sweeps/usable.md) are what external reference
sweeps owe, per [`methodology/references.md`](methodology/references.md).

## security

| file | answers |
| --- | --- |
| [`secrets.md`](security/secrets.md) | what never enters the tree, and what to do when something did. |
| [`remote-ops.md`](security/remote-ops.md) | the three tiers governing action on anything outside this machine. |

## visibility: one of these is kept, the other deleted

| file | answers |
| --- | --- |
| [`public/README.md`](public/README.md) | what changes because the repository is, or will be, public. |

## templates: the skeletons a new project receives

Everything in [`templates/`](templates/) carries double-brace placeholder
markers and guidance comments. ⛔ **Both are removed when a file is filled in**,
and `scripts/common/check-placeholders.sh` is what proves it.

⚠ This paragraph describes the marker rather than showing one, on purpose. A
document that demonstrates the thing a checker looks for makes the checker fire
on correct writing, which is the same class as a page about escape sequences
containing the byte it warns about.

| file | becomes |
| --- | --- |
| ⭐ [`AGENTS.md`](templates/AGENTS.md) | the project's router, carrying the task routing table. Under 300 lines. |
| [`PROGRESS.md`](templates/PROGRESS.md) | the record. The one file every session reads first. |
| [`INDEX.md`](templates/INDEX.md) | the entry list, in todo mode. |
| [`RULES.md`](templates/RULES.md) | how this repository is worked on, with what each rule cost. |
| [`HUMAN.md`](templates/HUMAN.md) | the operator's side: setup, validation, runbooks, prompts. |
| [`README.md`](templates/README.md) | the front door, for a competent stranger. |
| [`SECURITY.md`](templates/SECURITY.md) | the threat model. Writing it is the audit. |
| [`CHANGELOG.md`](templates/CHANGELOG.md) | what shipped, when, and where the evidence is. |
| [`todo-entry.md`](templates/todo-entry.md) | one entry, in todo mode. |

---

## The rules these documents hold themselves to

- ⛔ **One fact, one home.** A value in two documents with no check between them
  drifts, and the copy a reader trusts is the wrong one.
- ⛔ **Amend in place.** When a rule changes, the rule is rewritten and the
  superseded wording moves to a history file. Stacking a dated box under retired
  text has a documented failure mode: an agent reads the first paragraph of the
  box, stops, and acts on the retired rule.
- ⛔ **Every claim verified before it is written.** Writing the documentation is
  the audit, and the most confident sentence in a file is regularly the only
  false one.
- ⛔ **Never a fabricated number.** A dash where the value is unknown.
- ⚠ **A page nothing links to is a finding.** Unlinked means unread, which means
  uncorrected, which is the state every stale document passes through.
