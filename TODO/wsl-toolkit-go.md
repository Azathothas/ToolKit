# wsl-toolkit-go.md

Entries against the compiled tool under
[`../tools/windows/wsl-toolkit/`](../tools/windows/wsl-toolkit/).
[`wsl-ephemeral.md`](wsl-ephemeral.md) holds the PowerShell tool's entries and
[`issue-6.md`](issue-6.md) holds the one that built this one.

⭐ **Every entry here was found by a consumer running the published binary**,
not by a reading of this tree. That is the provenance worth keeping: eight
defects in a tool whose own gate was green, whose acceptance runner passed 23 of
23, and which had been through three review lenses. A test from outside found
what none of that did.

---

## WSL-32. Helper routing asks whether `wsl.exe` resolves, not whether it answers

**Source** [Issue 7](https://github.com/Azathothas/ToolKit/issues/7), filed by a
consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

The manual says a restricted caller finds the helper without having to know it
is there. It does not. On a host where `wsl.exe` exists and this process may not
talk to it, `run`, `matrix`, `base` and `gc` all take the direct route, fail
with `Wsl/EnumerateDistros/Service/E_ACCESSDENIED`, and then advise starting the
helper that is already running and answering.

## Premise

Measured by the consumer, from an actually restricted process with a live
helper: `helper status --json` returns exit 0 and `listening: true`, while
`run --image alpine -c 'uname -a'` returns exit 2 and the denial. Adding
`--via-helper` to the same command returns exit 0 and a kernel string.

`cmd_helper.go:188` decides on `FindWsl()`, and `internal/toolkit/wsl.go:57`
resolves the executable and returns. Nothing in that path calls `wsl.exe`, so
nothing in it can see a denial. `ErrWslDenied` exists and is produced by
`classifyWslFailure`, which runs on the output of a call that the routing
decision never makes.

## Approach

Probe rather than resolve. `useHelper` asks the handle a bounded question whose
answer is the access fact, and routes on that. The probe is one enumeration with
a short deadline, cached for the life of the process so a fleet does not pay for
it per row.

⛔ **Not a retry after a side effect.** Routing is decided before a job exists.
A design that ran the job, saw a denial and re-ran it through the helper would
run some jobs twice, and the second half of a partially applied `gc` is not
something to repeat.

⛔ **A probe failure that is not a denial is not a reason to route.** A timeout,
a missing distribution list and a denial are three answers, and only the third
is what the helper exists for.

## Consumers

None. `launcher.ps1` selects a binary and does not reach this decision, and no
row of [`../docs/consumers.md`](../docs/consumers.md) pins the routing.

## Prove

```bash
go test ./... -run TestRouting
```

A case where the probe reports a denial routes to the helper; a case where it
succeeds stays direct; a case where the probe fails for any other reason reports
that reason rather than routing. Plus an acceptance case that reaches the helper
with no `--via-helper` anywhere on the command line.


## Closing

**Closed 2026-09-10.** `useHelper` now routes on `ProbeWsl`, which calls
`wsl --list --quiet` with a 20 second deadline and reads the answer through the
same `classifyWslFailure` every other call uses. The result is cached for the
life of the process, so a twelve-row fleet pays for one probe. The decision
itself is `decideRoute`, which touches nothing and has a case per branch.

⚠ **An empty machine is not a denial.** `wsl --list --quiet` exits nonzero
with "no installed distributions" on a host where WSL works, so only a denial
marker routes; every other failure takes the direct path, which reports the real
reason itself. A probe that read any failure as a refusal would send every fresh
host to a helper that is not there.

```text
$ go test ./... -run 'TestRouting'
=== RUN   TestRouting
--- PASS: TestRouting (0.00s)
=== RUN   TestRoutingDoesNotDecideOnAPathLookup
--- PASS: TestRoutingDoesNotDecideOnAPathLookup (0.00s)
ok      github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit
```

Eight routing cases, and the acceptance runner's `a process that can reach
wsl.exe keeps the direct route` proves the positive half against a real machine.

⛔ **The denied half cannot be proved on this host**, because this session's
process is not sandboxed. It is proved by `TestRouting`, whose input is the
probe's error rather than the machine's state, and the mutation pass confirms
the case goes red when the probe stops being consulted.

---

## WSL-33. A failed artifact transfer exits 0 and the output is then destroyed

**Source** [Issue 8](https://github.com/Azathothas/ToolKit/issues/8), filed by a
consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

A job whose container succeeded and whose requested artifacts could not be
fetched exits 0. The JSON carries `exit: 0, artifacts: 0` beside an `error`
naming the refusal, a `matrix` over two such rows reports `ran: 2, failed: 0`,
and the guest directory holding the output is removed on the way out. A build
that produced its deliverables can lose them while the caller's CI records a
pass.

## Premise

Measured by the consumer: three commands, three exits of 0, one of them a
size-ceiling refusal printed on stderr. `job.go:274` sets `res.Error` for a
`FetchArtifacts` failure and leaves `res.Exit` alone; `cmd_run.go:175` reads
`res.Exit`, `res.TimedOut` and `res.Unreached` and never `res.Error`;
`matrix.go:153` counts the same three fields.

## Approach

Carry the transfer's outcome as its own field rather than folding it into the
command's. `JobResult` gains an artifact error distinct from `Error`, the
verdict reads it, and `matrix` counts a row whose transfer failed as failed.

⭐ **The container's own exit code still wins when it is nonzero.** A job that
exited 7 and could not deliver its output is a job that exited 7; flattening
that to a transfer failure would lose the more specific fact.

⛔ **The teardown does not run when the transfer failed.** The guest directory
is what the output can still be recovered from, and removing it is the half of
this defect that cannot be undone by re-reading a result.

## Decision

**Ruled by the operator on 2026-09-09.** Container exit wins when nonzero; a
container that exited 0 whose artifacts failed exits 1, and the row counts as
failed. The job's guest directory is kept, its path printed, and `gc` collects
it later under the age policy [WSL-36](#wsl-36-gc-removes-every-container-before-it-checks-any-age)
sets. The alternative, tearing down and documenting a re-run policy, lost
because a fleet row that took eight minutes is not something a caller re-runs to
recover a file that already exists.

## Consumers

None directly, but the exit code of `run` is what a caller's CI reads. A
consumer whose pipeline currently passes over a failed transfer will start
failing, which is the point. It is a behaviour change and belongs in the
changelog.

## Prove

```bash
go test ./... -run 'TestVerdict|TestMatrixCounts'
```

Cases: a refused artifact name with the container at 0 exits 1; the same with
the container at 7 exits 7; a fleet with one such row reports one failure; the
guest directory named in the result still exists afterwards.


## Closing

**Closed 2026-09-10.** `JobResult.ArtifactError` is the transfer's own outcome,
`JobResult.Failed()` is the one definition both the single-job verdict and the
fleet's counts read, and `MatrixReport.Recount()` is the one place the counts are
derived. A container that exited 0 whose artifacts failed exits 1; a container
that exited 7 still exits 7.

⭐ **The guest directory survives**, its path is on the result as
`guest_dir` and printed on stderr, and `gc` collects it later under the age
policy [WSL-36](#wsl-36-gc-removes-every-container-before-it-checks-any-age) sets.

```text
$ pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary .tmp/wsl-toolkit.exe
  ok    a job whose artifacts are refused exits nonzero and keeps the guest copy
  ok    a container that failed AND lost its artifacts reports the container code
```

⚠ **A correction to the approach above.** It said the teardown does not
run when the transfer failed, and that is what was built; the acceptance run then
showed the job's LEDGER RECORD was also being left open, which made cleanup read
the kept directory as live work and spare it at every age. A deliberate hold had
become a permanent leak. The record closes with a note saying why the directory
was kept; the directory does not. `TestAKeptJobIsNotLiveForever` covers it.

---

## WSL-34. Two artifact names that differ only in case become one file

**Source** [Issue 9](https://github.com/Azathothas/ToolKit/issues/9), filed by a
consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

A container that writes `/out/Result` and `/out/result` returns one file on
Windows. A container that writes `/out/normal.txt:stream` returns a zero-byte
`normal.txt` and the payload is gone. Both jobs exit 0 and both report their
artifact counts as though nothing was lost.

## Premise

Measured by the consumer on an ordinary NTFS directory: the case pair returned a
single five-byte file, and the stream name returned an empty file with only its
default data stream. `workspace.go:389` refuses traversal, rooted paths and
device names and stops there; the colon test at `:409` looks at position 1 only,
so `name:stream` passes. `extractInto:462` opens with `O_TRUNC`, so the second
of two entries that resolve to one path overwrites the first.

⚠ The consumer states the link case was not reproduced: creating a symbolic link
needed a privilege their host did not have. The `.link.txt` sidecar collision is
therefore believed and not measured, and this entry treats it as a case to write
rather than a defect to confirm.

## Approach

Validate the Windows filename grammar in full, and keep a set of destinations
already committed. Refuse a colon anywhere except a drive letter's own position,
refuse a trailing dot or space on any component, and refuse a second entry whose
case-folded path matches one already written.

⛔ **The rule runs on every host, not only on Windows.** `SafeArchiveName`
already splits on both separators for this reason: the artifacts are for a
Windows caller whatever machine validated them, and a rule that only fires on
Windows is a rule the Linux CI job cannot test.

⛔ **A collision is a refusal, not a rename.** A tool that wrote `result_2` would
be a tool whose output a caller cannot predict, and the container knows what it
named its files.

## Consumers

None. Artifact naming is not a fetched contract.

## Prove

```bash
go test ./... -run TestSafeArchiveName
```

Table cases for: a colon in any position, a trailing dot, a trailing space, a
case-folded duplicate, a `.link.txt` name colliding with a real entry, and the
device names that already pass. Plus an acceptance case running a real container
that writes a case pair.


## Closing

**Closed 2026-09-10.** `checkWindowsComponent` applies the destination's whole
name grammar to every component of every archive entry, on every host: the
characters `< > : " | ? *`, control characters, a trailing dot or space, and the
reserved device names that were already refused. `extractInto` keeps a set of
case-folded destinations and refuses a second entry that resolves onto an earlier
one, naming both.

⛔ **The `.link.txt` sidecar goes through the same set**, so a real file
called `x.link.txt` can no longer be overwritten by the record of a link called
`x`. The consumer flagged that case as believed rather than measured, and it is
covered now whichever it was.

```text
$ go test ./... -run 'TestSafeArchiveName|TestExtract|TestDestinationKey'
--- PASS: TestSafeArchiveNameRefusesWindowsGrammar (0.00s)
--- PASS: TestExtractRefusesCaseCollision (0.01s)
--- PASS: TestExtractAcceptsDistinctNames (0.01s)
--- PASS: TestDestinationKeyFoldsCase (0.00s)
```

Twenty refused names, ten legitimate ones, and two acceptance cases running real
containers that write a case pair and an alternate data stream.

⚠ **The case rule is an approximation of NTFS's own table.** The volume
compares with an uppercase map fixed when it was created, which this cannot read.
Lowercasing agrees with it for every name a build produces and errs toward
refusing, which is the safe direction for a rule about losing data.

---

## WSL-35. Output is bounded without saying so and arrives only at the end

**Source** [Issue 10](https://github.com/Azathothas/ToolKit/issues/10), filed by
a consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P1, **Effort** L, **Status** done

## Problem

Two separate things a caller cannot see. A command writing 9 MiB to stdout gets
exactly 8,388,608 bytes back, with no field, no warning and exit 0; stderr is
cut at 2,097,152 the same way. And nothing at all appears until the job is over,
so a twelve-image fleet is 72 seconds of silence and a stuck compiler looks the
same as a working one.

## Premise

Measured by the consumer with byte-preserving capture: the trailing marker is
absent in both cases and both exit 0. `job.go:248` builds two `boundedBuffer`
instances at those sizes, `process.go:44` drops the overflow and records a
`truncated` flag on the buffer, and nothing reads that flag. `cmd_run.go:160`
writes the captured strings after `Run` returns. The helper returns one JSON
response when the job is over, so the restricted route cannot stream at all.

## Approach

Two changes, and they are independent. The command's bytes are written through
to this process's own streams as they arrive rather than replayed, and the
complete transcript is spooled to a file on disk so nothing is bounded by
memory. The in-memory copy stays bounded for the structured answer and gains
`stdout_bytes`, `stdout_truncated` and their stderr pair, so a reader of the
JSON can tell.

For the helper, `/v1/run` and `/v1/matrix` answer with a stream of newline
framed events rather than one object at the end. The client renders each as it
arrives and reconstructs the same result from the last one.

⛔ **The bytes are not reflowed, prefixed or decoded on the way through.** A
caller reading a value off stdout gets what the container wrote.

## Decision

**Ruled by the operator on 2026-09-09.** Both routes stream. The alternative,
streaming the direct route and giving the helper only truncation metadata, lost
because the restricted route is the one the issue was filed from and the one a
sandboxed agent has no alternative to. It costs a helper protocol version.

## Consumers

None. The helper protocol is internal to this executable, and a client and a
helper of different builds already refuse each other by version.

## Prove

```bash
go test ./... -run 'TestSpool|TestHelperStream'
```

Payloads at one byte under, exactly at, and over each limit, each with a trailing
marker: the spooled transcript carries the marker in every case, the JSON says
whether the in-memory copy was cut, and the streamed events arrive before the job
ends. Plus an acceptance case reading a marker printed after 9 MiB.


## Closing

**Closed 2026-09-10.** Three sinks, one write. The caller's own streams get the
container's bytes as they arrive; a transcript on disk gets all of them,
unbounded; and the bounded copy for the structured answer now reports
`stdout_bytes`, `stderr_bytes`, `stdout_truncated` and `stderr_truncated`.
`--max-output` sets what the answer keeps and does not bound the transcript.

The helper's `/v1/run` and `/v1/matrix` answer with newline-framed events rather
than one object at the end, so the restricted route shows a build as it happens
and a fleet announces each row as it finishes. The protocol version moved to 2.

⛔ **A stream that ends without a final event is an error.** That is a
helper that died mid-job, and returning what arrived so far would report a killed
run as a finished one.

```text
$ go test ./... -run 'TestJobStreams|TestReadHelperEvents|TestChunkWriter|TestMaxOutput'
--- PASS: TestJobStreamsSpoolTheCompleteOutput (0.03s)
--- PASS: TestJobStreamsBelowTheLimitAreNotTruncated (0.01s)
--- PASS: TestJobStreamsWithoutAHomeStillWork (0.00s)
--- PASS: TestReadHelperEventsRequiresAFinalResult (0.00s)
--- PASS: TestReadHelperEventsDeliversInOrder (0.00s)
--- PASS: TestReadHelperEventsSurfacesAnError (0.00s)
--- PASS: TestChunkWriterEncodesArbitraryBytes (0.00s)
--- PASS: TestMaxOutputRaisesWhatTheAnswerKeeps (0.00s)
```

```text
  ok    output past the capture limit keeps its last byte in the transcript
  ok    logs writes a job transcript back, complete
```

`wsl-toolkit logs` reads a transcript back and is the other half of this: a path
on a result that no command can open is a recovery nobody performs. It answers on
both routes, because the client spools the bytes it receives as well.

---

## WSL-36. `gc` removes every container before it checks any age

**Source** [Issue 11](https://github.com/Azathothas/ToolKit/issues/11), filed by
a consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

`gc --apply --older-than 24h` killed a job that had been running for seconds.
The container was force-removed, the job exited 137 with its work half done, and
`gc` exited 0 reporting a successful cleanup. The manual's own unattended-cleanup
example is the command that does this.

## Premise

Measured by the consumer with a deliberately created 40-second job. The dry run
listed the live container; the apply removed it. `resources.go:292` loops over
`podman ps -a --filter label=...` and force-removes each one, and the age test at
`:297` applies to directories only. The plan at `:252` is built from an unfiltered
list, so the dry run and the apply disagree about what "older than" means.

## Approach

One filtered set, computed once, used by both the plan and the apply. The age
test moves onto containers, and a container or directory belonging to an open
ledger record is skipped and named.

⭐ **The plan is generated from the same set the apply consumes.** A dry run that
lists more than the apply removes is a dry run nobody can act on, and this defect
is what that shape produces.

## Decision

**Ruled by the operator on 2026-09-09.** Live work is skipped by default and
named in the plan, and `--include-live` is required to remove it. Refusing to
run at all while any job is live lost because one long fleet would then block
every cleanup for its duration; having no override at all lost because a wedged
job needs a supported way out.

## Consumers

None.

## Prove

```bash
go test ./... -run TestCleanupPlan
```

A fresh container is absent from the plan and present under `--include-live`; an
old orphan is present in both; a container exactly at the boundary is decided one
way and the case says which. Plus an acceptance case running `gc --apply` beside
a live job and asserting the job's own exit code afterwards.


## Closing

**Closed 2026-09-10.** `CleanupPolicy.Select` is the whole decision and it touches
nothing: the plan prints what it returns and the apply consumes the same slice, so
a dry run cannot list what an apply spares. The age test applies to containers as
well as directories, a container's age being its job directory's. Anything the
engine reports as running, or belonging to a job whose ledger record is still
open, is skipped and named under `Kept` with the reason. `--include-live` is the
only way to remove it.

⛔ **`applyGuestCleanup` is given a list and does not enumerate.** The
script it replaced re-globbed inside the guest at the moment of removal, so
whatever the plan had decided was irrelevant by the time anything was deleted.

```text
$ go test ./... -run 'TestCleanupPlan|TestCleanupOrder|TestStillRunning'
--- PASS: TestCleanupPlanSparesLiveWork (0.00s)
--- PASS: TestCleanupPlanIncludeLive (0.00s)
--- PASS: TestCleanupPlanAgeBoundary (0.00s)
--- PASS: TestCleanupOrderPutsContainersFirst (0.00s)
--- PASS: TestStillRunning (0.00s)
```

```text
  ok    gc --apply leaves a job that is running right now alone
```

That acceptance case starts a real 45-second job, waits on the CONDITION of its
container appearing rather than on a duration, runs `gc --apply --older-than 24h`
beside it, and then asserts the job's OWN exit code is 0 and that
`live-job-finished` reached its stdout. Against `wsl-toolkit-v1.1.0` the same job
exited 137.

⚠ **An open record is evidence and evidence goes stale.** A killed run
also leaves one, so treating it as live forever would mean cleanup can never
collect the leftovers of a crash. A record past its own deadline, or past six
hours when it carries none, stops counting; a running container counts whatever
its record says, because that signal is exact.

---

## WSL-37. The helper never releases an upload or an artifact set

**Source** [Issue 12](https://github.com/Azathothas/ToolKit/issues/12), filed by
a consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P2, **Effort** M, **Status** done

## Problem

After a batch of successful helper jobs and a `gc --apply` with no age limit, two
upload directories and thirteen artifact directories were still on disk. `gc`
reported nothing to do and `resources` did not mention them, so a long-lived
helper grows without any command being able to see it.

## Premise

Measured by the consumer by listing the helper's state directory after cleanup.
`helper.go:269` writes an upload to `home/uploads/<id>` and records it in a map;
`handleRun` and `handleMatrix` never remove either. `cleanupStaging` runs on
shutdown only, so a crash loses the map that names what to delete. Artifacts land
in `home/artifacts/<id>` and `handleArtifacts` streams without acknowledging.
`resources.go:270` scans `home/jobs` and nothing else.

## Approach

Give both a lifetime with an end. An upload is released when the job that named
it finishes; an artifact set is released when the client has read it, and expires
on age when the client never comes back. Both directories join the survey and the
cleanup plan under the same age and liveness rules as everything else.

⛔ **The map is not the record.** Recovery after a crash reads the directory, so
an entry whose owner is gone is collectable without the process that made it.

## Consumers

None.

## Prove

```bash
go test ./... -run TestHelperLifecycle
```

A completed job leaves no upload; a downloaded artifact set is released; an
abandoned one is in the plan once it is old enough; a helper started over a state
directory holding both collects them. Plus an acceptance case asserting the two
directories are empty after a helper job and a cleanup.


## Closing

**Closed 2026-09-10.** An upload is released by the job that named it, and an
artifact set when the client says it arrived. Both are recorded in the ledger
before they are written, so a helper that was killed leaves something cleanup can
find without the in-memory map. `home/uploads`, `home/artifacts` and the client's
own `home/incoming` are in `resources` and in the cleanup plan under the same age
and liveness rules as everything else.

⛔ **The helper does not delete on a successful write to the socket.**
Bytes leaving is not the same fact as bytes landing, and deleting on the first is
the shape of the defect where a job's guest output was torn down after a transfer
that had failed. The client sends `DELETE /v1/artifacts` once it has extracted;
anything nobody acknowledges is collected by age.

```text
  ok    a helper job leaves no uploaded workspace or artifact set behind
```

That case runs a real helper over its own state directory, runs a job with a
workspace and an artifact destination, and then asserts both directories are
empty. After the consumer's batch against `wsl-toolkit-v1.1.0` there were two
uploads and thirteen artifact sets left.

⚠ **A gap this found in its own fix.** `closeStaleRecords` only looked at
`GuestDir`, and an upload or artifact record names a `HostDir`, so every one of
them stayed open forever and the directories were spared as live work. It reads
both now.

---

## WSL-38. The CLI accepts a trailing word and silently drops what follows it

**Source** [Issue 13](https://github.com/Azathothas/ToolKit/issues/13), filed by
a consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P2, **Effort** S, **Status** done

## Problem

Three refusals that are not made. A stray word after the flags is ignored, and so
is every flag after it, so `--via-helper` written last does nothing and the
command tries a route the caller did not ask for. `--env BAD-NAME=value` is
accepted and then dropped, so the container runs without it. `--timeout -1s` runs
unbounded.

## Premise

Measured by the consumer: all three exit 0, and the first prints the output of a
command that should not have run. Go's `flag` stops at the first non-flag
argument and leaves the rest in `Args()`, which `cmd_run.go:102` never reads.
`envMap:81` checks for an equals sign and nothing else, and `job.go:323` drops
the names that fail `isShellName` without a word. The duration is used only when
positive.

## Approach

One refusal helper called by every subcommand after `Parse`, so a command added
later cannot forget it. Environment names are validated where they are parsed,
which is the place that still knows what the caller typed. A negative timeout is
refused by name and zero keeps its documented meaning.

⭐ **`doctor` already has the check.** This makes the other seven agree with it
rather than inventing anything.

## Consumers

None. The refused spellings are ones that already did not work.

## Prove

```bash
go test ./... -run TestFlagRefusals
```

Every subcommand refuses a trailing positional; a bad environment name is refused
with the name in the message; a negative timeout is refused; a zero timeout is
accepted and documented. Each case asserts the payload never ran.


## Closing

**Closed 2026-09-10.** `parseArgs` wraps every subcommand's parse and refuses a
positional argument, naming the stray word and saying that everything after it
was never looked at. Environment names are validated where the caller typed them,
with the name in the message. A negative timeout, a non-positive `--max-bytes`,
`--max-entries` or `--max-output`, and a `--parallel` below one are all refused
before any job or base side effect.

⭐ **`doctor` had the check and the other seven did not.** That is what a
guard written at a call site becomes; it is in the parse now, so a command added
later cannot forget it.

```text
$ go test ./... -run 'TestFlagRefusals|TestEveryFlagSet|TestJobFlagsRefuse|TestEnvNames'
--- PASS: TestFlagRefusalsRejectPositionals (0.00s)
--- PASS: TestFlagRefusalsAcceptOrdinaryFlags (0.00s)
--- PASS: TestEveryFlagSetRefusesPositionals (0.00s)
--- PASS: TestJobFlagsRefuseValuesThatMeanSomethingElse (0.00s)
--- PASS: TestEnvNamesAreRefusedWhereTheyAreTyped (0.00s)
```

```text
  ok    a stray word and the options after it are refused, not ignored
  ok    an unusable environment name is refused where it was typed
  ok    a negative timeout is refused rather than meaning unlimited
```

Each acceptance case asserts the payload never ran, not only that the exit code
moved: against `wsl-toolkit-v1.1.0` the first of them printed `SHOULD-NOT-RUN`
and exited 0.

⚠ **Asking for help is not a failure.** `-h` reaches `parseArgs` as an
error, and every subcommand was reporting it as `wsl-toolkit: flag: help
requested` beside exit 2. It exits 0 quietly now, since the defaults have already
been printed.

---

## WSL-39. An image that could not be pulled counts as a job that ran and failed

**Source** [Issue 14](https://github.com/Azathothas/ToolKit/issues/14), filed by
a consumer agent against `wsl-toolkit-v1.1.0` on 2026-09-09.
**Category** wsl, **Priority** P2, **Effort** M, **Status** done

## Problem

`run` against a tag the registry does not have returns exit 125 with
`unreached: false`. Nothing ran, and the result says a container ran and exited
125. In a fleet that row counts under `ran` and `failed`, which is the exact
distinction the three counts exist to make.

## Premise

Measured by the consumer against a tag that does not exist: the registry said
manifest unknown, no payload ran, and the JSON was
`{"exit":125,"artifacts":0,"timed_out":false,"unreached":false}`. `job.go:256`
takes podman's status for the whole invocation as the container's own, so a pull
failure, a create failure and a payload exiting 125 are one number.

⚠ The fleet consequence is derived from the shared code path rather than measured:
the consumer ran one job, not a failing catalog.

## Approach

Separate acquisition from execution at the engine boundary. The container script
writes a marker the moment the payload is entered, and a result with no marker is
unreached whatever the status was.

⛔ **Not a test on 125, 126 or 127.** A real payload returns those values, and a
classifier keyed to them would call a working job unreached. The issue says so
and it is right.

## Consumers

None.

## Prove

```bash
go test ./... -run TestUnreached
```

An unavailable tag is unreached; a payload that itself exits 125 is not; a fleet
mixing the two reports one of each; a fleet where every row is unreached exits 2.
Plus an acceptance case against a tag that does not exist.

## Closing

**Closed 2026-09-10.** The payload is entered through a wrapper that writes a
per-job random marker to stderr from INSIDE the container, and a result with no
marker and a nonzero status is `unreached`. So a tag the registry does not have,
a container that could not be created and a `--user` that does not exist are all
separable from a program that itself exited 125.

⛔ **Nothing is inferred from 125, 126 or 127.** Those are podman's
conventions and legal values for a real payload, and a classifier keyed to them
calls a working job unreached. The issue said so and it was right.

⚠ **The marker is stripped from the stream before any sink sees it**, by a
filter that holds back at most the marker's own length so a token split across
two writes is still matched. `TestMarkerStripperFindsASplitToken` runs every one
of the 47 possible split points.

```text
$ go test ./... -run 'TestMarker|TestNewMarkerToken|TestPartialSuffix|TestUnreached'
--- PASS: TestMarkerStripperFindsASplitToken (0.00s)
--- PASS: TestMarkerStripperPassesOutputThatHasNoToken (0.00s)
--- PASS: TestNewMarkerTokenIsPerJob (0.00s)
--- PASS: TestPartialSuffix (0.00s)
--- PASS: TestUnreachedIsNotInferredFromAStatus (0.00s)
```

```text
  ok    an image the registry does not have is unreached, not a failed job
  ok    a payload that exits 125 is a job that ran
```

The two acceptance cases are the pair: a tag that does not exist reports
`unreached: true` at exit 2, and `-c 'exit 125'` reports `unreached: false` at
exit 125. A rule that read the status alone cannot pass both.

