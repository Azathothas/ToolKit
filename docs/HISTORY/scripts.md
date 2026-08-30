# HISTORY: scripts/README.md

⛔ **Superseded. Nothing here is needed to use or to add a script.**
[`../../scripts/README.md`](../../scripts/README.md) is the live page: the
directory layout, the five-point check contract, what each script does, and the
constraints a reader has to keep.

This holds the counts two checks reported the day they were first armed. Moved
here on 2026-08-30, under `DOC-06`, because a reader deciding whether to keep a
check, or writing a new one, cannot act on a number from a tree that has since
been fixed. ⭐ What DID stay on the live page is the measurement that explains a
constraint, such as `Sort-Object -u` dropping two of four distinct values: a
reader who does not know that will undo the twins rule.

---

## `check-markers.sh`, armed 2026-08-29

⚠ **164 characters across 28 files**, every one of them in a script's comment
banner, with `check-docs.sh` reporting the tree clean throughout.

⭐ That last clause is the reason the check exists beside `check-docs.sh` rather
than inside it: `check-docs.sh` reads markdown, and every finding was in a
`.ps1` or a `.sh`.

## `check-one-home.sh`, armed 2026-08-29

⚠ **17 sentences with two homes**, seven of them involving a skeleton this
repository had copied from a template and never filled in.

⭐ [`../conventions/prose.md`](../conventions/prose.md) had always said one fact
lives in one document, and nothing checked it. That gap is the shape a rule takes
on its way to becoming a preference, and it is why
[`../conventions/docs.md`](../conventions/docs.md) prefers a form a check can
assert.
