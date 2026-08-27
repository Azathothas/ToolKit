# prose.md

How documents are written here. The mechanical half is checked by a linter; the
rest is a reading.

---

## The rule

Short sentences. No em dashes. No marketing adjectives. No emoji beyond the
five defined below, and only three of those belong in prose. Present tense.
Every claim backed by a command a reader can run or a path a reader can open.

Write for an agent with no memory of the session that wrote the file, and for a
person who is looking for one fact.

---

## The three markers, and nothing else

⛔ ⭐ ⚠ and no others. Each means one thing:

| marker | meaning |
| --- | --- |
| ⛔ | a rule that has already been broken, or one whose violation is unrecoverable. A hard stop. |
| ⭐ | reach for this first. The highest-value item on the page. |
| ⚠ | a trap. It works until it does not, and the failure is quiet. |

⛔ **They do not stack.** There is no `⛔⛔` and no `⛔⛔⛔`. Escalating a marker
is how a vocabulary stops meaning anything: once a page has three levels of
stop, a reader has to weigh them, and weighing is what a marker exists to
prevent. One marker or none.

⭐ **Use them sparingly enough that they are still visible.** A page where every
paragraph carries one has no markers at all. If a page needs more than a
handful, the page is a rulebook pretending to be a summary and it should be
split.

---

## Status glyphs are a second tier, and a different job

⭐ **A semantic marker and a status glyph are not the same thing.** ⛔ means a
rule whose violation is unrecoverable. A check reporting that one file of forty
failed needs a pass or fail glyph, which denotes a state rather than a rule.

The two tiers, and there is no third:

| tier | the set | where it belongs | what it means |
| --- | --- | --- | --- |
| prose markers | ⛔ ⭐ ⚠ | documents | the table above. Sparing, and they do not stack. |
| status glyphs | ✅ ❌ | machine output, result tables, checklists | passed, or failed. Nothing else. |

⛔ **A status glyph never carries a rule, and a marker never reports a result.**
With no glyph available an author reaches for ⛔ to mean "this one failed", and
that is exactly the dilution the three-marker rule exists to prevent. Widening
the set by two characters is what keeps the other three meaning what they say.

⚠ **The list is two characters, not a principle, and that is deliberate.** The
tempting version of this rule is "allow non-anthropomorphic symbols, forbid
anthropomorphic ones", on the reasoning that faces and hands carry tone while a
symbol denotes a state. The reasoning is right and the rule is unenforceable: no
check can decide what is anthropomorphic, so the boundary would move every time
somebody argued for one more glyph, and a vocabulary that grows stops meaning
anything. An explicit five-character allowlist is something
[`check-docs.sh`](../../scripts/common/check-docs.sh) can hold, and it holds it.

⚠ **The linter owns the allowlist. It does not own the tiers.** Nothing
mechanical can tell a result table from a paragraph, so a glyph used as a marker
passes the check and fails the review. That split is the same one already true
of sparingness, and it is why both are written here rather than only in the
check.

---

## Amend in place. Do not stack banners.

⛔ **When a rule changes, rewrite the rule.** Do not append a dated box under
the old text saying the text above is retired.

This is the correction with the most evidence behind it. A document written by
accretion, where the paragraph says one thing and a box below it says the
opposite, has a documented failure mode: an agent reads the first paragraph of
the box, stops, and acts on the retired rule. It happened, it broke a rule
about publishing, and the incident report is the reason this section exists.

What to do instead:

1. **Rewrite the rule to what it is now.** The current text is the only text.
2. **Move the superseded wording to a history file**, with the date and why it
   changed. A separate file, not a box on the live page.
3. **Link to it once**, from the rule, in a sentence.

The story of a change belongs in the changelog or the history file. The
document says what is true now. A reader reaching for a rule needs one answer,
and a page that offers two makes them guess which is live.

⚠ This is not licence to delete. A superseded rule is moved, never dropped, so
a future session that wonders why the rule is what it is can find out instead of
re-deriving it wrongly.

---

## Say what is not true

Reserve an explicit place for the truths that are tempting to hide. This is
slower than it looks. This feature has a known gap. This estimate excludes
something that cannot be measured.

⛔ **Never a fabricated number.** When the real value is unknown, write a dash.
A wrong number on a report is worse than no number, because a blank gets
checked and a number gets used.

⚠ **A measurement carries its conditions or it is not a measurement.** A rate
with no date, no machine, no sample count and no input size cannot be compared
to anything, which makes it worse than an absence: it invites a comparison that
means nothing.

---

## Every claim is verified before it is written

Writing the documentation is the audit. Being forced to state precisely what
something does, and then checking whether that is true, is where a startling
share of real defects are found. Expect the documentation pass to generate
findings, and treat that as the feature rather than as a delay.

⚠ Do not copy a number out of another document. Derive it, or name where it
came from. A value in two places with no check between them drifts, and the
copy a reader trusts is the wrong one.

---

## One fact, one home

Every fact lives in exactly one document. If it must appear in a second place,
derive it there or have a check assert that the two agree.

When two documents conflict, the technical reference wins and the other one is
the defect. Fix it in the same change and say so.

---

## What a document is not

**A document says what the thing does. It does not say what the project did.**

A fixed defect belongs in a reference page only when a reader needs it to use
the thing correctly. "The allocator takes a write lock now" is history and
belongs in the work record. "Two lints exist because another client will refuse
the file" is a constraint and stays.

⚠ An unlinked page is not read, so it is not corrected, and that is the state
every stale document passes through on the way to being wrong. A page nothing
links to is a finding.

---

## The mechanical half, which a linter checks

A documentation linter catches the things that rot silently and that no other
check sees:

1. **Every fenced shell block parses.** A block that does not parse is a block
   nobody can copy and paste.
2. **No angle-bracket placeholders inside a shell block.** A human reads
   `<deployment-id>` as "fill this in" and bash reads it as a redirect, so the
   reader gets a cryptic syntax error instead of an obvious instruction. Use an
   upper-case name or a quoted variable.
3. **No literal control bytes.** Documentation about escape sequences has a
   proven habit of containing the character it is warning about.
4. **Every relative link resolves**, and every cited path exists.
5. **No em dash, no emoji outside the five, none of the banned vocabulary.**
   The five are the three prose markers plus the two status glyphs.
6. **No page under the docs directory that nothing links to.**

⛔ **What a linter cannot check is whether a claim is true.** That is a reading,
and it belongs to the review pass. A guard that tried to verify prose would
either pass vacuously or refuse legitimate writing, and both are worse than an
honest scope.

---

## Banned vocabulary

Words that assert quality instead of demonstrating it. They survive review
because they feel like description:

> seamless, blazing, effortless, robust, powerful, cutting-edge,
> state-of-the-art, world-class, elegant, simply, just, obviously, of course,
> revolutionary, game-changing, rock-solid, bulletproof, lightning-fast

⚠ "Simply" and "just" are the two that do real damage. They tell a reader who
is stuck that the thing they cannot do is easy.

Replace the adjective with the measurement, or delete it. "Fast" becomes the
number and its conditions. "Robust" becomes what it survives.

---

## Defensive framing is not neutral

⛔ **Describe what code does in plain technical terms.** Do not write up-front
disclaimers arguing that something is legitimate, and do not tell a future
reader not to re-open a question.

Both backfire, in opposite directions. A defensive paragraph primes a skeptical
reader to look for the thing it denies. And a grepping agent trips on the
reassurance words themselves and spends its budget reading the matched line.

State the mechanism and its constraints. Name prior art briefly if it helps.
Stop there. The same applies to identifiers: prefer a neutral accurate name to
an evocative one.
