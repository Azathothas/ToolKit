# HISTORY: wsl-ephemeral.ps1

⛔ **Superseded. Nothing here is needed to use the tool.**
[`../../scripts/powershell-windows/wsl-ephemeral.md`](../../scripts/powershell-windows/wsl-ephemeral.md)
is the live page and says what the tool does now.

This holds the defects it shipped, the shapes its behaviour used to have, and
the measurements that produced the current design. Each was on the live page and
was moved here on 2026-08-30, because a reader using the tool cannot act on any
of it: the code no longer has the defect and there is nothing to do about it.

---

## Exit codes

**`New -Command` used to exit 0 over a failing command.** `Run` propagated the
inner code and `New` warned and exited 0, so every green result downstream of a
`New` meant nothing. `WSL-01`, closed 2026-08-27, and a deliberate break: a
caller written against the old behaviour is now told the truth.

**The final `ERROR: ...` line moved from stdout to stderr** on 2026-08-29. Not a
break by [`../consumers.md`](../consumers.md)'s definition, and recorded because
it is the one change of that batch a caller could observe.

**`New` gained three failures it used to paper over**, in the `WSL-06` to
`WSL-11` batch: it can exit 1 where it used to start an import it could not
finish, where it used to hang on a wedged distro, and where `-Systemd` was asked
for and could not be given.

## The command channel

**Two of the three payloads were hand-written inside a safe alphabet with
nothing enforcing it.** The smoke probe carried a bracket inside a double-quoted
echo, and every `-Action New` failed on Windows PowerShell 5.1 with
`syntax error: unterminated quoted string`. `WSL-12`, a P0. The fix was one
function building every payload and asserting the result stays inside the
measured alphabet.

**The transport file was created before it was unlinked, and the order is the
point.** Writing the file and then unlinking it reads the same in a diff and is
not: a redirect creates the file before the decode runs, so a guest with no
`base64` was left holding an empty one that nothing removed. The mutation that
planted a missing decoder is what found it.

**A `$` in a command was expanded in transit and its RESULT re-parsed**, so
`echo $PATH` died with `syntax error: unexpected "("` on the bracket in
`Program Files (x86)`. `WSL-08`. That is the measurement that made base64 the
only channel.

**`-CommandFile` used to warn about CRLF and send the bytes anyway.** The
reasoning was that silently editing somebody's payload is the failure the
channel exists to remove, and it was half right: the FILE is the caller's and is
never written to, but the COPY IN TRANSIT is the tool's to make correct. Changed
on 2026-08-30, `WSL-16`, with `-Verbatim` for a caller who meant the carriage
returns.

## Removal

**A delete that failed was reported as a delete that worked.** `Remove-Item
-ErrorAction SilentlyContinue` followed by an unconditional "deleted" left
multi-gigabyte VHDX files behind while reporting them gone. `WSL-04`. The fix
reads the directory back and exits non-zero when the path is still there.

**There were four deletion paths and the page claimed there was one.** The
temporary rootfs tarball had its own `Remove-Item`, no containment guard and no
read-back, while
[`../../scripts/powershell-windows/wsl-ephemeral.md`](../../scripts/powershell-windows/wsl-ephemeral.md)
said every path reached one deletion. A door sweep found it; the claim was false
for one commit.

**The rollback after a failed create went straight to `--unregister`** while
`Remove-EphemeralDistro` ran `--terminate` first. ⚠ That race has never been
reproduced on this machine, so the change was two paths agreeing rather than a
measured bug fix.

## The disk preflight

**"Roughly twice the rootfs size" was the premise and it was wrong.** Measured
on 2026-08-27, VHDX on disk against the rootfs tarball that produced it:

| image | rootfs `.tar` | VHDX on disk | ratio |
| --- | --- | --- | --- |
| `alpine:3.22` | 8.2 MiB | 76.0 MiB | ⛔ 9.27x |
| `python:3.13-alpine` | 45.4 MiB | 140.0 MiB | 3.08x |
| `debian:bookworm-slim` | 74.3 MiB | 172.0 MiB | 2.31x |
| `ubuntu:24.04` | 76.9 MiB | 172.0 MiB | 2.24x |

An 8 MiB rootfs costs 76 MiB, so the cost is dominated by a fixed floor rather
than by a multiple of the input. The requirement became 256 MiB plus twice the
tarball. `WSL-06`.

## systemd

**Written into an image with no systemd, the flag was a setting nothing read.**
Measured before the check existed, `ubuntu:24.04` carried on with its own init
and said nothing at all, and `alpine:3.22` spent 20 seconds failing and then
also carried on. A caller would come away believing they had systemd. `WSL-07`
added the `/proc/1/comm` read that makes the switch verify its own effect.

## The timeout

**A distro whose init wedged hung the script forever with no output at all.**
There was a bounded sleep loop inside the guest for the drvfs race, and the
outer call had no limit, so anything that never answered never returned.
`WSL-09`.

**`-TimeoutSeconds` shipped for one run doing nothing.** A `$script:TimeoutSeconds = 120`
in the constants block silently overwrote the parameter of that name, because a
script parameter IS a script-scoped variable. `-TimeoutSeconds 15` timed out at
120. The acceptance caught it because it asserted the number and not just the
refusal.

## Naming and the two read-only actions

**A name collision used to throw whether the caller chose the name or not.**
`WSL-10` split them: a generated name is drawn again, up to eight times, and a
name the caller gave is refused, because silently using a different one would be
worse.

**Answering what a distro reaches the host at used to mean building one.** A
caller created a throwaway VM, read `/proc/net/route` inside it and decoded
little-endian hex, to answer a question the host already knew. `WSL-14` measured
the two against each other on 2026-08-29: `.wslconfig` said `nat`, the adapter
was `vEthernet (WSL (Hyper-V firewall))`, `-Action HostAddress` answered
`172.23.96.1`, and `/proc/net/route` inside a real `alpine:3.22` distro said
`016017AC`, which is the same address.

## The launcher

**Its first version built the forwarded argument list with an
`ArrayList.ToArray()`**, and no argument reached the wrapped script correctly:
`-Action` bound positionally as the VALUE of `-Action`. Measured on 2026-08-29,
four other ways of building the same array forwarded the name correctly, and
every element was a `System.String` in all five.

**It printed its progress with `Write-Host`**, which goes to the information
stream in-process and to real stdout out of process. A caller capturing
`-Action HostAddress` through the launcher got
`==> Using the copy beside this launcher` ahead of the address. `WSL-15`.

**The sibling beside the launcher used to win over an explicit `-LauncherRef`.**
A run passing both a commit and a digest printed `Using the copy beside this
launcher`, ran a stale file and verified nothing. `Azathothas/bit-cli` hit it,
worked around it by deleting the sibling before every call, and wrote the
workaround into its own documentation. Reversed on 2026-08-30, `WSL-17`.

## Measured on 2026-08-30, while the stream log was being built

**A byte array is not two byte arrays.** `@($a) + @($b)` on two `[byte[]]`
unrolls both into an `[object[]]` of mixed `byte[]` and `byte`, and the cast
back fails with a message naming `System.Byte[]` and neither array.
`[Array]::Copy` is the fix. Found by the selftest on its first run, before any
distro existed to hit it.

**A PowerShell hashtable folded `%m` into `%M`.** Hashtable keys are
case-insensitive, so a strftime specifier table written as a literal `@{}`
refused to parse with "Duplicate keys 'M' are not allowed". That is the loud
version; the quiet version would have rendered a month where a minute belonged.
An ordinal `Dictionary[string, string]` is the fix.

**A tick advanced the delta clock.** A command that went quiet for five seconds
and then printed showed `+0.619` against the last tick, on stdout, where the
ticks are not even present: they go to stderr. Found by driving a real distro,
not by reading.

**`Invoke-WebRequest`'s `.Content` is not always a string.** With
`Accept: application/vnd.github.sha` it arrives as a `byte[]`, and `[string]` on
a byte array joins the DECIMAL BYTE VALUES with spaces: the commit `8efe6e02`
came back as `56 101 102 101 54 101 48 50 ...`, which is forty numbers that are
really the right answer in the wrong alphabet.
