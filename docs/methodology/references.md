# references.md

How to study somebody else's project: cloning it, reading it, reading its
tracker, and writing what a sweep owes.

Binding on any task whose verb is **clone, mine, survey or investigate.**

---

## The order

```
1 clone shallow -> 2 CAPTURE THE COMMIT -> 3 read the code -> 4 READ THE TRACKER
                                                            -> 5 trim by DELETING
                                                            -> 6 write the two files
```

---

## 1 and 2. Clone, and capture the commit first

```bash
git clone --depth 1 -q "https://github.com/OWNER/REPO.git" REPO
```

```bash
git -C REPO rev-parse HEAD
```

⛔ **Capture the commit before stripping anything.** Once the git directory is
gone the commit is unrecoverable and every line citation becomes unverifiable.

⛔ **Trim by deleting, never by moving.** A trim that rewrites paths invalidates
every citation already written, including the ones in the write-up you are
still writing.

⚠ **A clone is normally not tracked**, so ⭐ **the commit recorded in your
write-up is the only provenance that survives.** A reader on another machine
has nothing else.

---

## 3. Read the code

Passes, not a pass. ⭐ **At least three, and each asks a different question.**
Three readings with one question is one pass written up three times.

| pass | the question |
| --- | --- |
| 1 | what is this, what problem does it solve, what shape is it |
| 2 | the actual construction, in its source, at file and line |
| 3 | how it handles the thing **your** work finds hard |
| 4 | what transfers, what must not, and what it changes about your plan |

⭐ **Where a reference genuinely does not support that many, say which and
why.** A password vault has nothing to say about ranged reads, and claiming a
fourth pass over it is worse than admitting three.

---

## 4. Read the tracker. This is the step that gets skipped

⛔ **Fetch the issues and the pull requests, both states.**

A repository shows you what somebody built. ⭐ **Its tracker shows you what
broke, what was measured, what was refused and why, and what the maintainer
says the project is actually for.**

A sweep of eleven repositories once opened no tracker at all. The issue pass
that followed produced a measured production figure, a threat model nobody had
stated, and two corrections to claims already written down. **None of it was
visible in the code.**

```bash
gh api "repos/OWNER/REPO/issues?state=all&per_page=100" --jq '.[] | "\(.number)\t[\(.state)]\t\(if .pull_request then "PR" else "IS" end)\t\(.title)"'
```

⛔ **The issues endpoint returns pull requests too**, and the open-issue count
counts both. Discriminate on the pull-request field, or you will report a
dependency bump as an issue.

⭐ **Closed is where the decisions are.** Open alone is a defect list.

What to search for:

| ask | why it pays |
| --- | --- |
| the thing your work is about | somebody has usually already tried it. "Nice idea, never built" is cost evidence you cannot get from code. |
| memory, out-of-memory, large inputs, concurrency | the numbers are real and measured on production hardware, which no benchmark of yours will be |
| the failure mode you are designing against | if it is absent, that is information too |
| "is this superseded by" | whether the reference is live or archaeology, in the maintainer's own words |
| the maintainer's answers, not just the reports | "this cannot be done because" is a costing you would otherwise derive |
| ⭐ the confessions in pull request bodies | "the existing tests never caught this because the harness defaulted X off" is the richest single line a tracker produces |

⚠ **Read the comments, not only the body.** An issue still open with a
maintainer comment saying "check the latest version" means fixed in code and
unconfirmed by the reporter, which is neither fixed nor open. Report the state
you actually found.

⛔ **Reads only.** No write verb, no private repository, never an issue or a
comment created on the operator's behalf.
[`../security/remote-ops.md`](../security/remote-ops.md).

⛔ **If you cannot fetch something, say so in the write-up.** A silently skipped
reference is the failure this whole procedure exists to prevent.

---

## 5 and 6. What a sweep owes

Two files, with different jobs:

| file | job | ⛔ the failure mode |
| --- | --- | --- |
| the **findings** file | the findings, the verdicts, the **reasoning**, and a provenance table of name, commit and depth reached | becoming a diary. A verdict without a reason is an opinion. |
| the **usable** file | ⭐ the lessons and the **actual code lines**. This is the one a later session reads. | being written to be admired now instead of used later. If the next session cannot act on it without re-cloning, it failed. |

⛔ **Both say what they did not do.** Depth reached per reference, references
not fetched, passes not taken.

⭐ **The derived write-up is tracked even when the clone is not.** A
required-reading file that exists on one machine is one deletion away from
leaving every claim built on it unsourced, and the trim is the operation that
does the deleting.

---

## Verdicts

Every reference gets exactly one:

| verdict | meaning |
| --- | --- |
| **adopt** | a specific mechanism, cited at file and line, going into a named task |
| **confirms** | we already do this. Independent evidence, not new work. |
| ⭐ **anti-pattern exhibit** | kept **on purpose**. A shipped defect is worth more than an absence: record the defect and whether its own tests or audit missed it. |
| **filed elsewhere** | not this unit's. Write it into the one that owns it. Never dropped, never chased here. |
| **refused** | with the reason, so no future session re-derives it |

---

## The traps, each one paid for

1. ⛔ **Skipping the tracker.** Above.
2. ⛔ **Believing a document over its code.** Design records and READMEs go
   stale. One project's design record documented a derivation its own code had
   already replaced with a stronger one: the code was right and the record was
   three versions behind. Read the document, then check the code, then cite the
   code.
3. ⛔ **Trusting a reference's own citations.** A comment citing "issue 38" for
   a change that issue 38 is not about. Resolve a cited number before repeating
   it.
4. ⚠ **Grep locates; it does not confirm.** A search for a crypto term "found
   crypto" in one project; the hits were CSS class names. Open the file.
5. ⚠ **Counting lines with the wrong tool.** Some line counters skip blank
   lines, producing an undercount that reads like a precise figure.
6. ⛔ **Do not delegate a reference's reading to a sub-agent.** Operator ruling.
   A delegated read comes back confident and thin, and you cannot tell which
   parts were actually opened.
7. ⚠ **Re-mine a reference even if it has been swept before.** Projects move. A
   previous verdict was taken against a different commit, and you now have the
   commit to prove which.
8. ⛔ **A citation is evidence of what somebody else did, not evidence that your
   project does it.** Never let one become the other in a document.

---

## Adopt ideas, not architectures

⭐ The recurring conclusion across many sweeps, reached independently each time.

A reference's architecture is shaped by its own constraints, which are not
yours. What transfers is a **mechanism**, cited at file and line, with the
reason it applies here. What does not transfer is the shape of somebody else's
solution to a problem you do not have.

⚠ **Direction is easy to get backwards.** A client tuning itself against a
server is not a model for the server. Read what the reference *is* before
deciding what it teaches.
