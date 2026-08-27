#!/usr/bin/env node
// scripts/common/write-file.mjs - write, append to, or patch a file without the shell touching the payload.
//
// ── WHAT IT IS FOR ───────────────────────────────────────────────────────────────────────────
// Heredocs, PowerShell here-strings and `echo`/`printf` all put the FILE CONTENT on the command
// line, where a backtick, a `$`, an unbalanced quote, an emoji or an indented terminator changes it
// or eats the rest of the session. This project runs two shells with different rules for all of
// that. Every mode below takes the payload through a channel the shell cannot reach into.
//
//   write    <path>   replace the whole file (creates parents)
//   append   <path>   add to the end, creating the file if absent
//   replace  <path>   substitute one exact string for another, with a required count
//
// ── HOW THE PAYLOAD GETS IN - pick by what your shell makes easy ─────────────────────────────
//   --b64 <BASE64>    ⭐ the bulletproof one. Base64 is [A-Za-z0-9+/=] and needs no quoting in ANY
//                     shell. Use it whenever the content has quotes, backticks, `$`, `%` or emoji.
//   --from <file>     copy the bytes of another file (no re-encoding)
//   (nothing)         read stdin - natural behind a pipe, and the only mode that streams
//
// `replace` takes --find-b64 / --replace-b64 (or --find-from / --replace-from) and requires
// --expect <n>, the number of occurrences you believe are there. A substitution that matches a
// different number of times is REFUSED and the file is untouched: a silent no-op that reports
// success is the failure this mode exists to remove.
//
// ── GOTCHAS, EACH ONE LOAD-BEARING ───────────────────────────────────────────────────────────
//  • ⛔ IT REFUSES TO WRITE OUTSIDE THE REPO. The path is resolved (symlinks included) and must sit
//    under the git toplevel. `../../etc/hosts` exits 2.
//  • ⛔ IT REFUSES A PATH THE SHELL WOULD FIGHT YOU OVER - a shell metacharacter, a space, a
//    root-level redirect spill, a credential-shaped name. You cannot use this tool to create the
//    junk the other guard exists to catch.
//  • ⭐ THE WRITE IS ATOMIC: a temp file in the SAME directory, then rename. A killed process leaves
//    the old file intact, never a truncated one. Same directory matters - rename across volumes is
//    a copy, and loses the guarantee.
//  • ⚠ IT WRITES BYTES EXACTLY. No newline is added, no CRLF conversion is done. Git applies
//    .gitattributes on commit; this tool does not second-guess it.
//  • ⚠ `--b64` REJECTS input that is not valid base64 rather than writing mojibake - the common
//    cause is a shell having wrapped a long value across lines.
//  • ⚠ `append` on a file with no trailing newline joins the last line. Pass --nl to insert one.
//  • ⛔ POWERSHELL'S STDIN IS NOT BYTE-EXACT AND GIT BASH'S IS. Measured on one 59-byte fixture:
//    `Get-Content x -Raw | node …` wrote 61 bytes - PowerShell's native-command pipe APPENDS a
//    trailing CRLF (tail `… 3e 20 3c 0a` became `… 3c 0a 0d 0a`). The same file through `cat x |`
//    in Git Bash was byte-identical, as were `--b64` and `--from` from BOTH shells. The tool cannot
//    tell an intended trailing newline from an added one, so it does not guess.
//    ⭐ FROM POWERSHELL, USE `--b64` OR `--from`. Reserve stdin for pipes in Git Bash.
//
// ── EXAMPLES - both shells, same command ─────────────────────────────────────────────────────
//   node scripts/common/write-file.mjs write docs/note.md --b64 SGVsbG8gJ3dvcmxkJyBgJFBBVEhgCg==
//   node scripts/common/write-file.mjs append docs/note.md --nl --b64 dHJhaWxpbmcgbGluZQo=
//   node scripts/common/write-file.mjs replace src/a.ts --find-b64 Zm9v --replace-b64 YmFy --expect 3
//   Get-Content in.md -Raw | node scripts/common/write-file.mjs write docs/note.md      # PowerShell
//   cat in.md          | node scripts/common/write-file.mjs write docs/note.md          # Git Bash
//
// Exit codes: 0 ok · 1 refused (count mismatch, bad base64, missing input) · 2 path refused.

import { execFileSync } from 'node:child_process';
import {
  readFileSync,
  writeFileSync,
  renameSync,
  mkdirSync,
  existsSync,
  rmSync,
  realpathSync,
} from 'node:fs';
import { dirname, resolve, relative, sep, basename } from 'node:path';
import { randomBytes } from 'node:crypto';

const argv = process.argv.slice(2);
const MODE = argv[0];
const TARGET = argv[1];

const flag = (name) => {
  const i = argv.indexOf(`--${name}`);
  return i === -1 ? undefined : argv[i + 1];
};
const has = (name) => argv.includes(`--${name}`);

const die = (code, msg) => {
  console.error(msg);
  process.exit(code);
};

if (!['write', 'append', 'replace'].includes(MODE) || !TARGET || TARGET.startsWith('--')) {
  die(
    1,
    `usage: node scripts/common/write-file.mjs <write|append|replace> <path> [--b64 <B64> | --from <file>]
       replace also needs --find-b64/--find-from, --replace-b64/--replace-from and --expect <n>
       read the header of this file for the full contract`,
  );
}

// ── the repo boundary ────────────────────────────────────────────────────────────────────────
const ROOT = realpathSync(
  execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim(),
);
const abs = resolve(process.cwd(), TARGET);
// Resolve the nearest EXISTING ancestor: the target itself may not exist yet, but a symlinked
// parent must not be a way out of the tree.
let probe = abs;
while (!existsSync(probe) && dirname(probe) !== probe) probe = dirname(probe);
const realAbs = resolve(realpathSync(probe), relative(probe, abs));
// ⛔ CONTAINMENT IS A PREFIX TEST, NOT `relative(...).startsWith('..')`. On Windows
// `path.relative('D:\\repo', 'C:\\Windows\\x')` returns `C:\Windows\x` - no leading `..` at all -
// so the obvious spelling lets any other drive through. Windows paths are also case-insensitive.
const norm = (p) => (process.platform === 'win32' ? p.toLowerCase() : p);
if (norm(realAbs) !== norm(ROOT) && !norm(realAbs).startsWith(norm(ROOT) + sep)) {
  die(
    2,
    `REFUSED: ${TARGET}\n  resolves to ${realAbs}, outside the repository (${ROOT}).\n  FIX: write inside the tree, or use your editor for files that genuinely live elsewhere.`,
  );
}
const rel = relative(ROOT, realAbs);

// ── the path predicate, so this tool cannot create a name that needs quoting ever after ──
const SHELL_CHARS = new Set([...'()&|;<>$`\'"*?[]!%#\\: \t']);
const relPosix = rel.split(sep).join('/');
const base = basename(relPosix);
const badChars = [...relPosix].filter((c) => SHELL_CHARS.has(c) || c.codePointAt(0) < 0x20);
const SECRET_NAME =
  /(^\.(env|dev\.vars)(\..+)?$)|(^\.[^/]*(token|secret|credential)[^/]*$)|(\.(pem|key|p12|pfx|keystore|jks)$)|(^id_(rsa|dsa|ecdsa|ed25519)$)/i;
const SECRET_EXEMPT = /(\.example$)|(\.sample$)|(\.template$)|(\.pub$)/i;
if (badChars.length) {
  die(
    2,
    `REFUSED: ${relPosix}\n  path contains ${[...new Set(badChars)].map((c) => JSON.stringify(c)).join(' ')}, which a shell would need quoting for ever after.\n  FIX: name it with [A-Za-z0-9_.-] only.`,
  );
}
if (!relPosix.includes('/') && /^\d[^.]*$/.test(base)) {
  die(
    2,
    `REFUSED: ${relPosix}\n  a repo-root name of that shape is what a stray shell redirect leaves behind.\n  FIX: give it a real name and an extension, or write it under a subdirectory.`,
  );
}
if (SECRET_NAME.test(base) && !SECRET_EXEMPT.test(base)) {
  die(
    2,
    `REFUSED: ${relPosix}\n  that name says it holds a credential, and this tool will not create one in the tree.\n  FIX: keep secrets outside the repo, or gitignore the path FIRST and write it with your editor.`,
  );
}

// ── payload channels ─────────────────────────────────────────────────────────────────────────
const decodeB64 = (s, what) => {
  const cleaned = s.replace(/\s+/g, '');
  if (!/^[A-Za-z0-9+/]*={0,2}$/.test(cleaned) || cleaned.length % 4 !== 0) {
    die(
      1,
      `REFUSED: --${what} is not valid base64.\n  The usual cause is the shell splitting a long value across lines.\n  FIX: re-encode and pass it as one unbroken argument, or use --${what.replace('b64', 'from')} <file>.`,
    );
  }
  return Buffer.from(cleaned, 'base64');
};

const readStdin = () => {
  try {
    return readFileSync(0);
  } catch {
    return Buffer.alloc(0);
  }
};

const payloadFor = (b64Flag, fromFlag, label) => {
  const b64 = flag(b64Flag);
  const from = flag(fromFlag);
  if (b64 !== undefined) return decodeB64(b64, b64Flag);
  if (from !== undefined) {
    const p = resolve(process.cwd(), from);
    if (!existsSync(p)) die(1, `REFUSED: --${fromFlag} ${from} does not exist.`);
    return readFileSync(p);
  }
  if (label === 'body') {
    const s = readStdin();
    if (s.length === 0)
      die(
        1,
        `REFUSED: no payload.\n  Pass --${b64Flag} <BASE64>, --${fromFlag} <file>, or pipe the body on stdin.`,
      );
    return s;
  }
  die(1, `REFUSED: ${label} not supplied - pass --${b64Flag} or --${fromFlag}.`);
};

// ── the atomic write ─────────────────────────────────────────────────────────────────────────
const atomicWrite = (target, buf) => {
  mkdirSync(dirname(target), { recursive: true });
  // Same directory, so the rename is a metadata operation on one volume rather than a copy.
  const tmp = `${target}.tmp-${process.pid}-${randomBytes(4).toString('hex')}`;
  try {
    writeFileSync(tmp, buf);
    renameSync(tmp, target);
  } catch (err) {
    rmSync(tmp, { force: true });
    throw err;
  }
};

const existed = existsSync(abs);
const before = existed ? readFileSync(abs) : Buffer.alloc(0);
let next;

if (MODE === 'write') {
  next = payloadFor('b64', 'from', 'body');
} else if (MODE === 'append') {
  const add = payloadFor('b64', 'from', 'body');
  const needsNl = has('nl') && before.length > 0 && before[before.length - 1] !== 0x0a;
  next = Buffer.concat([before, needsNl ? Buffer.from('\n') : Buffer.alloc(0), add]);
} else {
  if (!existed)
    die(1, `REFUSED: ${relPosix} does not exist - replace cannot patch a missing file.`);
  const expect = Number(flag('expect'));
  if (!Number.isInteger(expect) || expect < 1) {
    die(
      1,
      `REFUSED: --expect <n> is required and must be >= 1.\n  It is what turns a silent no-op into a failure. Count the occurrences first if you are unsure.`,
    );
  }
  const find = payloadFor('find-b64', 'find-from', 'the search string').toString('utf8');
  const repl = payloadFor('replace-b64', 'replace-from', 'the replacement').toString('utf8');
  if (find.length === 0) die(1, 'REFUSED: the search string is empty.');
  const text = before.toString('utf8');
  const actual = text.split(find).length - 1;
  if (actual !== expect) {
    die(
      1,
      `REFUSED: ${relPosix} - expected ${expect} occurrence(s), found ${actual}. File UNCHANGED.\n  FIX: widen the search string until it is unique, or correct --expect. Nothing was written.`,
    );
  }
  next = Buffer.from(text.split(find).join(repl), 'utf8');
}

if (has('dry-run')) {
  console.log(
    `DRY RUN ${MODE} ${relPosix}: ${before.length} -> ${next.length} bytes (nothing written)`,
  );
  process.exit(0);
}

atomicWrite(abs, next);
console.log(
  `${MODE} ${relPosix}: ${existed ? `${before.length} -> ${next.length}` : `created, ${next.length}`} bytes`,
);
