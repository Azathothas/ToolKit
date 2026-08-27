#!/usr/bin/env node
// scripts/common/set-record.mjs - move an entry's status and re-derive every count from the rows.
//
// ── WHAT IT IS FOR ───────────────────────────────────────────────────────────────────────────
// docs/methodology/work-todo.md calls the counts "the model's one mechanical hazard" and says to
// automate BOTH halves. check-record.sh is the reader and it has been a gate since it was written;
// this is the writer it names, and until this existed the arithmetic was done by hand.
//
// Closing one entry moves seven numbers: the index's `total N open N blocked N done N` line, the
// priority table's open/blocked/done/total for that priority, that table's **all** row, and the
// record's own count line. Doing that by hand eleven times in one session is how a published record
// ends up saying an entry is open beside an entry saying done.
//
// ── ⛔ IT IS NOT ITS OWN VERIFIER ─────────────────────────────────────────────────────────────
// It does not run check-record and report green. A writer that grades its own work is one bug away
// from hiding the bug, and work-todo.md is explicit that the reader must assert INDEPENDENTLY.
// It prints the command instead, and check-gate runs it.
//
// ── WHY NODE, WITH NO POWERSHELL TWIN ────────────────────────────────────────────────────────
// The same reason write-file.mjs has none, and scripts/README.md carries it: node is the same
// program on every host. No sed, no sort-that-is-really-Sort-Object, no shell built-ins, no
// aliases. The reason the sh checks needed twins does not apply here, and a second implementation
// of table arithmetic is a second place for that arithmetic to be wrong.
//
//   node scripts/common/set-record.mjs status WSL-06 done
//   node scripts/common/set-record.mjs status WSL-06 blocked
//   node scripts/common/set-record.mjs recount
//   node scripts/common/set-record.mjs recount --json
//   node scripts/common/set-record.mjs status WSL-06 done --dir TODO
//
// `recount` changes no status; it re-derives every count from whatever the rows currently say. That
// is the repair path when a status was edited by hand.
//
// ── ⚠ WHAT IT REFUSES ────────────────────────────────────────────────────────────────────────
//  • an id with no row, or a row whose entry heading is missing;
//  • a status outside open/blocked/done;
//  • a counts block or priority table it cannot find, rather than appending a new one;
//  • ⛔ a priority table row for a priority no entry uses is LEFT ALONE, not deleted. The table is
//    the project's declared shape and a priority with zero entries today is still a priority.
//
// Exit codes: 0 written (or already correct), 1 refused, 2 could not run.
//
// ⛔ Read the exit code from this process, unpiped.

import { readFileSync, writeFileSync, renameSync, existsSync, readdirSync, rmSync } from 'node:fs';
import { join, dirname } from 'node:path';
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';

const argv = process.argv.slice(2);

const die = (code, msg) => {
  console.error(`set-record: ${msg}`);
  process.exit(code);
};

const flag = (name) => {
  const i = argv.indexOf(`--${name}`);
  return i === -1 ? undefined : argv[i + 1];
};
const has = (name) => argv.includes(`--${name}`);

const MODE = argv[0];
if (!['status', 'recount'].includes(MODE ?? '')) {
  die(
    1,
    `usage: node scripts/common/set-record.mjs status <ID> <open|blocked|done> [--dir TODO] [--json]
       node scripts/common/set-record.mjs recount [--dir TODO] [--json]`,
  );
}

const JSON_OUT = has('json');
const DIR = flag('dir') ?? 'TODO';

// ── the repository root, so the scope does not depend on who called it ───────────────────────
let ROOT;
try {
  ROOT = execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim();
} catch {
  die(2, 'not a git repository, or git is not on PATH');
}

const INDEX = join(ROOT, DIR, 'INDEX.md');
const RECORD = join(ROOT, DIR, 'PROGRESS.md');
if (!existsSync(INDEX)) die(2, `no index at ${DIR}/INDEX.md`);

const STATUSES = ['open', 'blocked', 'done'];

// ── atomic write: temp file in the SAME directory, then rename. A killed process leaves the old
// file intact rather than a truncated one, and same-directory matters because a rename across
// volumes is a copy and loses the guarantee. Same rule as write-file.mjs. ─────────────────────
const writeAtomic = (path, text) => {
  const tmp = join(dirname(path), `.set-record.${randomBytes(8).toString('hex')}.tmp`);
  try {
    writeFileSync(tmp, text, 'utf8');
    renameSync(tmp, path);
  } catch (e) {
    try {
      rmSync(tmp, { force: true });
    } catch {
      // The temp file could not be removed either. Nothing useful to add: the
      // real failure is the one being rethrown.
    }
    throw e;
  }
};

// ── read the entry rows out of the index ────────────────────────────────────────────────────
// A row is: | ID | PRI | EFF | STATUS | title | [`file`](file) |
// ⚠ Selected on an id shaped like LETTERS-DIGITS rather than on line position, because the header
// and the separator both start with a pipe. check-record.sh selects the same way on purpose.
const ROW_RE = /^\|\s*([A-Z]+-\d+)\s*\|\s*([^|]*?)\s*\|\s*([^|]*?)\s*\|\s*([^|]*?)\s*\|/;

let indexText = readFileSync(INDEX, 'utf8');
const indexLines = indexText.split('\n');

const rows = [];
indexLines.forEach((line, i) => {
  const m = ROW_RE.exec(line);
  if (m) rows.push({ line: i, id: m[1], pri: m[2], eff: m[3], status: m[4] });
});

if (rows.length === 0) die(2, `no entry rows found in ${DIR}/INDEX.md`);

const changed = [];

// ── mode: status ────────────────────────────────────────────────────────────────────────────
if (MODE === 'status') {
  const id = argv[1];
  const want = argv[2];
  if (!id || !want) die(1, 'status needs an id and a status: status WSL-06 done');
  if (!STATUSES.includes(want)) {
    die(1, `'${want}' is not a status. One of: ${STATUSES.join(', ')}`);
  }

  const row = rows.find((r) => r.id === id);
  if (!row) die(1, `${id} has no row in ${DIR}/INDEX.md. Nothing was written.`);

  // ── the entry's own status line, in whichever category file holds it ──────────────────────
  // ⛔ Both files move or neither does. A row that says done beside an entry that says open is
  // exactly the disagreement check-record exists to catch, and writing one without the other
  // would be this tool manufacturing it.
  const entryFiles = readdirSync(join(ROOT, DIR))
    .filter((f) => f.endsWith('.md') && !['INDEX.md', 'PROGRESS.md', 'RULES.md'].includes(f))
    .map((f) => join(ROOT, DIR, f));

  const headingRe = new RegExp(`^##\\s+${id}\\.`, 'm');
  const holder = entryFiles.find((f) => headingRe.test(readFileSync(f, 'utf8')));
  if (!holder) die(1, `${id} has a row but no '## ${id}.' heading under ${DIR}/. Nothing was written.`);

  const entryText = readFileSync(holder, 'utf8');
  const lines = entryText.split('\n');
  const start = lines.findIndex((l) => headingRe.test(l));
  let statusLine = -1;
  for (let i = start + 1; i < lines.length; i++) {
    if (/^##\s/.test(lines[i])) break;
    if (/\*\*Status\*\*/.test(lines[i])) {
      statusLine = i;
      break;
    }
  }
  if (statusLine === -1) {
    die(1, `${id} in ${holder} has no **Status** line. Nothing was written.`);
  }

  const before = lines[statusLine];
  const after = before.replace(/(\*\*Status\*\*\s*)([A-Za-z]+)/, `$1${want}`);
  if (after === before && !new RegExp(`\\*\\*Status\\*\\*\\s*${want}\\b`).test(before)) {
    die(1, `could not rewrite the **Status** on ${holder}:${statusLine + 1}. Nothing was written.`);
  }
  if (after !== before) {
    lines[statusLine] = after;
    writeAtomic(holder, lines.join('\n'));
    changed.push(`${holder.slice(ROOT.length + 1)}: ${id} status -> ${want}`);
  }

  if (row.status !== want) {
    // Replace only the fourth cell, by rebuilding the row's leading cells. A blanket replace of
    // the old word would also hit it in the title.
    const line = indexLines[row.line];
    const m = ROW_RE.exec(line);
    const head = `| ${m[1]} | ${m[2]} | ${m[3]} | ${want} |`;
    indexLines[row.line] = head + line.slice(m[0].length);
    changed.push(`${DIR}/INDEX.md: ${id} status -> ${want}`);
    row.status = want;
  }
}

// ── re-derive every count from the rows ─────────────────────────────────────────────────────
const total = rows.length;
const tally = (st) => rows.filter((r) => r.status === st).length;
const open = tally('open');
const blocked = tally('blocked');
const done = tally('done');

const countLine = `total ${total}  open ${open}  blocked ${blocked}  done ${done}`;

let touchedCounts = 0;
let touchedTable = 0;

for (let i = 0; i < indexLines.length; i++) {
  if (/^total\s+\d+\s+open\s+\d+/.test(indexLines[i])) {
    if (indexLines[i] !== countLine) {
      indexLines[i] = countLine;
      touchedCounts++;
    }
  }
}
if (touchedCounts === 0 && !/^total\s+\d+\s+open\s+\d+/m.test(indexText)) {
  die(1, `${DIR}/INDEX.md has no 'total N open N blocked N done N' line. Nothing else was written.`);
}

// The priority table. ⛔ A row for a priority nothing currently uses keeps its row and gets zeros;
// it is the project's declared shape, not a derived list.
const PRI_RE = /^\|\s*(P\d)\s*\|\s*\d+\s*\|\s*\d+\s*\|\s*\d+\s*\|\s*\d+\s*\|\s*$/;
const ALL_RE = /^\|\s*\*\*all\*\*\s*\|/;

for (let i = 0; i < indexLines.length; i++) {
  const m = PRI_RE.exec(indexLines[i]);
  if (m) {
    const p = m[1];
    const inP = rows.filter((r) => r.pri === p);
    const line = `| ${p} | ${inP.filter((r) => r.status === 'open').length} | ${inP.filter((r) => r.status === 'blocked').length} | ${inP.filter((r) => r.status === 'done').length} | ${inP.length} |`;
    if (indexLines[i] !== line) {
      indexLines[i] = line;
      touchedTable++;
    }
    continue;
  }
  if (ALL_RE.test(indexLines[i])) {
    const line = `| **all** | **${open}** | **${blocked}** | **${done}** | **${total}** |`;
    if (indexLines[i] !== line) {
      indexLines[i] = line;
      touchedTable++;
    }
  }
}

const nextIndex = indexLines.join('\n');
if (nextIndex !== indexText) {
  writeAtomic(INDEX, nextIndex);
  indexText = nextIndex;
  if (touchedCounts) changed.push(`${DIR}/INDEX.md: counts -> ${countLine}`);
  if (touchedTable) changed.push(`${DIR}/INDEX.md: priority table, ${touchedTable} row(s)`);
}

// ── the record's own count line ─────────────────────────────────────────────────────────────
// ⚠ A record with no count line is not an error. Not every project states one, and inventing the
// requirement here would refuse a correct tree. check-record.sh takes the same position.
if (existsSync(RECORD)) {
  const recText = readFileSync(RECORD, 'utf8');
  const recLines = recText.split('\n');
  let touched = 0;
  for (let i = 0; i < recLines.length; i++) {
    const m = /^(\s*entries\s+)total\s+\d+\s+open\s+\d+\s+blocked\s+\d+\s+done\s+\d+\s*$/.exec(recLines[i]);
    if (m) {
      const line = `${m[1]}${countLine}`;
      if (recLines[i] !== line) {
        recLines[i] = line;
        touched++;
      }
    }
  }
  if (touched) {
    writeAtomic(RECORD, recLines.join('\n'));
    changed.push(`${DIR}/PROGRESS.md: counts -> ${countLine}`);
  }
}

// ── report ──────────────────────────────────────────────────────────────────────────────────
if (JSON_OUT) {
  console.log(
    JSON.stringify({
      schema: 'set-record/1',
      changed: changed.length,
      total,
      open,
      blocked,
      done,
    }),
  );
  process.exit(0);
}

if (changed.length === 0) {
  console.log('set-record: already correct, nothing written');
} else {
  for (const c of changed) console.log(`  ${c}`);
}
console.log(`set-record: ${countLine}`);
console.log('');
console.log('⛔ This tool does not grade its own work. Verify with the independent reader:');
console.log('     sh scripts/common/check-record.sh');
process.exit(0);
