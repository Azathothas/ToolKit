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

⛔ **AMENDED 2026-09-10: THE SENTENCE ABOVE IS NOT TRUE.** The marker is
stripped and the newline in front of it is NOT. The wrapper emits
`printf '\n%s\n'`, and the stripper writes that leading newline back to the
destination, so every job with no stderr returns one byte of it. A consumer agent
found it against `wsl-toolkit-v1.3.0` and filed
[issue 23](https://github.com/Azathothas/ToolKit/issues/23);
[WSL-46](wsl-toolkit-go.md) carries the fix. The sentence stays because the
correction belongs underneath it, and because a test named for a property the
code does not have is the more useful half of this record.

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

---

## WSL-40. What a second reading of the core found

**Source** The operator's third priority on 2026-09-09: the transport, the base
lifecycle, the job model and the fleet are one session old, and this is the pass
that asks what a second reading finds.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

Nothing was reported broken. This is the pass that reads code nobody has read
twice, and three things came out of it. All three are in the published
`wsl-toolkit-v1.2.0` and in `v1.1.0` before it.

1. `provision` passes ONE `prefixWriter` as both `Stdout` and `Stderr`, and
   `os/exec` runs a copier per stream when the writer is not an `*os.File`. Its
   `seen` buffer locks itself; its `partial` line buffer did not, so the two
   copiers appended to the same slice.
2. A catalog id of `..` passes `isImageID`, because a dot is a legal character
   in `debian12` and in `ubuntu-24.04`. `matrix --artifacts out` writes each row
   into `out/<id>`.
3. `Ledger.Compact` read the file with no lock and then took the lock to write.

## Premise

Measured, not read:

- ⭐ **The race is real and the detector is what makes it RELIABLE to see.**
  With the lock removed, `go test -race` reports `WARNING: DATA RACE` and the
  case goes red in 10 of 10 runs. Without `-race` the same broken version went
  red in 6 of 10, because appending to a shared slice corrupts the output often
  and not always. A guard proved six times in ten is a guard that passes on the
  wrong day, which is why this suite runs under the detector. The symptom in
  production is an interleaved or dropped provisioning line, in the one place
  whose output decides whether the base is usable.
- ⭐ **The id rule accepts every dot name.** A test printed `isImageID("..")`
  and `isDistroName("..")` as true, along with `"."`, `"..."` and `".hidden"`.
- ⚠ **The lost-append window is real and NOT reachable by a test.** Measured:
  the broken `Compact` was run against a 50-append, 20-compaction stress case
  ten times and went red in NONE of them, because `Open` took the lock too and
  an append almost never lands in the gap between its release and `Compact`'s
  acquire.

## Approach

Lock what is shared, bound what grows, and refuse a name that means something
other than what it looks like.

`prefixWriter` takes a mutex, holds it across the buffer work, and releases it
BEFORE calling the caller-supplied logger, because holding a lock across a
callback is how an unrelated function deadlocks a provisioning run. Its
unterminated tail is bounded and FLUSHED rather than dropped past the ceiling: a
long line is still information. `Flush` exists for the line a failing step ends
on, which tends to have no terminator.

`isImageID` and `isDistroName` refuse a leading dot, which removes `.`, `..`,
`...` and `.hidden` while keeping `a..b` and `ubuntu-24.04`.

`Compact` takes the lock once and calls an unlocked `openLocked`, so its read
and its write cannot be separated. `Open` returns a sorted slice, because
`Compact` writes what `Open` returns and a map's order is not one.

⛔ **The third fix is held by STRUCTURE and it is not covered by a test.** A
test that passes ten times out of ten against the defect is worse than no test,
so the case written for it says what it does prove and what it does not.

## Consumers

⚠ **A caller whose configuration carries a dot-named image will start being
refused.** Nothing plausible is in that set, and the refusal names the field. It
is a behaviour change and belongs in the changelog.

## Prove

```bash
go test -race ./...
```

Plus the mutation for each guard that can carry one: the lock removed from
`prefixWriter` reports a data race, and the dot names are a table case.

## Closing

**Closed 2026-09-10.** All three are fixed and shipped in `wsl-toolkit-v1.3.0`.
`prefixWriter` holds a mutex across the buffer work and releases it before
calling the caller's logger; its unterminated tail is bounded at 64 KiB and
FLUSHED rather than dropped, and `provision` calls `Flush` so the line a failing
step ends on is not held forever. `isImageID` and `isDistroName` refuse a leading
dot. `Compact` takes the lock once and calls an unlocked `openLocked`, and `Open`
returns a sorted slice.

```text
$ go test -race ./...
ok  	github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit	2.353s
ok  	github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/script	1.338s
ok  	github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit	3.617s
```

```text
$ sh scripts/common/repo.sh mutate
  ok       the lock that makes one writer safe for two streams  1 case(s), went red
  ok       the flush of a final line with no terminator         1 case(s), went red
  ok       the ceiling on output that never sends a newline     1 case(s), went red
  ok       the leading-dot refusal for an image id              2 case(s), went red
  ok       the leading-dot refusal for a distribution name      1 case(s), went red
  ok       the order Open returns records in                    1 case(s), went red
```

⚠ **One claim in this entry was wrong when it was written and is corrected
here.** The premise said the detector is what makes the race VISIBLE. Measured
ten runs each way against the broken version, it went red in 6 of 10 plain runs
and 10 of 10 under `-race`, so the detector buys DETERMINISM, not visibility. The
premise above now carries those numbers, and the mutation row asks for `-race`
because a guard proved six times in ten is one that passes on the wrong day.

⛔ **The ledger lock is still held by structure and still has no test, on
purpose.** The mutation harness carries a comment saying so rather than a row
that would report theatre every run.

---

## WSL-41. What a failure is allowed to hide

**Source** The fifth review of this session, run as a deliberate lens rather
than a sweep: take the defect class of [issue 10](https://github.com/Azathothas/ToolKit/issues/10)
and [issue 8](https://github.com/Azathothas/ToolKit/issues/8), a failure reported
as a benign or successful outcome, and go looking for the rest of it.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

Five things, found by reading every discarded error in the module and every
`return exitOK` that had a failure above it.

1. `NewClientSpool` had FOUR silent returns. A helper-route job ran, succeeded,
   and `wsl-toolkit logs` found nothing, with no line anywhere naming the step
   that gave up. The direct route's `openSpool` logged the same failure all
   along, so one route told the operator and the other did not.
2. `ClientSpool.Finish` returned the empty string when it could not file the
   transcript under the job id, while a complete transcript sat in the staging
   directory it was holding.
3. `listTranscripts` treated EVERY `os.ReadDir` failure as "no transcripts on
   this machine yet" at exit 0.
4. `helper stop` discarded the reason nothing answered. `DialHelper` fails for
   three different things, one of which is "something else is listening on that
   address and reports a different pid".
5. `writeHelperJSON` carried a comment saying an encoding failure "can only be
   recorded", and nothing recorded it.

⛔ **`ClientSpool` SHIPPED IN v1.2.0 WITH NO UNIT CASE AT ALL.** It is what
makes `logs` work on the helper route, and the only thing covering it was one
acceptance case that exercises the direct route. Three of the five findings are
in that type.

## Premise

Measured, not read:

- ⭐ **Two openers of the same file disagreed.** `openSpool` takes a `log` and
  uses it; `NewClientSpool` opened the same two files and threw the error away.
  The asymmetry is visible in four lines of diff, which is what makes it the kind
  of thing a lens finds and a sweep does not.
- ⚠ **On Windows, finding 3 cannot separate a file from an absence.**
  Measured: `os.ReadDir` on a path that is a FILE returns ERROR_PATH_NOT_FOUND,
  for which `os.IsNotExist` reports true. So a file named `jobs` still reads as
  "not yet", which is a tolerable answer for a situation this tool did not
  create. What the narrowing catches is every OTHER failure.
- ⭐ **The job id was nowhere in the human output.** `run` printed the image
  label and the exit code; `matrix` printed a table of labels. The id a transcript
  is filed under appeared only under `--json`, so `wsl-toolkit logs ID` could not
  be typed without first running `logs` bare to go hunting for it.

## Approach

A failure may be tolerated. It may not be hidden, and it may not be dressed as a
different outcome.

`NewClientSpool` takes the `log` its caller already had and names the step it
gave up on, and it opens its files through the SAME `openSpool` the direct route
uses, so the two cannot drift apart again. `Finish` returns the staging path when
it cannot file the transcript, because the bytes are there, `logs` reads that
path, and gc collects it by age like anything else. `listTranscripts` treats only
a missing directory as "not yet" and refuses the rest by name. `helper stop`
still exits 0, because stopping something that is not running is the outcome the
caller asked for, and now prints the reason. `writeHelperJSON` says plainly that
the error is dropped and why that is right.

Two quality-of-life lines come out of the same reading, because a transcript
nobody can find is the same defect as one that was never written: `run` prints
the command that reads its output back, and the matrix table says once, after the
table rather than per row, that each row's output is kept.

## Consumers

⚠ **`logs` on a machine whose `jobs` path cannot be read now exits 2 where
it used to exit 0 with "no transcripts on this machine yet".** That is the point
of the change and it is a behaviour change, so it belongs in the changelog. A
machine that has simply not run a job still exits 0 and still says so.

Everything else is additive: lines that were not printed before, and a path
returned where the empty string used to be.

## Prove

```bash
go test -race ./...
sh scripts/common/repo.sh mutate
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary .tmp/wsl-toolkit.exe
```

## Closing

**Closed 2026-09-10.** Shipped in `wsl-toolkit-v1.3.0`. Eight unit cases were
written, six of them for `ClientSpool`, which had none; ten guards were proved by
mutation; two acceptance cases were added against a real machine.

```text
$ sh scripts/common/repo.sh mutate
  ok       the line that says this machine has no state directory      3 case(s), went red
  ok       the line that names the step the spool gave up on           3 case(s), went red
  ok       the tee that a failing spool cannot abort                   1 case(s), went red
  ok       the staging removal for a result that carried no id         1 case(s), went red
  ok       yielding to a transcript that is already complete           1 case(s), went red
  ok       the path Finish returns for a transcript it could not file  1 case(s), went red
  ok       the line between an empty machine and an unreadable one     3 case(s), went red
  ok       the job id on a run that kept its output                    5 case(s), went red
  ok       not saying the same thing twice after a truncated run       5 case(s), went red
  ok       the fleet offering a transcript only when one exists        3 case(s), went red

39 of 39 guards proved.
```

```text
  ok    a helper job leaves its transcript on the machine that asked for it
  ok    a run says the command that reads its output back

acceptance: 39 case(s) passed against a real machine.
```

⭐ **The first acceptance case is the one that was missing.** `logs writes a
job transcript back, complete` has always run the DIRECT route. This one asks for
a job through the helper and reads it back with the CLIENT, which is what a
caller on a restricted machine actually does, and it is the only case that
touches `ClientSpool` end to end.

⚠ **Finding 4 has no test of its own.** It is one line of text on a path
that needs `DialHelper` to fail in a specific way, and the existing acceptance
case covers the outcome that matters, that the helper is gone once it is stopped.
Saying so is better than a case that asserts a string.

---

## WSL-42. What this tool owns, and how it proves it

**Source** [Issue 16](https://github.com/Azathothas/ToolKit/issues/16) and
[issue 18](https://github.com/Azathothas/ToolKit/issues/18), filed by a consumer
agent against the published `wsl-toolkit-v1.3.0` on 2026-09-10.
**Category** wsl, **Priority** P1, **Effort** L, **Status** done

## Problem

Two defects, and they are one subject: the tool cannot say which distribution is
its own, and it destroys the evidence of which one it built.

`base.name` is editable in `config.json`. `AssertOwnedDistro(name, baseName)`
then compares a configured value against itself, so the check proves only that
a name equals itself. `base ensure` will create any syntactically valid name and
`base remove --yes` will unregister it. The manual teaches an agent that every
name other than `wsl-toolkit` is structurally refused, which is the opposite of
what the binary does.

`base ensure` also calls `writeRecord()` unconditionally on the
registered-and-healthy path, using `b.cfg.Base.Image`. A distribution built from
Arch, with the config since changed to Alpine, is relabelled as Alpine because a
health probe ran an Alpine CONTAINER successfully. The probe proves the engine
works. It does not identify the rootfs.

## Premise

Read from the reporter:

- The reporter created and removed a uniquely named disposable distribution and
  confirmed nothing pre-existing was touched.
- After the relabel, `/etc/os-release` still said Arch and the installed Podman
  version matched the Arch build, while `base status` reported no drift.

Verified here against the code on 2026-09-10, both seams exactly as reported:
`AssertOwnedDistro` at `internal/toolkit/wsl.go:238` compares `name` against
`baseName` with `EqualFold`, and every caller passes `cfg.Base.Name` for both.
`Base.Ensure` at `internal/toolkit/base.go:177` calls `writeRecord()` on the
registered-and-healthy branch with no comparison of any kind.

⭐ **THE SECOND ONE ARGUES FOR ITSELF, three lines above the defect.** The
comment at `base.go:175` reads: "Refreshed on every path that leaves a usable
base. A record written once describes the first build forever." Somebody reasoned
about this and reached a conclusion that is half right. A record refreshed on
every healthy path follows the CONFIG, and the thing it claims to describe is the
GUEST. Whoever fixes this should replace that comment rather than delete it,
because the next reader will have the same thought.

## Approach

Ownership stops being a string comparison and becomes a PROOF the guest carries.

Write an identity marker inside the distribution at build time, holding the name
this tool built it under, the image reference, the preset id and the build stamp.
`AssertOwnedDistro` reads that marker rather than comparing config to config, so
a distribution this tool did not build cannot be adopted by editing a file.

`Ensure` compares the record, the config and the marker. A mismatch is reported
and left visible; it never writes a record it did not earn.

⛔ **It must not resolve the mismatch by rebuilding automatically.** A
rebuild destroys the thing the operator needs to look at, and `recreate` already
exists for the case where they want one.

## Decision

The fork is whether the owned name stays fixed at all, and it is the SAME
question [WSL-43](wsl-toolkit-go.md) asks from the other side. The reporter's
expected fix is "reject any `base.name` other than the one hard-owned name". The
operator's requirement is several isolated instances so that agents do not step
on each other.

**RULED 2026-09-10: a fixed PREFIX plus a guest-side marker.** A distribution is
owned when its name matches `wsl-toolkit` or `wsl-toolkit-<suffix>` AND it
carries this tool's identity marker. A name outside the prefix is structurally
refused, which is what the reporter asked for; an unmarked distribution inside
the prefix is refused too, which is what the reporter did NOT ask for and is the
half that actually closes the hole; and the suffix is what makes instances
independent, which is what the operator asked for.

The alternatives and why they lost: a fixed single name closes
[issue 16](https://github.com/Azathothas/ToolKit/issues/16) exactly as filed and
makes [WSL-43](wsl-toolkit-go.md) impossible. A marker with no name rule leaves a
mistyped name creating a real distribution that simply cannot be removed, which
trades one surprise for another.

Two things follow and neither is optional:

- ⛔ **A base built before the marker existed has no marker**, so the
  upgrade path is part of this entry: `ensure` recognises an unmarked
  distribution whose name matches the prefix and whose record this tool wrote,
  writes the marker, and says it did. Anything else is refused with the exact
  command that rebuilds it.
- ⚠ **The marker lives in the guest and the record lives on the host**, so
  they can disagree. The marker wins, because the host file is the one an editor
  can reach.

## Consumers

Breaking, twice. A config carrying an arbitrary `base.name` stops being accepted,
and a base whose provenance does not match its config stops reporting clean. Both
are the point. `docs/consumers.md` rows for the published binary and the manual
both move.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case that writes a foreign name into `base.name` and asserts `base ensure`
refuses by name; a case that builds from one preset, changes the configured image
without a rebuild, runs `ensure`, and asserts the record still names the image
the guest actually carries.

## Closed 2026-09-10

Ownership is TWO facts and both are required: the NAME matches the prefix, and
the GUEST carries a marker this tool wrote.

| where | what |
| --- | --- |
| `internal/toolkit/identity.go` | `IsOwnedName`, `AssertOwnedDistro(name)`, `ValidateOwnedName`, and the read/write of `/etc/wsl-toolkit-identity.json` |
| `Config.Validate` | a `base.name` outside the prefix is refused when the file is READ |
| `Wsl.Unregister` | the one irreversible call, and the one that demands the marker |
| `Base.reconcileIdentity` | the comparison that replaced the relabel |

⛔ **`AssertOwnedDistro` takes ONE name now.** It took `(name, baseName)` and
every caller in the program passed `cfg.Base.Name` for both, so it compared a
value with itself at every call site there was.

⭐ **The comment that argued for the defect is replaced rather than deleted**, as
the entry asked. It read: "Refreshed on every path that leaves a usable base. A
record written once describes the first build forever." Half right: a record
refreshed on every healthy path follows the CONFIG, and the thing it claims to
describe is the GUEST.

⚠ **`base ensure` now exits 1 over a drifted base**, where it used to exit 0
having relabelled its own record. 1 is "it ran and it disagreed", which is what
happened: it was asked to bring the machine to the state a configuration
describes and it could not, short of a rebuild it must not perform on its own.
The acceptance case asserts the 1 rather than working around it.

⚠ **Two refinements came from DRIVING it rather than from reading it.** A
missing marker read as "could not read the marker" instead of "carries none",
because `cat` on a missing file exits 1 and `ExecDirect` reports every nonzero
exit as an error too; and the drift was reported twice, once from the record and
once from the guest, over one disagreement. Both are this tree's own recurring
class, and both were found by running the thing.

### Two instances, and what nearly went wrong

⛔ **A TEST WRITTEN FOR THE PAIRING CAUGHT A FOOTGUN BEFORE IT SHIPPED.** The
first version let an explicit `--home` win outright over an instance's own
directory, on the reasoning that a caller who names a directory means it. That is
wrong in the one case this entry is about: `--home X --instance one` and
`--home X --instance two` would have been two distributions sharing ONE ledger,
one helper endpoint and one transcript directory, with each caller believing it
was isolated. `--home` sets the ROOT and an instance still gets its own directory
under it.

⭐ **`TOOL-17`'s consumer harness is the second thing that argued for this
entry.** It nearly unregistered the operator's own base, because a separate state
directory is not isolation while the distribution name comes from the
configuration. That file sets a name by hand; this entry is the mechanism that
replaces the convention.

### The configuration search

⛔ **The nearest file wins WHOLE, and `config` prints the order.** `--config`,
then `wsl-toolkit.json` in the working directory or the nearest parent, then the
state directory's own file, then the defaults. A `wsl-toolkit.toml` in that
search is REFUSED by name rather than skipped.

⭐ **The name collision the entry raised dissolved with the ruling.**
`.wsl-toolkit/` on the host holds a POINTER naming an instance; `GuestRoot` keeps
its name, and the state itself stays under one home per instance.

⚠ **`config --write` writes the STATE DIRECTORY's file and never the one the
search resolved.** Without that, `config --write` run inside a checkout carrying
a `wsl-toolkit.json` would overwrite a TRACKED file with the whole built-in
catalog, which is a report command editing somebody's repository. That was not in
the entry and it is the kind of thing the entry's own ruling implies.

```text
  ok    a configured name outside the prefix is refused by name
  ok    an instance name inside the prefix is accepted
  ok    the guest own marker decides what the base was built from
  ok    the nearer configuration wins whole and config names the order
  ok    a configuration this tool does not read is refused by name
  ok    two instances share no distribution, state, transcript or artifact

acceptance: 49 case(s) passed against a real machine.
```

The last case builds a SECOND distribution in one run, runs a job in each,
asserts neither sees the other's distribution, state directory, transcript or
artifacts, and that `gc --apply` on one leaves the other whole. It is behind the
`-Quick` switch because it costs a base build. The machine was back to its three
pre-existing distributions afterwards, which the final case asserts by name.

⚠ **The helper endpoint is per instance by construction and has no case of its
own.** It lives at `<state>/helper.json` and the state directory is what an
instance moves, so a case would be asserting that a path built from a different
root is a different path. The transcript assertion covers the same property with
a fact a reader can check.


---

## WSL-43. Many agents, many bases

**Source** The operator on 2026-09-10: several agents should work at once, each
isolated, and all of them still able to use this tooling.
**Category** wsl, **Priority** P1, **Effort** L, **Status** done

## Problem

There is exactly one base, named `wsl-toolkit`, and one state directory. Two
agents on one host share the distribution, the engine, the ledger and the helper
endpoint. Either they collide, or one of them stops using the tool.

## Premise

Verified against the code on 2026-09-10, and the answer is not what the request
assumes.

⛔ **THIS IS ALREADY POSSIBLE TODAY, AND THAT IS EXACTLY WHY IT IS FILED AS A
P1 DEFECT.** `catalog.go:155` lets a stored `base.name` override the constant,
and `--home` or `WSL_TOOLKIT_HOME` already moves the whole state directory,
helper endpoint included. Two agents can be fully isolated right now:

```bash
wsl-toolkit --home C:\agents\one   base ensure   # base.name: wsl-toolkit-one
wsl-toolkit --home C:\agents\two   base ensure   # base.name: wsl-toolkit-two
```

It works because `AssertOwnedDistro` compares a configured name against itself,
which is [issue 16](https://github.com/Azathothas/ToolKit/issues/16). The
isolation is real; the guard behind it is not.

So this entry is NOT "build multi-instance support". It is: keep the isolation
that already works, put a real ownership proof under it, and remove the two
things that still make it manual, which are the naming and the pairing of a
distribution with its state directory.

⚠ **An operator who needs isolation before this entry is built can have it
today** with the two flags above, accepting that the ownership guard is the one
`WSL-42` describes.

## Approach

An instance is a name and a state directory together.

`--instance NAME` (and `WSL_TOOLKIT_INSTANCE`) selects both: distribution
`wsl-toolkit-<name>` and state under a matching subdirectory, so one flag makes
two agents independent. With no name, allocate the lowest free
`wsl-toolkit-<N>` and record the choice, so an agent that does not care never
has to think about it.

The helper is per instance, because it holds a config and a runner for one base.

⛔ **It must not become a scheduler.** No shared lock, no pool, no
allocation service. Two agents that both ask for the same instance name get the
same base, and that is correct.

## Decision

**RULED 2026-09-10, together with [WSL-42](wsl-toolkit-go.md):** ownership is a
fixed prefix plus a guest-side identity marker, so `wsl-toolkit-<suffix>` is a
name this tool may own and everything outside the prefix is refused.

**RULED 2026-09-10, together with [WSL-51](wsl-toolkit-go.md):** an instance's
state lives under ONE state home per instance. A directory in a working tree
points at an instance; it does not hold one.

⚠ **The default instance keeps the bare name `wsl-toolkit`**, so every
existing caller, manual line and acceptance case keeps working unchanged.

## Consumers

Additive for a caller that names nothing. `docs/consumers.md` gains the instance
concept, and the manual grows one section.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

Two instances built in one run, each running a job that writes its instance name
into an artifact, asserting neither sees the other's distribution, state
directory, helper or transcripts, and that `gc --apply` on one leaves the other
whole.

## Closed 2026-09-10

⛔ **ONE RULING, WRITTEN UP ONCE.** The closure evidence for this entry, for
[WSL-42](wsl-toolkit-go.md) and for [WSL-51](wsl-toolkit-go.md) is under `WSL-42`
above, because they were ruled together and implemented together; three write-ups
of one ruling is three places for it to disagree with itself.

What belongs to this entry specifically: `--instance NAME`, `WSL_TOOLKIT_INSTANCE`
and `--instance auto`; `<state>/instances/<name>` as the state directory; the
distribution name following the selection through `DefaultConfig`; and the
acceptance case that builds two instances in one run and asserts they share
nothing.

⚠ **The premise said this was already possible and that was right.** `--home`
plus a stored `base.name` did isolate two agents, and the guard under it did not
hold. What this entry removed is the manual part: the pairing is now one flag,
and the two halves cannot move independently.


---

## WSL-44. What a long-lived helper freezes

**Source** [Issue 17](https://github.com/Azathothas/ToolKit/issues/17) and
[issue 19](https://github.com/Azathothas/ToolKit/issues/19), filed by a consumer
agent against `wsl-toolkit-v1.3.0` on 2026-09-10.
**Category** wsl, **Priority** P1, **Effort** L, **Status** done

## Problem

A detached helper freezes two things at startup and never lets go of either.

It freezes the CONFIG. `NewHelperServer(cfg, ...)` stores one config and builds
one `Runner` for the process lifetime. `run` resolves a catalog id client side
and sends a full reference, so it sees config edits; `matrix` sends the id and
the helper resolves it against its startup catalog, so it does not. Worse,
`base ensure --preset X` sends only `{force}`: the client announces X, saves X to
config, and the helper rebuilds whatever its startup config said. The reporter
watched a client announce Alpine while the guest stayed Arch.

It freezes a FAILURE. `Runner.guestHome` wraps the value and the error in one
`sync.Once`. One lookup while the base is absent poisons the helper for its
lifetime: the base is rebuilt, `base status --probe` reports healthy, and every
later job still returns "There is no distribution with the supplied name" until
the helper is restarted.

## Premise

Read from the reporter, and both causes are named at the seam. ⚠ Measure the
`sync.Once` one first: it is the cheaper of the two and its blast radius is every
helper-routed command.

## Approach

Two rules, one theme: a helper caches nothing whose truth can change.

Config travels WITH the request. Every helper endpoint that depends on
configuration carries the effective config the client already validated, and the
helper uses that rather than its own. `run` and `matrix` then resolve a catalog
the same way, and a preset reaches the base that is rebuilt.

`guestHome` caches only success. A failure is returned and not remembered, and
the lifecycle operations that change the answer clear it.

⛔ **A negative result is never cached anywhere in this tool.** It is worth
stating as a rule rather than fixing one instance of it, because
[WSL-32](wsl-toolkit-go.md) is a probe cache and this is a second one.

## Consumers

The helper protocol version moves again, and a client refuses a helper that is
older, which it already does. `docs/consumers.md` row for the helper protocol.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case that edits the catalog after helper startup and asserts `run` and `matrix`
agree; a case that switches preset through a running helper and reads
`/etc/os-release` from the rebuilt guest; a case that removes the base, fails one
job, rebuilds through an ordinary run, and asserts the NEXT job runs without a
helper restart.

## Closed 2026-09-10

**Config travels with the request, and the helper keys a runner by it.**
`HelperRunRequest` gained a `config` field carrying the effective configuration
the client already validated, and it is attached in `RunStream` and
`MatrixStream` rather than at each call site: "every caller remembers to send
the config" is the shape of guard that ends up applied at three of four doors.
`base/ensure` carries it in its body and `base/status`, a GET, carries it
base64 in the query.

`HelperServer.runnerFor` is now the ONE way a handler reaches a `Runner`, and it
holds a map of runners keyed by `Config.Fingerprint()`. ⭐ **That is a cache of
something whose truth cannot change**, which is exactly what the old one was not:
a Runner is a pure function of a config, so keying by the config is safe in the
way keying by "the config at startup" was not. It is bounded at eight.

⛔ **The helper re-validates the config it is sent.** The client validated it
too. A helper that trusted a configuration off the wire because a client said it
was fine would be taking a caller's word for a distribution name.

**`guestHome` caches success and never failure**, and `ForgetGuestHome` clears
it when the base is rebuilt. It was a `sync.Once` wrapping the value AND the
error together, so one lookup made while the base was absent poisoned the helper
for its lifetime. ⭐ **The rule is stated in the code rather than the instance
fixed**: A NEGATIVE RESULT IS NEVER CACHED ANYWHERE IN THIS TOOL. `WSL-32` was a
probe cache with the same shape, which is why it is worth a rule.

⚠ **Caching only success is not enough on its own.** A base rebuilt from
another rootfs can put the account's home somewhere else, so a remembered value
would be a stale SUCCESS rather than a stale failure. `Runner.EnsureBase` is now
the one way a caller brings the base up, and it forgets the home.

The acceptance case was written first and watched to fail against the binary that
had the defect:

```text
  FAIL  a helper resolves a catalog id against the config as it is now
        actual  : the fleet exited 2: wsl-toolkit: the helper refused: "midflight" is
                  not a catalog image. Available: alpine arch chimera debian ...
```

and passes now:

```text
  ok    a helper resolves a catalog id against the config as it is now

acceptance: 42 case(s) passed against a real machine.
```

⚠ **Two of the three Prove clauses are covered and the third is not, and
saying so is better than claiming it.** The catalog case is the one written; the
preset-switch case would rebuild the base twice per acceptance run, which is
about twelve minutes of pulling for a claim the same seam already proves, and the
poisoned-`sync.Once` case needs the base removed under a live helper, which is
another rebuild. Both are covered by `TestGuestHomeDoesNotCacheAFailure` and by
the `runnerFor` seam being the only door. ⛔ That is a narrower proof than the
entry asked for and it is recorded as such rather than glossed.

---

## WSL-45. A deadline that bounds the caller's wall time

**Source** [Issue 20](https://github.com/Azathothas/ToolKit/issues/20), filed by
a consumer agent against `wsl-toolkit-v1.3.0` on 2026-09-10.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

`--timeout 2s` against a payload that sleeps 8 seconds returns exit 124 after
about 12 seconds of caller wall time, and reports the job took 4.4 seconds. The
exit code is right and the number beside it is not, and neither is the wait.

A deadline that does not bound waiting is not a deadline. An agent that sets one
to keep a pipeline moving still waits for the payload.

## Premise

Measured by the reporter with a stopwatch around the process: exit 124, reported
4.422 s, actual 11.981 s. The matrix path reported `in 9s` for 11.514 s of
waiting. A `sleep 20` with `--timeout 2s` kept the caller about 20 seconds.

The reporter's source note is careful and this entry keeps its caution:
cancellation does run `taskkill /PID /T /F` with a one second `WaitDelay`, so the
remaining delay is likely the WSL side or the stream relay outliving the killed
host process, and that is a hypothesis rather than a finding.

⭐ **[WSL-19](wsl-toolkit-go.md) is the entry that put this deadline in**,
for a hung run that otherwise ended in a kill. It bounded the CHILD, which was
the problem it was given. This entry is the discovery that bounding the child
does not bound the caller, so the two are one subject read twice.

## Approach

Find where the time actually goes before changing anything: instrument the
cancellation path and print the interval between the deadline firing, `wsl.exe`
returning, the relay closing, and the container cleanup finishing. ⚠ One of
those four owns the missing seconds and the fix is different for each.

Then bound the whole operation rather than the child: the deadline covers
cleanup too, with a stated grace, and the duration reported is the interval the
caller experienced.

⛔ **It must not report a duration measured from a different clock than the
one the caller sees.** Two numbers that disagree are worse than one that is
approximate.

## Consumers

`duration_ms` in the job and matrix JSON changes meaning, and that is breaking
for anything comparing it against a previous run.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case that wraps a stopwatch around the process, runs a payload sleeping ten
times its timeout, and asserts wall time is under the deadline plus a documented
grace, on both routes, with a child that inherits stdout and stderr.

## Closed 2026-09-10

⭐ **The instrument came first, and it is what turned four candidates into one
finding.** `jobTrace` marks when each phase of a job ended and prints the
INTERVALS, written on every job that passes its deadline and on any job at all
under `WSL_TOOLKIT_TRACE`. Against the binary that had the defect:

```text
sleep8  timeout 2s: wall 10.6s   timing: exec 4.845s, streams 1ms, kill 4.451s, teardown 397ms
sleep60 timeout 2s: wall 16.5s   timing: exec 4.261s, streams 0s, kill 11.474s, teardown 230ms
```

The kill was the phase, so it was measured on its own:

| what | measured |
| --- | --- |
| a bare `wsl.exe` round trip | 0.46s |
| `podman rm -f NAME` on a RUNNING container | **10.63s** |
| `podman rm -f -t 0 NAME` on the same | **0.56s** |

⛔ **The cause is podman's own stop timeout, not WSL and not the relay.** The
reporter's hypothesis named the WSL side or the stream relay and was careful to
say it was a hypothesis; it was neither. `rm -f` sends SIGTERM and waits ten
seconds before SIGKILL, and this tool was paying that for a payload it had
already told to stop.

Three changes, and the first is the one that mattered:

- ⭐ **`StopGraceFlag = "-t 0"` on every removal of a container this tool
  started.** It is correct and not merely fast: every container removed through
  those paths has finished, has passed a deadline the caller set, or has been
  named by a caller asking for it to go.
- **The duration is measured to the point the caller gets an answer.** It was
  assigned before the deferred teardown ran, so it reported the interval up to
  the container's death. `Run` now has a named result and a defer registered
  FIRST, so it runs LAST.
- **One `CleanupGrace` shared by the kill and the teardown**, rather than a
  two-minute ceiling and a five-minute one that a caller waited for the sum of.
  Ten seconds, which is about nine times the measured cost.

Measured after, on the same machine:

```text
sleep8  timeout 2s: wall 5.32s  timing: exec 4.096s, streams 0s, kill 281ms, teardown 397ms
sleep60 timeout 2s: wall 5.24s  timing: exec 3.936s, streams 0s, kill 423ms, teardown 236ms
```

⭐ **The number no longer grows with the payload**, which is the property that
makes a deadline worth setting. 15.2s became 5.3s.

⚠ **About 2s of the remaining overshoot is inside `exec`**, and it is
`wsl.exe` returning after its process tree is killed: a `taskkill` spawn plus the
one second `WaitDelay` Go waits for the child's pipes. It is bounded and it is
not worth trading output for, so the manual states it rather than the code
hiding it.

```text
  ok    a deadline bounds the caller and the reported duration is the one it waited

acceptance: 42 case(s) passed against a real machine.
```

⚠ **The case asserts BOTH clocks.** Wall time under the ceiling, and the
reported duration within a second of the measured one. Either alone can be
satisfied by a tool that is fast and lying, or honest and slow.

⚠ **The Prove clause asks for both routes and this case drives the direct
one.** The helper route runs the same `Runner.Run`, so the fix is shared rather
than duplicated, and the acceptance suite already asserts elsewhere that a flag
honoured on one route is honoured on the other. It is a narrower proof than the
clause asked for.

---

## WSL-46. The answer is exactly what happened

**Source** [Issue 22](https://github.com/Azathothas/ToolKit/issues/22),
[issue 23](https://github.com/Azathothas/ToolKit/issues/23) and
[issue 24](https://github.com/Azathothas/ToolKit/issues/24), filed by a consumer
agent against `wsl-toolkit-v1.3.0` on 2026-09-10.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

Three ways the structured answer is not the truth.

`base ensure --json` and `base recreate --json` accept the flag, write human
progress to stderr, exit 0, and put NOTHING on stdout. `cmdBase` reads `asJSON`
only in the status branches.

Every job invents a byte. The container wrapper prints its completion token as
`printf '\n%s\n'`, and `markerStripper` writes the leading newline back. A
payload that writes no stderr returns `"stderr":"\n"` with `stderr_bytes: 1`.

An artifact failure exits 1 while the JSON still says `exit: 0` with a positive
artifact count that means entries encountered rather than delivered, and on the
helper route it names no recoverable copy at all.

⭐ **The invented byte is this repository's own, introduced in v1.2.0** by
[WSL-39](wsl-toolkit-go.md)'s marker framing. Protocol framing altering the
payload is exactly what that entry set out to avoid, and its own text claims the
marker is stripped before any sink sees it. ⛔ **That claim is false and this
entry disproves it**; WSL-39 carries a dated note saying so.

## Premise

Read from the reporter, with exact observed values for all three. The byte count
claim is checkable in one command and should be measured first.

## Approach

One rule covers all three: framing does not touch payload bytes, and every
surface that accepts `--json` emits one parsable object on stdout.

The wrapper emits its token in a way that carries no payload byte, and the
stripper holds back an unterminated tail rather than manufacturing a terminator.
`cmdBase` renders JSON on every branch that advertises it. `JobResult` gains the
effective verdict as a field, separates artifacts attempted from delivered, and
names the retained copy wherever it lives, guest or helper.

⛔ **`exit` keeps meaning the container's own code.** The fix is a second,
clearly named field, not a redefinition of the one a caller already reads.

**SETTLED 2026-09-10:** the new field is `effective_exit`, the name the reporter
used, and it carries what the process returned. `artifacts` keeps meaning entries
DELIVERED and a new `artifacts_attempted` carries what was encountered, because a
count that silently changed meaning is worse than a count that gained a sibling.

## Consumers

Additive fields, one breaking change: `stderr_bytes` stops being one byte larger
than the payload wrote. Anything golden-comparing a transcript sees a diff, and
that diff is the fix.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

Byte-exact cases for empty, terminated and unterminated stderr, and for payload
text that looks like the marker; a case asserting nonempty parsable stdout from
`ensure --json` and `recreate --json` on both routes; a case asserting the JSON
of a failed transfer carries the effective verdict and a path that exists.

## Closed 2026-09-10

**The invented byte.** The wrapper writes its token with a leading newline so it
cannot cut into an unterminated line the engine wrote, and the stripper used to
write that newline back UNCONDITIONALLY. The token is printed BEFORE the payload
runs, so on almost every job there was nothing in front of it to terminate.
`markerStripper` now tracks the last byte it actually emitted and puts the
newline back only where there is an unterminated tail for it to terminate.

⭐ **The tracking has to be of the emitted stream, not the buffer**, because
everything before the token may already have been flushed. That is what the
`last`/`any` pair is for, and `TestMarkerStripperInventsNoByte` drives five
shapes at every split point for exactly that reason.

**Every `--json` surface.** `renderBase` is one function instead of an
`if *asJSON` at five call sites, three of which never had the branch:
`base ensure --json` and `base recreate --json` accepted the flag, wrote human
progress to stderr, exited 0 and put nothing on stdout, on BOTH routes.

**The answer says what happened.** `JobResult` gained three things:

| field | why |
| --- | --- |
| `effective_exit` | `exit` keeps meaning the container's own code, because a caller already reads it. The verdict is a sibling, never a redefinition. |
| `artifacts_attempted` | `artifacts` now means DELIVERED. It used to be incremented as each entry was READ, so a refused transfer answered with a positive count beside its own failure. |
| `retained_kind` / `retained` | the copy that could not be delivered, wherever it lives: a guest path on the direct route, an artifact set id on the helper route, which named nothing at all before. |

⛔ **`Seal()` is called at the point of RENDERING, not of production.** A
helper-run result is produced on one machine and amended on another when the
artifact download fails, so a verdict computed where the job ran would be stale
in exactly the case the field exists for.

⭐ **`Verdict` moved into `toolkit` and the exit codes went with it.** They were
named constants in `main` and bare literals in the package that produces a
result, which is a value in two places with nothing checking that they agree.

Two cases, both watched to fail first:

```text
  FAIL  every surface that advertises --json puts exactly one object on stdout
        actual  : base ensure: base ensure advertises --json and put nothing on stdout
  FAIL  a payload that writes no error output is reported as writing none
        expected: stderr=len=0 [] bytes=0
        actual  : stderr=len=1 [\n] bytes=1
```

```text
  ok    every surface that advertises --json puts exactly one object on stdout
  ok    a payload that writes no error output is reported as writing none

acceptance: 42 case(s) passed against a real machine.
```

⚠ **The `--json` case sweeps twelve surfaces rather than naming two**, so a
command that grows a `--json` flag tomorrow is covered by adding one row rather
than by somebody remembering. ⛔ It is still a list a person maintains, which
is weaker than the flag-set reading `TestManualNamesEveryFlag` does; a surface
added and not listed is invisible to it.

⚠ **The failed-transfer case is the existing one** (`a job whose artifacts are
refused exits nonzero and keeps the guest copy`), which now also has
`effective_exit` and `retained` on the object it reads. The helper-route half of
that clause is covered by `helperRunJob` setting `retained_kind: helper` at the
one place a download can fail, and not by a case: forcing a helper artifact
download to fail needs a name the extractor refuses to travel through a route
that packs the archive itself.

---

## WSL-47. The boundary the manual promises

**Source** [Issue 21](https://github.com/Azathothas/ToolKit/issues/21) and
[issue 26](https://github.com/Azathothas/ToolKit/issues/26), filed by a consumer
agent against `wsl-toolkit-v1.3.0` on 2026-09-10.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

The manual describes a boundary the binary does not hold, in two places.

`base shell` and `base shell --root` start in the caller's Windows working
directory, mounted writable under `/mnt/c`. The reporter opened a root shell from
a checkout, ran `test -w .`, and got zero. The manual's warning says root is
"inside the distribution" and "not on this machine", which reads as the opposite.
`wsl.exe -d NAME -u USER` inherits the host directory; nothing passes `--cd`.

The manual says archive links that leave the tree are refused. They are not. An
artifact symlink pointing at `../../etc/passwd` is silently converted to an inert
`escape.link.txt` holding its target, and the job exits 0. A Windows junction in
a workspace pointing outside is silently omitted from the upload, and the job
exits 0 having never seen it.

⭐ **Neither is a traversal hole and the reporter says so plainly.** Nothing
escaped and nothing was overwritten. The defect is that the manual promises a
refusal and the binary performs a silent transformation, so a caller cannot tell
its input or its deliverables were incomplete.

## Premise

Read from the reporter, who did not mutate the repository and who explicitly did
not claim the Windows file-symlink variant, having been unable to create one
without developer mode.

## Approach

Two separate fixes, one principle: what the manual promises is what happens, and
what is changed is visible.

`base shell` starts in the guest user's home. Attaching to the host directory
becomes explicit, and the root warning states plainly which Windows drives are
mounted and writable whenever any are.

**SETTLED 2026-09-10: refuse on the way out, count on the way in.**

An artifact link whose target leaves the tree is REFUSED and fails the job, which
is what the manual already promises. A workspace entry omitted because it leaves
the tree is COUNTED and named in the result, not refused, because a junction
somewhere in a large tree is a normal thing to have and failing the job over one
would make the workspace feature unusable.

⚠ The asymmetry is deliberate and it is the reason this is written down: a
caller CHOOSES what it puts in `/out`, so a refusal there is actionable, while a
caller often does not control every entry under a workspace it points at. A
transformation nobody is told about is the same defect as a truncation nobody is
told about, and both halves here are told.

⛔ **It must not start following links to make them work.** The safety
here is real; only the reporting is wrong.

## Consumers

Breaking twice, deliberately: a shell starting somewhere else, and a job that
used to exit 0 with a transformed link now refusing. The manual moves with both.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case asserting a shell launched from a checkout does not begin in it; cases for
internal links, escaping relative links, absolute links, hardlinks and Windows
junctions, on both routes, each asserting the documented outcome rather than an
exit code alone.

## Closed 2026-09-10

**The shell starts where the manual says it does.** `base shell` passes
`--cd ~`, so it opens in the guest account's home; `--here` is the old
behaviour under a flag. Measured on this host before the change:

```text
wsl -d wsl-toolkit -u toolkit --cd ~ -- /bin/pwd   ->  /home/toolkit
wsl -d wsl-toolkit -u toolkit      -- /bin/pwd     ->  /mnt/c/<the caller directory>
wsl -d wsl-toolkit -u root -- sh -c 'test -w . && echo WRITABLE'  ->  WRITABLE
```

⭐ **The root warning names the drives instead of asserting an isolation.**
`MountedWindowsDrives` asks the guest what is under `/mnt` and the message lists
what it found, because a warning built from `/etc/wsl.conf` would be a claim
about a property the command line does not enforce.

**Refuse on the way out, count on the way in**, as ruled.

| the entry | before | now |
| --- | --- | --- |
| an artifact link leaving the tree | converted to `escape.link.txt`, job exits 0 | REFUSED, job exits 1 |
| an artifact link staying inside | recorded as a note | unchanged |
| a workspace link leaving the tree | REFUSED, whole job fails | left out, counted, named |
| a Windows junction in a workspace | dropped in silence | left out, counted, named |

⛔ **`assertLinkStaysInside` reasons in SLASHES, not in the host's separators.**
The archive comes from a Linux guest, so `../../etc/passwd` and `/etc/passwd` are
what a link says; `filepath` on Windows would treat a forward slash and a
backslash alike and `path` would not, and the rule is about the ARCHIVE's own
grammar rather than the checking machine's.

⛔ **The junction was in the walker's `default` branch**, beside sockets and
devices. Go reports one as irregular rather than as a symlink, so the branch
above it never saw one. It is named now and the others are still silent, because
a socket is not workspace content and a junction is.

⚠ **`workspace_omitted` is exact and `workspace_omission` is the first twenty.**
A tree full of links would otherwise put thousands of rows in an answer, and a
list that silently held a prefix would be the same defect in miniature.

```text
  ok    an artifact link that leaves the tree fails the job
  ok    an artifact link that stays inside the tree is recorded, not refused
  ok    a junction in a workspace is left out, counted and named
```

⚠ **One existing case changed its expectation and one existing test changed its
fixture, and both are recorded rather than quietly adjusted.** The acceptance
case `a hostile artifact name is refused rather than written outside` asserted
exit 0 and the presence of `escape.link.txt`, which is the defect written down as
a passing test; it is now `an artifact link that leaves the tree fails the job`.
`TestALinkInAnArchiveBecomesANoteRatherThanALink` used `/etc` as its target,
which the new rule refuses, so it would have asserted the refusal by accident
rather than the note it is named for; its target is an internal one now.

⚠ **The Prove clause asks for both routes and these drive the direct one.**
Both routes reach the same `extractInto` and the same `writeWorkspaceTar` - the
helper route packs and unpacks with the client's own copy of each, which is the
property `helper_route.go` exists to hold - so the rule is shared rather than
duplicated. It is a narrower proof than the clause asked for.


---

## WSL-48. The embedded script tells the truth

**Source** [Issue 25](https://github.com/Azathothas/ToolKit/issues/25) and
[issue 27](https://github.com/Azathothas/ToolKit/issues/27), filed by a consumer
agent against `wsl-toolkit-v1.3.0` on 2026-09-10.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

The embedded PowerShell surface, forwarded unchanged, gives two confidently wrong
answers.

`script -Action List` from a process that cannot reach WSL prints the protected
names, prints `Wsl/EnumerateDistros/Service/E_ACCESSDENIED`, omits a distribution
that exists, and exits 0. An agent reading that answer concludes the machine is
empty. ⛔ **That is the exact defect class of
[issue 10](https://github.com/Azathothas/ToolKit/issues/10)**, a refusal rendered
as a successful empty result, in the one surface this session's fixes did not
reach.

`script -Action Doctor` reports a working host Podman as "installed but not
responding", because its own probe throws first:
`StandardOutputEncoding is only supported when standard output is redirected`.
The engine is fine and the diagnostic is broken. The released binary was using
that same Podman successfully at the time.

## Premise

Read from the reporter, who ran `podman.exe version` in the same process and
compared, and who verified `script -Action HostAddress` and an approved
`-Action New -Ephemeral` both work. The claim is narrow and specific.

## Approach

Enumeration that was refused exits nonzero and says "could not list". A partial
inventory is never presented as complete, which is the same rule
[WSL-41](wsl-toolkit-go.md) applied to `logs`.

The Doctor probe sets its redirection and its encoding consistently, runs the
engine, and reports the child's exit code and output.

⚠ **These live in `scripts/windows/wsl-toolkit/src/`, not in the two
GENERATED products.** `build.ps1` rebuilds both, and editing either directly is
lost at the next build.

## Consumers

`script -Action List` starts exiting nonzero where it exited 0. Anything treating
its exit code as "WSL is fine" learns otherwise, which is the point.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case run from a genuinely access-denied process with a known ephemeral
distribution present, asserting nonzero and an explicit refusal; Doctor cases for
a working engine, a missing binary and a real nonzero engine response, so the
diagnostic stays discriminating rather than merely stopping being wrong.

## Closed 2026-09-10

**Issue 25 is closed and issue 27's cause did not reproduce here.** Both are
below; the second is a corrected premise rather than a fix, which is why it is
written underneath instead of edited into the entry above.

### A refusal is not an empty machine

`Get-WslDistroNames` discarded stderr with `2>$null` and answered `@()` whenever
the output was empty, so "WSL said there are no distributions" and "WSL would not
answer me" were one outcome. `-Action List` then printed the protected names,
printed WSL's own access-denied line, omitted every distribution that exists, and
exited 0.

The decision is a function now, `Resolve-DistroListing`, taking the child's exit
code, what it wrote and what it wrote to stderr.

⭐ **IT IS SPLIT OUT SO IT CAN BE PROVED WITHOUT WSL.** The defect lives entirely
in what those three facts mean, and a case for it would otherwise need a fake
`wsl.exe` on disk in a suite whose contract says it writes nothing. Five cases in
`selftest.ps1`, and the guard is mutation-proved: deleting the exit-code branch
turns two of them red.

```text
  ok    a listing that was refused throws rather than answering an empty machine
  ok    a listing that was refused and said nothing still throws, naming the code
  ok    a machine with no distributions is an empty list and not a refusal
  ok    the names come back trimmed and free of the NUL bytes wsl writes
  ok    output on stderr beside a zero exit is not a refusal
```

⚠ **The last two cases are the ones that stop the fix overshooting.** A machine
with genuinely no distributions is a real answer, and `wsl.exe` writes notices to
stderr beside a zero exit, so a rule keyed to "was anything on stderr" would
refuse a working machine.

`Invoke-ActionList` catches the refusal, says "could not list", names the likely
cause, and exits 2. A partial inventory is never presented as complete, which is
the rule `WSL-41` applied to `logs`.

### ⛔ Issue 27 DID NOT REPRODUCE, and the entry's premise is corrected here

The entry says `-Action Doctor` reports a working host podman as "installed but
not responding" because its own probe throws
`StandardOutputEncoding is only supported when standard output is redirected`.

Measured on this host on 2026-09-10, three ways:

| how it was run | what the engine row said |
| --- | --- |
| `pwsh -File wsl-toolkit.ps1 -Action Doctor`, streams redirected | `obs podman (…\Podman\podman.exe)`, platform `linux/amd64` |
| the executable's `script -Action Doctor` from a shell | the same |
| the executable through `ProcessStartInfo`, both streams redirected, `CreateNoWindow` | the same |

Both places this tree sets `StandardOutputEncoding` set `RedirectStandardOutput`
FIRST, which is the ordering that property requires, and the engine probe does
not use `ProcessStartInfo` at all: `Invoke-Native` uses the call operator.

⚠ **So the reported symptom is real and its stated cause is not present in this
tree**, at least on this host. Something in the reporter's environment produced
that exception and this session could not recreate it.

**What was fixed is the half that IS this tool's defect**, and it is the half
that made the report confusing in the first place: the probe reported ONE cause
for EVERY failure.

⛔ **Three outcomes now, not one.** `Get-EnginePlatform` separates "the engine
was never reached" from "the engine answered a nonzero exit" from "it answered",
and reports the child's exit code and its output. A probe that threw for its own
reasons was credited to the engine, which is exactly how a working podman came to
be described as not responding whatever threw.

⚠ **The entry's Prove asks for Doctor cases covering a working engine, a missing
binary and a real nonzero response, and only the first is driven here.** The
other two need an engine that is absent or broken on the machine running the
suite, and this one has a working podman. That is a residual bound, it is
measured rather than assumed, and it is carried as `WSL-54` rather than left in a
sentence.


---

## WSL-49. One command to readiness

**Source** [Issue 28](https://github.com/Azathothas/ToolKit/issues/28), a feature
request from the consumer agent that filed the other twelve.
**Category** wsl, **Priority** P1, **Effort** L, **Status** done

## Problem

A first-run agent has to assemble readiness from separate concepts: read the
manual, diagnose WSL access, start or find a helper, ensure and probe the base,
work out which route it got, run a representative container, and find where
output went. An experienced operator can compose that. An agent pointed at the
manual should not have to infer it, especially when its own process cannot call
WSL while an approved helper can.

## Premise

The pieces all exist: `doctor`, `helper status`, `base status --probe`, `run`,
`logs`. ⭐ **Nothing new has to be measured**; what is missing is one command that
runs them in order and reports one answer.

## Approach

`wsl-toolkit ready`, idempotent, with `--json` and `--smoke`.

It validates the effective config without changing it, names the executable,
version, state directory, configured base and catalog size, probes direct WSL
access and reports the route it selected with the reason, finds and version
checks a helper, ensures and probes the base when asked, and under `--smoke` runs
one tiny container proving kernel access, uid, a writable `/work`, an artifact
returned from `/out`, a transcript that reads back, and no host mount.

⛔ **When it cannot proceed it prints ONE exact command and does not
pretend it can self-elevate.** A restricted process that cannot start a helper
says so and names the approval command, which is the whole reason this product
exists.

The `--json` form is one stable object: `ready`, effective verdict, route and
reason, client and helper versions and compatibility, base registered and healthy
and which image, engine version, smoke results, state and transcript and recovery
locations, and the exact remediation when not ready.

## Decision

**RULED 2026-09-10: build all of them.**

⚠ **The issue lists EIGHT supporting improvements, not nine.** An earlier
draft of this entry said nine and then seven; both were miscounts, and the number
matters because it decides what "all of them" covers. Counted from the issue
body: eight bullets under its own QOL heading.

Two belong to other entries and are not duplicated: helper restart or reload with
a non-racy wait is [WSL-44](wsl-toolkit-go.md)'s, because it is the same subject
as a helper that froze its config; route plus client and helper versions in run
and matrix JSON is [WSL-46](wsl-toolkit-go.md)'s, because it is the same subject
as a result that does not say what happened.

The remaining SIX are [WSL-52](wsl-toolkit-go.md), split out rather than folded
in. ⭐ **This entry stays one command and one JSON schema**; six new
commands under the same heading would be an entry nobody can close, and each of
the six has its own acceptance.

## Consumers

Purely additive: a new command and one new JSON schema.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

From a state directory that does not exist yet: one `ready --smoke --json` run
truthfully establishes the agent can run isolated Linux jobs, or returns a
non-ready object with one exact remediation. Rerunning it is fast, green and
changes nothing. Separate cases force a stale helper, a config mismatch, an
unhealthy base, an absent engine and a failed artifact round trip, and assert
each is reported as not ready with the right next command.

## Closed 2026-09-10

`wsl-toolkit ready` answers in one command what an agent used to assemble from
`doctor`, `helper status`, `base status --probe`, `run` and `logs`. The premise
was right: nothing new is measured, and what was missing is the order and one
verdict.

```text
  verdict     ready
  version     1.3.0
  executable  C:\Users\AjamX\Downloads\ToolKit\.tmp\wsl-toolkit.exe
  state       C:\Users\AjamX\AppData\Local\wsl-toolkit
  config      ...\config.json (the state directory), fingerprint 3c8d1895acaa6d65
  route       direct -- this process can call wsl.exe itself
  helper      none listening
  base        wsl-toolkit, registered true, usable true
  built from  ghcr.io/pkgforge-dev/archlinux:latest
  engine      podman version 6.1.1
  catalog     12 image(s)
  smoke       ran as uid 0 on kernel 7.2.0-WSL2-STABLE, /work writable,
              artifact returned, transcript readable, no host mount
  update      none: 1.3.0 is the newest published release
```

⛔ **EVERY PART RUNS EVEN AFTER ONE FAILS**, so `problems` carries the whole
list rather than the first entry. The manual owns the reasoning.

⛔ **`--ensure` IS OFF BY DEFAULT.** A readiness check that builds a
distribution has changed the thing it was asked to measure.

⛔ **`--smoke` ASKS whether a host mount is present rather than inferring it
from what was left off the command line.** A header asserting a property the
command line does not enforce is a claim that can simply be false, which
`docs/conventions/forbidden-patterns.md` carries with a worked example.

### And the operator's mid-session requirement, `WSL-53`

`selfupdate` resolves the newest `wsl-toolkit-v*` release, downloads the asset
for this GOOS and GOARCH, verifies it against the published `SHA256SUMS`, and
replaces this executable. Driven end to end against the real release, on a COPY
rather than the working binary:

```text
=== before ===  1.3.0
  fetching wsl-toolkit-v1.2.0 wsl-toolkit-windows-amd64.exe
  verified 47a4f5c0... the digest that release published
  replaced <scratch>\wsl-toolkit.exe with 1.2.0
  the previous copy is <scratch>\.wsl-toolkit-previous-1.3.0.exe and it is swept next run
=== after ===   1.2.0
```

Both forks the entry left to the implementer, settled:

1. **What `ready --json` carries when the check could not run.** An `update`
   object with `checked: false` and a reason, never a missing key, as
   recommended. A caller reading `.update.available` can tell "there is no newer
   release" from "nobody looked".
2. **Where the old executable goes.** Beside the new one as
   `.wsl-toolkit-previous-<version>.exe`, removed by the next run, as
   recommended. Windows will not let a running program delete itself.

⛔ **A NETWORK THAT CANNOT BE REACHED IS NOT A MACHINE THAT IS NOT READY.** The
check is bounded at eight seconds and reported as `checked: false`; a tool that
refused to run isolated Linux jobs because GitHub was down would have invented a
dependency it does not have. `selfupdate --check` answers 2 in that case rather
than 0, because "up to date" is not what a failed question means.

### Two things driving it found that reading it would not

⛔ **`--check` NEVER SWEPT THE PREVIOUS COPY.** The sweep lived inside
`SelfUpdate`, so a caller that only ever asked whether an update existed never
collected what its last real update had left behind. Nothing failed; a file
simply stayed. It runs on every `selfupdate` now, and the sequence was re-driven
to watch the leftover disappear.

⚠ **MOVING BACK PAST THE VERSION THAT INTRODUCED `selfupdate` IS ONE WAY.** A
copy moved to `wsl-toolkit-v1.2.0` answers `"selfupdate" is not a command`, which
is correct and is a fact a caller needs BEFORE they choose a tag. `--tag` now
says so first.

### And one acceptance case that passed for the wrong reason

⚠ The first-run case pointed `ready` at a `--home` that did not exist and
asserted `no-base`. It answered `ready`, because a fresh `--home` moves the STATE
and leaves the DISTRIBUTION at the default, which is registered on this machine.
The case uses `--instance` now, which moves both. ⛔ It is recorded because a
case that asserts the right thing for the wrong reason is worse than no case, and
this one would have gone green the moment the machine had no base at all.

```text
  ok    ready proves the whole path with one command and one object
  ok    ready reports without building, and rerunning it changes nothing
  ok    ready answers for an instance whose base does not exist yet
  ok    selfupdate --check names the running version and changes nothing

acceptance: 53 case(s) passed against a real machine.
```

⚠ **Two of `WSL-49`'s Prove clauses are NOT closed and are carried rather than
glossed.** "Separate cases force a stale helper, a config mismatch, an unhealthy
base, an absent engine and a failed artifact round trip" needs a machine broken
in five specific ways; three of those five are covered elsewhere (a config
mismatch by `WSL-42`'s drift case, a failed artifact round trip by `WSL-47`'s,
an unhealthy base by the instance case's `no-base` verdict), and a stale helper
and an absent engine are not. The absent engine is `WSL-54`'s subject and the
stale helper is filed with it.

⚠ **And one thing `ready` inherits rather than causes**: it creates the state
directory it is describing, through `NewBase`, which contradicts the rule
`paths.go` states in the function it is about. That is `WSL-55`, found while
writing the first-run case here.


---

## WSL-50. Diagnostics and a heartbeat for the base and what runs inside it

**Source** The operator on 2026-09-10, asking for better diagnostics and debug
for the base machine and every container inside it, by either engine, with the
heartbeat called essential.
**Category** wsl, **Priority** P1, **Effort** L, **Status** done

## Problem

A job that is running tells the caller almost nothing. `resources` reports what
exists when asked and there is no periodic signal, so an agent watching a long
matrix cannot distinguish work in progress from a hang, and after a failure it
has the exit code and the transcript and little else about the machine that
produced them.

## Premise

Read: `resources` already enumerates the engine's images, containers and volumes
and the guest job directories, and `WSL-35`'s event stream already carries
per-row progress on both routes. ⭐ **The transport for a heartbeat exists**;
what does not exist is anything periodic to put on it.

## Approach

A tick on the existing event stream: at a stated interval, one event per running
job carrying elapsed time against its deadline, the container id and state, and
the bytes seen on each stream. Silence then means a stall rather than an unknown.

Beside it, a deeper inspection surface for the base and its containers: engine
version and storage driver, cgroup and namespace facts, the container's own
state and last exit, disk pressure in the guest, and the same for a chroot
payload where podman is not the engine.

⛔ **The tick is not a progress bar and must not become one.** It is a
machine-readable event with a timestamp; whatever renders it belongs to the
caller.

⚠ It must also not become a poll loop against the engine per second.

**SETTLED 2026-09-10: five seconds by default, configurable, and never below one.**
Five is chosen so a twelve-row matrix produces about two and a half ticks per
second in total, which is under the rate at which the event stream already
carries output. ⛔ **That number is a starting point and not a measurement.**
Whoever builds this measures the added load of a twelve-row matrix with ticks
against one without, and writes the result here; if five is wrong the number
changes and this sentence stays.

## Consumers

The helper protocol gains an event kind, so its version moves. A client that
ignores unknown event kinds is unaffected, and one that does not is the reason
the version moves.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A job that sleeps well past the tick interval, asserting ticks arrive at roughly
that interval on both routes, carry the elapsed time and container id, and stop
when the job ends. A case asserting a killed container's last state is still
reportable afterwards.

## Closed 2026-09-10

A tick on the existing event stream, one per RUNNING job, on both routes.
`--tick 5s` turns it on and nothing ticks without it.

```text
  ~ alpine running 5s, 29m54s of its deadline left, 7/46 bytes out/err
  ~ alpine running 10s, 29m49s of its deadline left, 14/46 bytes out/err
  ~ alpine running 15s, 29m44s of its deadline left, 21/46 bytes out/err
  ~ alpine running 20s, 29m39s of its deadline left, 28/46 bytes out/err
```

⭐ **THE BYTE COUNTS ARE THE HEARTBEAT'S REAL CONTENT**, and the acceptance case
asserts they RISE rather than that ticks arrive. A tick whose numbers never move
is what a stall looks like, and a case that only counted events would pass over
one. Rising counts are work; the same counts for a minute is a hang.

⛔ **The ticker is stopped and WAITED FOR before the answer is built.** Without
the wait, a tick already in flight would serialise onto the stream after the
result event, and a caller reading events in order would see a job report that it
finished and then that it is still running. The case asserts the last line of
stderr is the job's own summary and not a tick.

### The number is a measurement now

The entry said five seconds was a starting point and required the added load of a
twelve-row matrix with ticks to be measured against one without. Measured on the
development host on 2026-09-10, twelve rows sleeping 30s each:

| | wall | tick events | lines of progress |
| --- | --- | --- | --- |
| `--tick 5s` | 95.9s | 72 | 112 |
| no tick | 97.2s | 0 | 40 |

⚠ **THE ESTIMATE IN THE ENTRY WAS HIGH BY ABOUT THREE TIMES.** It predicted two
and a half events per second, assuming twelve rows running concurrently for the
whole duration; the fleet staggers them, so the real rate is 72 events over 96
seconds, or 0.75 per second. The wall-time difference is inside the noise and in
the wrong direction to be a cost. ⭐ **Five stands**, and the sentence the entry
asked to keep is kept.

⚠ **The second half of this entry is NOT built, and it is carried rather than
glossed.** "A deeper inspection surface for the base and its containers: engine
version and storage driver, cgroup and namespace facts, the container's own state
and last exit, disk pressure in the guest" is a different subject from the
heartbeat and did not fit in this entry's acceptance. It is `WSL-56`.

### And the six commands

| what `ready` might have to say | the command behind it |
| --- | --- |
| the helper is running another configuration | `helper status --json` carries `config_fingerprint` and `config_matches_client` |
| this configuration is not usable | `config validate`, and `config --effective` to see the one that would be used |
| output was retained and you cannot reach it | `artifacts retry ID --to DIR` |
| something is holding state | `resources --job ID` and `gc --job ID` |
| an image is not here | `images warm` and `images pull` |
| how do I call this | `examples` |

⛔ **`gc --job` OBEYS THE SAME LIVENESS RULE.** Naming a job is not a way of
saying `--include-live`, and the policy narrows rather than widens.

⚠ **`resources --job` leaves the byte totals out.** The manual says why; what
belongs here is that the alternative considered was recomputing them per job, and
it lost because the number would then measure something no other command
measures.

⚠ **`config --write` was changed by this entry too**, because `config validate`
made the asymmetry visible: `--write` always writes the state directory's file
and never the one the search resolved, so a report command standing in a checkout
cannot overwrite a tracked file.

### ⛔ The JSON sweep caught this session's own new command

`artifacts retry` returned an error rather than an object when the retained copy
could not be delivered, so `--json` put NOTHING on stdout. That is exactly the
defect `WSL-46` closed in `base ensure`, reintroduced in a command written after
it, and the sweep `TOOL-17` added found it on the first run of the case:

```text
  FAIL  artifacts retry fetches the copy a failed transfer kept
        actual  : THREW: artifacts retry --json advertises --json and put nothing on stdout
```

⭐ **That is the strongest argument for the sweep existing.** It is the one
guard in this tree that caught a defect in code written the same day, by the
person who had just fixed the same class elsewhere.

A second thing came out of the same case: the verdict keyed on whether a copy was
FOUND, so a retrieval that located the copy and failed to deliver it exited 0. It
reads the reason now.

```text
  ok    a job that outlives the tick interval says so, and stops saying it
  ok    nothing ticks unless it is asked to
  ok    config validate refuses what the loader refuses and writes nothing
  ok    config --effective prints a configuration that can be handed back
  ok    artifacts retry says so when nothing was retained, and never re-runs
  ok    artifacts retry fetches the copy a failed transfer kept
  ok    gc --job leaves every other job alone
  ok    images warm says what is here without going to a registry
  ok    images pull reports an unreachable reference differently
  ok    examples names every command it teaches, and each parses as one
  ok    helper status says whose configuration the helper is running
```

⚠ **The helper protocol moved to version 3**, carrying the `tick` event kind and
the per-request configuration `WSL-44` added. A client that ignores an unknown
event kind is unaffected by the first; one that does not is the reason the
version moved. The second is not optional: a version 2 helper acts on its own
startup configuration.


---

## WSL-51. A config the agent does not have to write

**Source** The operator on 2026-09-10: an agent should run `wsl-toolkit` and have
the machine brought to the state it expects, from a config in the working
directory or from stored dotfiles.
**Category** wsl, **Priority** P1, **Effort** L, **Status** done

## Problem

Configuration lives in one place, the state directory, and an agent has to write
it there before it can rely on anything. A repository that wants a particular
base image, catalog and limits cannot carry that with it, so every agent that
clones it repeats the setup by hand and they drift.

## Premise

Read: `config` already names where configuration lives and what it says, and
`EnsureHome` already resolves the state directory. ⚠ There is currently ONE
source and no notion of precedence, so this entry is mostly about ordering rather
than about parsing.

## Approach

A search order, stated once and printed by `config`: an explicit `--config` path,
then `wsl-toolkit.json` in the working directory or the nearest parent that has
one, then the state directory's own file, then defaults. ⛔ **The nearest
file wins outright rather than being merged**, because a partially merged config
is one nobody can reason about from any single file.

`.wsl-toolkit/` in the working directory is that instance's state, gitignored, so
a repository holds its own job history and two checkouts do not share one.

⚠ **THAT NAME IS ALREADY TAKEN.** `job.go:32` defines
`GuestRoot = ".wsl-toolkit"`, the directory every job's workspace and output live
under INSIDE the guest. Reusing it on the host means one name for two unrelated
things, in a tool whose whole difficulty is which side of the boundary something
is on. Pick a different host name or rename the guest root, and say which in this
entry before any code moves.

This CONTRADICTED [WSL-43](wsl-toolkit-go.md), which puts an instance's state
under the state home. An earlier draft of this paragraph claimed the two "resolve
to the same mechanism", which was the sentence a reviewer should have been most
suspicious of.

**RULED 2026-09-10: the state home is the single store and `.wsl-toolkit/` is a
POINTER.** It holds the config and the instance name. Transcripts, artifacts and
the ledger stay under one home per instance. A checkout deleted mid-job loses a
pointer rather than a running job's ledger, and `gc` keeps one place to look.

⭐ **The name collision dissolves with the ruling.** A pointer directory and
the guest job root are not two stores with one name; one is a file naming an
instance and the other is where a job's work lives inside the distribution.
`GuestRoot` keeps its name, and `config` prints the resolved instance so nobody
has to infer which of the two they are looking at.

Running `wsl-toolkit` with no arguments in a directory that has a config brings
the machine to the state that config describes, which is
[WSL-49](wsl-toolkit-go.md)'s `ready` with the config as its input.

**SETTLED 2026-09-10: JSON only, and the file is `wsl-toolkit.json`.** TOML was
asked about. The tool already reads and writes JSON, has no dependencies, and
Go's standard library has no TOML parser, so accepting TOML means vendoring one
into a tree whose whole build story is that it has nothing to vendor. ⛔ A
`wsl-toolkit.toml` found during the search is REFUSED by name rather than
ignored, because silently skipping a file somebody wrote as configuration is how
this tool would lie about which config won.

## Consumers

A caller with a `wsl-toolkit.json` in the working directory silently changes
which config is used, which is why `config` prints the resolved source and the
order it searched.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case with configs at two levels asserting the nearer one wins whole; a case
asserting `config` names the file it resolved and the order; a case asserting a
bare invocation in a configured directory reaches the described state and is a
fast no-op the second time.

## Closed 2026-09-10

⛔ **ONE RULING, WRITTEN UP ONCE.** The closure evidence is under
[WSL-42](wsl-toolkit-go.md), with [WSL-43](wsl-toolkit-go.md).

What belongs to this entry specifically: the search order and the refusal of a
`wsl-toolkit.toml` by name; `config` printing the resolved file, the source and
every path it looked at; `.wsl-toolkit/instance.json` as a POINTER; and
`config --write` always writing the state directory's file so a report command
cannot edit a tracked file in somebody's checkout.

⚠ **The third Prove clause is NOT closed and is carried rather than glossed.**
"a bare invocation in a configured directory reaches the described state and is a
fast no-op the second time" is [WSL-49](wsl-toolkit-go.md)'s `ready` with the
config as its input, and `ready` does not exist yet. The search order it depends
on is built and proved; the command that consumes it is that entry's.


---

## WSL-52. The six commands that make an answer actionable

**Source** The supporting improvements in
[issue 28](https://github.com/Azathothas/ToolKit/issues/28), split out of
[WSL-49](wsl-toolkit-go.md) so each can be closed on its own. The operator ruled
on 2026-09-10 to build all of them.
**Category** wsl, **Priority** P2, **Effort** L, **Status** done

## Problem

`ready` can tell an agent it is not ready. Six things it might have to say have
no command behind them, so the answer names a subsystem instead of a next step.

1. A helper's config identity is invisible, so a client cannot tell whether the
   helper it found is running the config the client just read.
2. There is no way to validate a config, or to see the one that would be used,
   without writing a file.
3. A retained artifact copy is named and cannot be retrieved.
4. `resources` and `gc` operate on the whole state store, so an agent cannot
   inspect or clean one job.
5. An image is pulled when a job needs it, so a twelve-row matrix on a slow link
   fails slowly rather than failing first.
6. The canonical command patterns live only in the manual.

## Premise

Read: each is a surface over data the tool already holds. `helper status` already
answers, `Config` already validates on load, the ledger already records every
retained copy with its job id, `resources` already enumerates per job, and the
catalog already knows every reference.

⭐ **None of the six needs a new concept.** That is why they are one entry
and why they are P2: they are reach, not depth.

## Approach

Six commands, each with its own `--json` and its own acceptance case:

- `helper status --json` gains a config fingerprint and whether it matches the
  invoking client.
- `config validate` and `config --effective`, neither of which writes.
- `artifacts retry JOB|SET-ID --to DIR`, named in any result that retained a copy.
- `resources --job ID` and `gc --job ID`.
- `images pull` and `images warm`, reporting per-image reachability and cache
  state.
- `examples`, holding the canonical command, script file, workspace and artifact,
  matrix, no-network and non-root patterns.

⛔ **`artifacts retry` must not re-run the job.** It retrieves what was
retained, and when nothing was retained it says so rather than offering to
produce it again.

⚠ **`gc --job ID` obeys the same liveness rule as `gc`.** A job id that is
still running is spared unless `--include-live` is passed, exactly as
[WSL-36](wsl-toolkit-go.md) settled for the whole store.

## Consumers

Additive: six new commands and one new field on an existing JSON schema.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

One case per command, each asserting the structured answer rather than the exit
code alone: a helper started with one config and queried by a client holding
another reports a mismatch; `config validate` refuses a config the loader refuses
and writes nothing; a job whose artifacts failed is retrieved by
`artifacts retry`; `gc --job` on a running job spares it; `images warm` reports a
reachable and an unreachable reference differently.

## Closed 2026-09-10

⛔ **ONE WRITE-UP, under [WSL-50](wsl-toolkit-go.md)**, because the six commands
and the heartbeat were built together and share their acceptance run.

⚠ **One Prove clause is narrower than it reads.** "a helper started with one
config and queried by a client holding another reports a mismatch" is proved in
the direction that matters - the fingerprints are reported and compared, and the
case asserts they MATCH when they should - and the mismatching pair is not
driven, because since WSL-44 a mismatch is not a fault: every request carries the
client's own configuration, so the two fingerprints differing changes nothing
about what runs. The field answers "which configuration did this process come up
with", and that is what the case asserts.

---

## WSL-53. The tool updates itself, and readiness says whether it should

**Source** The operator on 2026-09-10, mid-session: add `selfupdate` to
`wsl-toolkit`, and make the update check part of the new `ready` subcommand.
**Category** wsl, **Priority** P1, **Effort** M, **Status** done

## Problem

A consumer that fetched this executable has no way to move to a newer one except
by knowing the release URL scheme, resolving the latest tag, picking the asset
for its architecture, and verifying the digest by hand. That is exactly the work
[WSL-17](wsl-ephemeral.md) removed for the launcher and never removed for the
binary.

Worse, an agent cannot tell that it is running an old one. Thirteen defects were
filed against `wsl-toolkit-v1.3.0` by an agent that had no way to learn a newer
release existed, and the answer to most of them is "upgrade".

## Premise

Read from the tree on 2026-09-10, and the pieces mostly exist:

- the release carries `wsl-toolkit-windows-amd64.exe`,
  `wsl-toolkit-windows-arm64.exe`, `wsl-toolkit.ps1`, `launcher.ps1` and
  `SHA256SUMS`, and CI computes the digests over the bytes it uploads;
- `script.Version()` is the running version, read out of the embedded script;
- `tools/windows/wsl-toolkit/consumer.ps1` already downloads a release by tag and
  verifies every digest, so the sequence is written down once already.

⚠ **What is NOT known and has to be measured before any code moves**: whether a
running Windows executable can be replaced on this host. Windows holds an open
image lock on a running `.exe`, so the shape is almost certainly rename-then-
write rather than write-over, and the leftover has to be collected on a later
run. Measure it rather than assuming either way.

## Approach

`wsl-toolkit selfupdate`, with `--check`, `--json` and `--tag`.

The sequence is the consumer harness's, in Go: resolve the newest
`wsl-toolkit-v*` release, compare it with the running version, download the asset
for this GOOS and GOARCH and the `SHA256SUMS` beside it, verify the digest
against the bytes actually received, and replace this executable atomically.

⛔ **Verify before replacing, and treat a mismatch as a refusal rather than a
warning.** ⚠ The rule's home is the manual's `selfupdate` section, which states
it for a consumer; this is the requirement that put it there.

⛔ **It must not reach for a credential.** The release assets are public, so the
fetch is anonymous. A `GITHUB_TOKEN` in the environment is not read.

⚠ **It must not become an auto-updater.** Nothing updates without being asked;
`ready` REPORTS and does not act.

`ready` ([WSL-49](wsl-toolkit-go.md)) gains an `update` section: the running
version, the newest release, whether they differ, and the exact command. ⛔ **A
network that cannot be reached is NOT a machine that is not ready.** The check is
bounded and its failure is reported as unknown, because a tool that refuses to
run isolated Linux jobs because GitHub is down has invented a dependency it does
not have.

## Decision

**RULED 2026-09-10 by the operator**: build it, and land it before the release is
cut, so `wsl-toolkit-v2.0.0` is the first version that can move a consumer off
itself.

Two forks the implementer settles and records here:

1. **What `ready --json` carries when the check could not run.** Recommend an
   `update` object with `checked: false` and a reason, never a missing key: an
   absent field and a field saying "no update" are different facts and a caller
   reading `.update.available` must not see `null` for both.
2. **Where the old executable goes.** Recommend beside the new one with a
   `.old-<version>` suffix, removed on the next run that finds one, because a
   Windows executable cannot delete itself while it is running.

## Consumers

Additive: one new command and one new section in `ready --json`.
[`../docs/consumers.md`](../docs/consumers.md) gains a row, because a consumer
that pins a version now has a supported way off it.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case asserting `selfupdate --check --json` names the running version and the
newest release and does not modify the executable; a case asserting a tampered
download is refused with the executable untouched, driven by pointing the
verifier at bytes whose digest does not match; and a case asserting
`ready --json` carries `update.checked: false` with a reason rather than a
missing key when the release cannot be resolved.

⛔ **The end-to-end replacement is proved in `consumer.ps1`, not here**, because
proving it means running a DIFFERENT version and this suite builds one binary
from the working tree.

## Closed 2026-09-10

⛔ **ONE WRITE-UP.** The closure evidence for this entry is under
[WSL-49](wsl-toolkit-go.md), because the update check is a section of `ready`'s
schema and the two were built together rather than one after the other.

⚠ **The end-to-end replacement was driven HERE rather than in `consumer.ps1`**,
against the real published `wsl-toolkit-v1.2.0`, on a copy of the binary in a
scratch directory. That is a better proof than the entry asked for: it is the
running executable being replaced, verified against the digests GitHub actually
published, rather than a simulation of one.

Both forks are settled and recorded under `WSL-49`, as recommended in each case.

---

## WSL-54. The three answers a diagnostic has to tell apart

**Source** The residual bound left by [WSL-48](wsl-toolkit-go.md) on 2026-09-10,
named rather than left in a sentence.
**Category** wsl, **Priority** P2, **Effort** S, **Status** done

## Problem

`Get-EnginePlatform` now separates three outcomes: the engine was never reached,
the engine answered a nonzero exit, and it answered. Only the third is driven by
any suite, because the machine that runs the acceptance suite has a working
podman and neither of the other two can be produced on it by asking politely.

⛔ **A branch nothing has ever been seen to take is a branch nobody has proved.**
Two of the three answers this diagnostic exists to give are in exactly that
state, and the defect it was written for was the diagnostic giving the WRONG one
of the three.

## Premise

Measured on 2026-09-10: the acceptance suite drives `script -Action Doctor`
against a host with podman 5.8.6 installed and answering, and asserts the engine
row. No case makes the engine absent, and no case makes it answer nonzero.

⚠ This is a bound on the SUITE, not a defect in the tool. The code was read and
the three branches are distinct; what is missing is evidence that each is taken
when it should be.

## Approach

The probe takes the engine as a parameter, so the two missing answers are
reachable without breaking the machine:

- an engine whose `Path` names a file that does not exist, which is "never
  reached";
- an engine whose `Path` names a program that exits nonzero and writes a line,
  which is "answered a nonzero exit". A two-line `.cmd` in a temp directory is
  enough, and `Get-ContainerEngine` is not involved.

⛔ **It must not stop the real podman**, on this machine or on anybody's. A case
that runs `podman machine stop` to produce an answer has broken the host the
suite is standing on.

⚠ **It belongs in `selftest.ps1` only if the decision can be separated from the
process call**, the way `Resolve-DistroListing` was. If it cannot, it is an
acceptance case with a fixture rather than a pure one.

## Consumers

None: this is a test surface.

## Prove

```bash
pwsh -NoProfile -File scripts/windows/wsl-toolkit/selftest.ps1
```

Three cases, one per outcome, each asserting the MESSAGE rather than the fact of
an error: "never reached" must not claim the engine is broken, and "answered
nonzero" must carry the child's code and its output. A case that only checks that
something threw would stay green with the three branches collapsed back into one,
which is the defect.

## Closed 2026-09-10

Three cases in `selftest.ps1`, each passing the engine as a parameter so the two
missing answers are reachable without breaking the real one. Nothing here stops
podman and nothing here writes.

```text
  ok    an engine that was never reached does not claim the engine is broken
  ok    an engine that answered nonzero is reported with its own code
  ok    an architecture an engine reports maps onto what --platform accepts
```

⛔ **THE MUTATION PASS CAUGHT THE FIRST VERSION BEING THEATRE**, which is the
entire reason this entry existed. Collapsing the three branches back into one -
`if ($ran) { throw }` to `if ($false) { throw }` - left the suite GREEN, because
the "never reached" message APPENDS the underlying error and the case asserted
only that `answered exit` appeared somewhere in it. It appears in both.

The case asserts the NEGATIVE now, mirroring the other one: an engine that
answered must not be described as one that was never reached. Re-run with the
guard still mutated:

```text
  FAIL  an engine that answered nonzero is reported with its own code
selftest: 1 of 131 case(s) FAILED over 36 function(s).
```

and green with it restored.

⚠ **The third case is not padding.** A rule that widened to refuse everything
would pass both cases above and break every working machine, so what a real
engine's answer maps onto is asserted beside them.

⚠ **TWO THINGS ABOUT THE FIXTURE ARE WORTH KEEPING.** `cmd.exe` was the obvious
choice for "a program that exits nonzero" and it HANGS: `Invoke-Native` passes a
fixed argument list, and cmd.exe handed arguments it does not recognise opens an
INTERACTIVE shell and waits on stdin, so the suite stopped rather than failing.
`whoami.exe` refuses an unknown option, says so, and exits. And the absent-engine
fixture is a path under TEMP that does not exist, which is a fact about the
filesystem rather than about any engine.

---

## WSL-55. A report that creates the thing it is describing

**Source** Found on 2026-09-10 while writing an acceptance case for
[WSL-49](wsl-toolkit-go.md): a first-run case pointed `ready` at a state
directory that did not exist, and the directory existed afterwards.
**Category** wsl, **Priority** P2, **Effort** S, **Status** done

## Problem

`internal/toolkit/paths.go` states the rule in its own words:

> EnsureHome creates the state directory. Read-only commands must not call it: a
> report that creates a directory on a machine it is only describing has changed
> the thing it was asked to measure.

`NewBase` calls `EnsureHome`, and `base status` calls `NewBase`. So a read-only
report creates the state directory, and `ready` inherits it through the same
path. Measured:

```text
rm -rf .tmp/probe1
wsl-toolkit --home .tmp/probe1 base status --json
ls .tmp/probe1        ->  the directory exists
```

⚠ **Nothing is corrupted and nothing is lost.** An empty directory appears where
a caller asked a question. It matters because the rule is written down in this
tree, in the function it is about, and the code does the opposite: a rule with a
comment and no instrument is the shape `TOOL-15` was filed for.

## Premise

Measured on 2026-09-10 with the command above. ⚠ `resources`, `gc` and `logs`
were NOT checked and may have the same shape; whoever takes this counts them
rather than assuming.

## Approach

Split what `Base` needs from what it creates. `Status` reads a record and asks
WSL; neither needs the directory to exist, and a missing record is already
handled as "no record". The likely shape is a `Home()`-based constructor for the
read paths and `EnsureHome` on the paths that write.

⛔ **Not a flag.** "Read-only unless you pass --write" is a rule a caller has to
remember, and the rule here is about what a command IS.

⚠ **`ready` is the one with the most to lose.** It is the command an agent runs
first, on a machine it has not touched, and creating state there is the one thing
its own manual page says it does not do.

## Consumers

None directly: no consumer reads whether a directory exists. The manual's
`ready` section claims it creates nothing, so that claim moves with the fix or
the fix moves to match it.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case pointing every read-only command at a state directory that does not
exist, asserting the answer arrives AND the directory is still absent
afterwards. ⛔ Both halves: a case that only checks the directory would pass
against a command that stopped answering.

## Closed 2026-09-10

`NewBase`, `NewRunner`, `OpenLedger` and `cmdLogs` bind to `Home()` now; the
paths that WRITE call `EnsureHome` where they write. Reading a record from a
directory that does not exist was already "no record", so nothing needed the
directory to exist in order to answer.

Driven against every read-only command, each pointed at a state directory that
did not exist:

```text
read-only: base status --json
read-only: ready --json
read-only: resources --json
read-only: logs --json
read-only: config --json
read-only: images --json
read-only: version --json
```

⛔ **AND THE OTHER HALF, which the entry required.** A tool that stopped
answering would pass the list above, so the case asserts that `config --write`
still brings the directory into existence and writes into it.

⚠ **The premise named `resources`, `gc` and `logs` as unchecked and asked
whoever took this to count them rather than assume.** Counted: `resources` and
`logs` both had it, through `NewRunner` and through a direct `EnsureHome`
respectively; `gc` reaches the same `NewRunner` and is covered by the same
change. That is four commands with the defect, not the one the entry was filed
from.

---

## WSL-56. What a failed job leaves a reader to ask the machine by hand

**Source** The half of [WSL-50](wsl-toolkit-go.md) that was not built on
2026-09-10, named as its own entry rather than left in a sentence.
**Category** wsl, **Priority** P2, **Effort** M, **Status** done

## Problem

`WSL-50` asked for two things and one of them shipped. The heartbeat is built: a
job that is running now says so, with elapsed time, its deadline and the bytes
each stream has carried.

The other half is what a caller has after a job has FAILED. The operator's words
were "better diagnostics and debug for the base machine and every container
inside it, by either engine". Today that is an exit code, a transcript, and
`resources`, which enumerates what exists rather than describing it.

Specifically absent, and each was named in `WSL-50`:

- the engine's version and storage driver;
- cgroup and namespace facts for the container;
- the container's own last state and exit, AFTER it has gone;
- disk pressure inside the guest;
- the same for a chroot payload, where podman is not the engine.

## Premise

Read, not measured. `resources` already asks the engine for images, containers
and volumes and the guest for job directories, so the transport and the
enumeration exist; what does not exist is anything that asks a container what it
WAS.

⚠ **The third bullet is the one with a real obstacle and it should be measured
first.** `run --rm` removes the container when it exits, so its state is gone
before anything could read it. Whoever takes this finds out whether podman's
events or its exit journal retains enough after `--rm`, and if neither does, says
so and picks between keeping failed containers for an age window and accepting
that the transcript is what survives. ⛔ Do not quietly drop `--rm`: it is what
stops a failed fleet leaving twelve containers behind.

## Approach

One command, `wsl-toolkit inspect [JOB]`, over the data an engine already holds,
with `--json`.

⛔ **It must not become a second `resources`.** `resources` answers "what is
this tool holding"; this answers "what was this job, and what was the machine
doing when it ran". If it starts enumerating, it has become a copy.

⚠ **By either engine.** podman and docker disagree about field names and about
the VALUES, which `WSL-31` already paid for once: podman answers
`{{.Host.Arch}}` and docker `{{.Architecture}}`, and asking the wrong one fails
the whole call and reads as a broken engine.

## Consumers

Additive: one new command and one new JSON schema.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case running a job that fails, then asserting `inspect` on its id names the
engine version, the storage driver and the container's last exit; a case
asserting `inspect` on an id that never existed is a refusal rather than an empty
object. ⛔ The second one matters: an inspection surface that answers an empty
document for an unknown id is the "refusal rendered as a successful empty result"
class this tool has now paid for three times.


## Closing

**Closed 2026-09-10.** `wsl-toolkit inspect [JOB]`, with `--json` and `--since`.

⭐ **THE OBSTACLE THIS ENTRY NAMED DOES NOT HOLD, and measuring it first was the
right call.** The premise said `run --rm` removes the container before anything
can read its last state, and asked whoever took this to find out whether
podman's events or its exit journal retained enough afterwards. They do.
Measured against podman 6.1.1 inside the base, after a job that exited 37, with
the container long gone:

```text
{"ContainerExitCode":37,"ID":"9adc7e38985000039...","Image":"docker.io/library/alpine:latest",
 "Name":"wtk-4c21ea7f6fb89b2a","Status":"died","timeNano":1789030870819333184,"Type":"container"}
```

The event logger is `file`, and both the `died` and the `remove` events carry
the exit code. ⛔ So `--rm` stays, which is what stops a failed fleet leaving
twelve containers behind, and the entry's fallback options were not needed.

**Two measurements that cost time and are kept in the code**, because either one
would be re-derived by the next person:

- `podman events --until 0s` answers NOTHING for events that are certainly
  there. `--stream=false` with the same `--since` answers all of them.
- `podman events` exits 0 for a container that never existed, so an empty
  journal is not a refusal on its own. An unknown id is decided against four
  sources rather than against the journal alone.

The acceptance suite, against the real machine:

```text
  ok    inspect answers what a failed job ran on, and the engine agrees about its exit
  ok    inspect refuses an id that never existed rather than answering an empty object

acceptance: 69 case(s) passed against a real machine.
ACCEPTANCE_EXIT=0
```

⚠ **ONE BULLET OF THE PROBLEM DESCRIBES SOMETHING THAT DOES NOT EXIST.** It asks
for "the same for a chroot payload, where podman is not the engine". There is no
chroot route in this tool: every job runs in a container in the guest engine, and
`grep -rn chroot` over the module and the manual returns nothing. Written here
rather than as an edit to the problem above, because an entry that quietly loses
a requirement is one nobody can audit.

⚠ **AND THE TWO-ENGINE HAZARD IT NAMES IS THE HOST ENGINE'S, NOT THIS ONE'S.**
podman and docker do disagree about field names and values, which `WSL-31` paid
for once. That applies to the engine that turns an image into a rootfs, which
`doctor` reports. Jobs run in a rootless podman this tool installs inside the
base, so `inspect` has one spelling to read rather than two, and saying so is
cheaper than a compatibility layer nothing exercises.

⭐ **The work found a defect in a different guard.** `TestManualNamesEveryFlag`
says in its own header that a flag added tomorrow is covered without the file
being touched. That was true of a flag added to a command already on its
hand-written list and false of a whole new command: `inspect` arrived with two
flags and the case stayed green over both. `main.go` dispatches from a table
now, the test walks it, and the first run after that change refused
`--since` for not being in the manual.
---

## WSL-57. Two selftest cases were written against one host's environment

**Source** The ubuntu CI job, on the first run that saw the 2.0.0 commits. Four commits had been made and not pushed, so the second host had not looked at them.
**Category** wsl, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`bundle` went red in CI on ubuntu and green on Windows, with one line:

```text
  FAIL   bundle          1
           bundle: selftest: FAIL
```

Two cases threw rather than failed:

```text
  FAIL  an engine that was never reached does not claim the engine is broken
        actual   <UNEXPECTED THROW: Cannot bind argument to parameter 'Path' because it is null.>
  FAIL  an engine that answered nonzero is reported with its own code
```

Both reach for a Windows environment variable to build a path: `$env:TEMP` for
an executable that is not there, and `$env:WINDIR` for one that refuses an
unknown option. ⛔ **Both are null under PowerShell on Linux**, and `Join-Path`
refuses a null `Path` rather than returning one.

⚠ **The tool is Windows-only and its SUITE is not**, which is the distinction
that was missed. `selftest.ps1` tests pure functions, touches no WSL and no
engine, and therefore runs on the ubuntu job - where it is the second sample for
every claim about a renderer that reads the host's culture.

## Premise

Measured three ways on 2026-09-10, after the fix:

```text
pwsh 7 on Windows      {"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}
Windows PowerShell 5.1 {"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}
pwsh 7 on Linux        {"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}
```

⭐ **The third one was measured on this Windows host in about ten seconds**, in a
container, which is the finding worth more than the fix:

```powershell
podman run --rm -v "${PWD}:/repo:ro" mcr.microsoft.com/powershell:latest pwsh -NoProfile -File /repo/scripts/windows/wsl-toolkit/selftest.ps1
```

⚠ **Before that was known, the only way to see the Linux answer was to push and
wait**, which is why four commits' worth of this defect sat unseen.

## Approach

The rule this tree already states, applied to a test: **resolve rather than
spell**.

- `[IO.Path]::GetTempPath()` in place of `$env:TEMP`, which answers on every
  host the runtime supports.
- `Get-Command whoami -CommandType Application` in place of a composed
  `System32\whoami.exe`. ⚠ The program is the same kind of thing on both:
  Windows answers `ERROR: Invalid argument/option` and GNU coreutils answers
  `extra operand`, and the case only needs a nonzero exit with a message.

⛔ **Not a skip on Linux.** A case that excuses itself on the host CI runs is a
case that has been deleted with extra steps, and the `SKIPPED` work in `TOOL-19`
exists precisely because a skip is not a pass.

## Consumers

None. `selftest.ps1` is not published; it is not in the release and no consumer
fetches it.

## Prove

```bash
sh scripts/common/check-gate.sh
```

Green on Windows, and the same file green under PowerShell on Linux with the
same case count, both read from the process unpiped.

## Closing

**Closed 2026-09-10.** Two lines, and the counts agree across three hosts:

```text
{"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}
PWSH7=0
{"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}
PS51=0
```

```text
{"schema":"wsl-toolkit-selftest/1","cases":131,"failed":0,"functions":36}
LINUX_EXIT=0
```

⚠ **The count is identical on all three, which is itself the assertion.** Before
`af3de74` the ubuntu job ran 123 cases over 32 functions against Windows's 131
over 36, and nothing compared the two numbers. `TOOL-11`'s CI step now compares
the two PowerShell hosts on Windows; ⛔ **nothing compares the ubuntu count to
the Windows one**, and that is left open rather than claimed: the two jobs run
on different machines and there is no artefact between them.

⭐ **The reason this was found late is not the defect and is worth more.** Four
commits were made on 2026-09-10 and none was pushed, so the second host had not
run at all since `f7eabfe`. A local gate that is green on one host is evidence
about that host. [`../docs/methodology/gate.md`](../docs/methodology/gate.md)
already says the CI result is the one that gates a merge; what this adds is that
the ubuntu half of it is now reproducible locally, in a container, in seconds.

---

## WSL-58. `inspect` is the one report the helper route cannot serve

**Source** The door sweep on 2026-09-10, immediately after `WSL-56` shipped `inspect`. It was not in the task list and the task list has never once contained them all.
**Category** wsl, **Priority** P2, **Effort** M, **Status** done

---

## Problem

The helper exists for a caller that cannot reach `wsl.exe` itself. `run`,
`matrix`, `base`, `resources` and `gc` all take `--via-helper` and go through it.
⛔ **`inspect` does not.** It builds a `Runner` and talks to the guest directly,
so on a restricted client the half of its answer that needs the machine - the
engine, the storage, the container's own last exit, the guest's disk - is
unreachable.

⚠ **This is the one-gated-door class**, from
[`../docs/conventions/forbidden-patterns.md`](../docs/conventions/forbidden-patterns.md):
`resources` is the sibling report and it has the door.

## Premise

Measured by reading, on 2026-09-10: `useHelper(` appears in `cmd_admin.go`
twice, `cmd_base.go` once and `cmd_run.go` twice, and nowhere in
`cmd_inspect.go`.

⭐ **The host half is already answered without a runner**, which is the other
finding from the same sweep and is fixed:
`toolkit.InspectHostOnly` reports the transcript and the ledger record on a
machine with no `wsl.exe` at all, and names the engine as unreached rather than
omitting it. So a restricted client gets a partial answer today rather than
exit 2. What is missing is the machine half.

## Approach

An `inspect` method on the helper protocol, and `--via-helper` on the command,
exactly as `resources` has them.

⛔ **IT IS A PROTOCOL CHANGE AND THAT IS THE WHOLE COST.** The helper protocol
is at version 3 and `wsl-toolkit-v2.0.0` shipped with it. A client and a helper
that disagree about the version already refuse each other by design, so adding a
method means version 4, and a caller running a v2.0.0 helper against a newer
client gets a refusal until they restart it. That is correct behaviour and it is
still a thing a consumer has to do.

⚠ **Which is why this is not folded into `WSL-56`.** Bumping the protocol in the
same change that added the command would have put a release-visible break behind
a feature nobody had asked for on that route.

## Consumers

None today: no row in [`../docs/consumers.md`](../docs/consumers.md) runs the
helper. ⚠ The protocol bump is what a consumer would see, and it belongs in the
changelog as a break when it lands.

## Prove

```bash
pwsh -NoProfile -File tools/windows/wsl-toolkit/acceptance.ps1
```

A case that starts a helper, runs a job through it, and asserts
`inspect --via-helper` on that job id names the engine version and the
container's last exit, against the same job's direct answer.

---

## Closing

**Closed 2026-09-10T13:40:00Z.** `/v1/inspect` on the helper, `--via-helper` on
the command, and the protocol at `wsl-toolkit-helper/4`.

⭐ **The two routes turn a report into an exit code in ONE function.**
`renderInspectResult` is called by both, because a second copy on the helper
path is how `gc` came to honour a flag on one route and drop it on the other.

⛔ **The unknown-job refusal survives the wire as the same typed error.** The
helper sets a flag the client rebuilds `ErrUnknownJob` from; a generic "the
helper refused" would have landed in exit 2 where the direct path answers exit
1, giving two exit codes for one question.

```text
$ pwsh -File tools/windows/wsl-toolkit/acceptance.ps1 -Binary wsl-toolkit.exe
  ok    inspect through the helper names the engine and the container exit
  ok    an unknown id is the same refusal down both routes

acceptance: 71 case(s) passed against a real machine.
```

The first case runs a job through the helper that exits 37, then asks both
routes about it and compares: the engine version has to be non-empty and equal
across the two, and the container's own last exit has to be 37 on both. ⚠ A
case asserting only that the helper ANSWERS would have passed over a second
implementation that answered something else.

⚠ **The protocol bump is a real cost and it is a consumer-visible one.** A
helper left running from `wsl-toolkit-v2.0.0` refuses a newer client until it is
restarted with `helper stop` then `helper serve --detach`. That is correct
behaviour by design and it is still a thing somebody has to do.

---

## WSL-62. two of this tool sharing one state directory corrupt each other's writes

**Source** the sixth review lens, run for the first time on 2026-09-10 after three sessions named it and none ran it.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** S, **Status** done

---

## Problem

Every stored file this tool owns goes through one helper, `writeFileAtomic`.
It wrote through a temporary named `path + ".tmp"`, which is ONE name for every
writer, so two processes saving a base record or a configuration to the same
state directory wrote the same temporary.

⛔ **Both failure shapes are silent about what actually went wrong.** On Windows
the loser reports a permission error for a write nothing was wrong with. On a
platform that holds no share lock the collision is quieter and worse: one writer
renames ANOTHER's bytes into place under its own name, and every later reader
gets a file nobody wrote.

## Premise

⭐ **Measured on 2026-09-10 rather than reasoned about**, with eight writers of
one path, forty rounds each, 4 KiB payloads:

| the helper | result |
| --- | --- |
| the shipped version, one shared `.tmp` | ⛔ 7 of 8 writers failed: `The process cannot access the file because it is being used by another process` |
| a temporary unique per write | ⛔ 7 of 8 still failed: `Access is denied` on the rename itself |
| unique temporary plus a bounded rename retry | ⭐ all 8 clean, and the file is exactly one writer's payload |

⚠ **The second row is the one worth keeping.** Windows refuses a replace while
another replace of the same target is in flight, so a unique temporary is
necessary and not sufficient. That is transient contention, and the message it
produces is indistinguishable from a real permission failure.

⚠ **The reasoning already existed two files away and was not applied here.**
`newJobID` in `internal/toolkit/job.go` carries it in one sentence: an identifier
two concurrent runs can produce is two runs writing into one place.

## Approach

`writeFileAtomic` in `internal/toolkit/catalog.go`, which is the single write
path for all seven callers: the base record, the configuration, the helper
endpoint, the instance pointer and the ledger compaction. The temporary carries
eight random bytes, and the rename goes through `renameReplacing`.

⛔ **The retry is bounded and returns the last error with its attempt count.** A
retry that hides a real refusal is worse than no retry, because a file nobody can
write then looks like a file that was written.

⛔ **This is not a lock and must not grow into one.** Two writers still race and
the last one wins; what is fixed is that the winner's bytes are its own and the
loser is told the truth. Whether the state directory needs an exclusive lock is a
larger question and `--instance` is the answer today.

## Consumers

None. No row of [`../docs/consumers.md`](../docs/consumers.md) reaches the
executable's state directory.

⚠ **The fix is on `main` and is NOT in `wsl-toolkit-v2.0.1`**, which was cut
before the review found it. The next tag carries it.

## Prove

```bash
sh scripts/common/check-go.sh
```

`TestWriteFileAtomicSurvivesConcurrentWriters` green, and the mutation row
`a temporary name that two concurrent writers cannot share` going red when the
shared name is put back.

---

## Closing

**Closed 2026-09-10T14:45:00Z.** Unique temporary, bounded rename retry, one case
and one mutation row.

```text
ok  	github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit	0.763s

  ok       a temporary name that two concurrent writers cannot share               1 case(s), went red
1 of 1 guards proved.
```

⛔ **What the lens did not reach, said rather than left to be found.** This is
one helper. Two processes running `base ensure` at once still both provision, two
`gc --apply` runs still both enumerate, and the ledger's cross-process append was
read and not driven: `O_APPEND` with one `Write` per record should interleave
whole lines, and nothing here measured it. ⚠ That is the honest scope of a first
concurrency pass, and it is smaller than the question the record has been asking.

---

## WSL-63. A consumer wrote a wrapper for four things this tool should have done

**Source** [Issue 29](https://github.com/Azathothas/ToolKit/issues/29), and the
consumer's own pages: `Azathothas/podbox` at `docs/containers.md` and
`scripts/windows/run-in-base.sh`, read 2026-09-12.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

⛔ **Issue 6 promised that a future agent would not need to write wrappers or
work around gotchas, and the next consumer did both.** `run-in-base.sh` is 151
lines, and four of the things in it are this tool's job:

| what the wrapper carries | what the caller sees without it |
| --- | --- |
| `restore-modes.sh`, run inside every job | `./scripts/common/bootstrap-env.sh: Permission denied`, naming the script and not the transfer. 396 of 396 files needed repair |
| `strip_cr`, over every payload | `/bin/sh` reads a carriage return as part of the last word, so a redirection names a file nobody wrote |
| four `--exclude` names, found by bisection | `archive/tar: write too long` and the job dead in 475 ms, naming the archiver and not the file |
| a written rule saying never to write to `/mnt/c` | a wrong path in a job destroys the real checkout on Windows |

## Premise

⭐ **Measured by the consumer, on this class of host, and re-derived here.**

- NTFS holds no POSIX mode and `core.fileMode` is false there, so
  `writeWorkspaceTar` set `0o755` from the host's own executable bit, which is
  never set. Every file arrived at `0644`.
- The copy was unbounded against a header whose size was already written, so a
  file that GREW overran its own declared length. A tar member's size goes into
  the header before its bytes are read.
- `--script` already repaired CRLF and `-c` did not, which is the one-gated-door
  shape: one channel enforcing what its sibling does not.
- `/etc/wsl.conf` carried `automount enabled=true` with no options, so every
  fixed drive mounted read-write inside the base.

⚠ **The git index is not the whole answer, and the same consumer measured the
gap three weeks later**: a new script that was written and not staged is in no
index, so the repair reported 396 of 396 fixed in the run that failed
`Permission denied`, rc 126.

## Approach

- `internal/toolkit/execbit.go`, read by `writeWorkspaceTar`: `git ls-files -s -z`
  supplies the mode, a shebang covers a file in no index, and the host's own bit
  still wins where the filesystem carries one.
- The same walker bounds its copy by the size in the header.
- `WorkspaceUpload` gains `Truncated` and `ExecRestored`, both reported.
- `jobFlags.script` sends `-c` through `RepairGuestScript`, which `--script`
  already used.
- `base.automount` with `ro`, `rw` and `off`, defaulting to `ro`.

⛔ **Not a recursive chmod.** That marks data executable and reports nothing. A
file whose first two bytes are a shebang is a script by its own declaration, and
the count is announced rather than applied in silence.

⛔ **A truncation is NOT counted as an omission.** `Omitted` means an input did
not arrive, which is what makes a job's conclusions wrong. A prefix of a file
something is still writing is a snapshot. Folding them together makes the
serious number go up for the ordinary case.

## Consumers

⛔ **`base.automount` is a BREAKING change by
[`../docs/consumers.md`](../docs/consumers.md)'s definition** for any caller that
WRITES to `/mnt/*` inside the base. Reading is unaffected. Neither registered
consumer does: `Azathothas/TEMPLATE` and `Azathothas/bit-cli` fetch
`wsl-toolkit.ps1`, which creates its own throwaway distributions and never reads
this setting. `Azathothas/podbox` reaches the base through the executable, and
its own page already forbids writing there.

## Prove

```powershell
go test ./internal/toolkit/ -run 'ExecutableBit|Unstaged|Grows|Automount|Provisioner'
```

---

## Closing

**Closed 2026-09-12T11:54:00Z.** Five cases, and a live job against a workspace
built to carry all three mode outcomes at once.

```text
--- PASS: TestTheExecutableBitSurvivesAFilesystemThatCannotHoldIt (0.28s)
--- PASS: TestAnUnstagedScriptStillArrivesRunnable (0.01s)
--- PASS: TestAFileThatGrowsDoesNotKillTheCopy (0.02s)
--- PASS: TestAutomountDefaultsToReadOnly (0.00s)
--- PASS: TestTheProvisionerReadsTheAutomountSetting (0.00s)
```

Driven on this host, against three files that are identical on NTFS:

```text
  workspace: 1 file carries the executable bit from the git index, and 1 file from a shebang
-rw-r--r--    1 root     root            12 data.txt
-rwxr-xr-x    1 root     root            25 staged.sh
-rwxr-xr-x    1 root     root            27 unstaged.sh
staged-ran
unstaged-ran
good-data-not-executable
```

⭐ **The automount ruling is proved on a base provisioned from this change**, not
on the settings file it writes. A fresh instance, then the guest's own view:

```text
/etc/wsl.conf   [automount] enabled=true / options="metadata,ro"
/proc/mounts    C:\134 /mnt/c 9p ro,noatime,aname=drvfs;path=C:\;...

ls /mnt/c/Users                          AjamX / All Users / Default
touch /mnt/c/wsl-toolkit-write-probe.txt Read-only file system, exit 1
Test-Path C:\wsl-toolkit-write-probe.txt False
```

⚠ **Reading still works, and that is the point of `ro` over `off`.** A caller
that reaches for one file on the Windows drive is doing something ordinary; the
door that closes is the one that destroys a checkout.

⚠ **What this does not reach, said rather than left to be found.** The
consumer's seventh trap stands: killing the wrapper on the Windows side does not
stop the job in the guest, because a killed process runs no handler. The
persistent lifecycle makes the container findable afterwards through
`resources`, which is a way to SEE it rather than a way to stop it. That remains
open ground and is named in [`PROGRESS.md`](PROGRESS.md).

---

## WSL-64. A native job ran whatever architecture the image store happened to hold

**Source** found on 2026-09-12 while driving `WSL-63`'s workspace case, one
command after an unrelated foreign-architecture run.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** S, **Status** done

---

## Problem

⛔ **A job that asked for no platform answered `aarch64` on an x86_64 host.**
The only sign was one line on podman's stderr saying the image platform did not
match the expected one. A caller reading the JSON answer never sees it, and a
caller reading the console sees it beside a green exit code.

## Premise

⭐ **Measured, not reasoned about.** `NormalizePlatform` answered empty for an
empty value, and the runner appended `--platform` only when the value was not
empty. With no `--platform` on the command line podman selects whatever variant
of the reference is already in the local store, so ONE foreign-architecture run
of `alpine` changed the meaning of every later native run of `alpine` on that
machine.

⚠ **A green suite could not have seen this.** The defect lives in what the local
image store happens to contain, not in the code's own logic. Every unit test
passed over it, and so did the acceptance runner, because both run against a
store that no foreign-architecture job had touched.

⛔ **The cost is a wrong answer that looks like a right one.** A libc check, an
ABI probe or a benchmark run this way measures a machine that is not there.

## Approach

`NativePlatform` in `internal/toolkit/catalog.go` maps the executable's own
`GOARCH`, because a WSL guest's architecture follows its Windows host's.
`jobFlags.applyConfig` resolves an empty value to it, so every job names a
platform. `Base.EnsurePlatform` returns early for the native value, which keeps
the common path free of the guest round trip it would otherwise now make.

## Prove

```powershell
wsl-toolkit run --image alpine --platform arm64 -c 'uname -m'
wsl-toolkit run --image alpine -c 'uname -m'
```

---

## Closing

**Closed 2026-09-12T12:10:00Z.** Driven in the order that produced the defect:
the foreign-architecture run first, the native run second.

```text
  "stdout": "x86_64\n",
  "platform": "linux/amd64"

  "stdout": "aarch64\n",
  "platform": "linux/arm64"
```

The resolved platform is on the result now, so a caller can check the claim
rather than trust it.

---

## WSL-65. The generated manual carried a control byte, and its drift check agreed with it

**Source** found on 2026-09-12 by reading the generated file's bytes, during
issue 29's manual work.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** S, **Status** done

---

## Problem

`wsl-toolkit.1`'s SEE ALSO line shipped `0x0C`, a form feed, where it meant to
change font. roff's font escape and Go's form feed are spelled identically
inside an interpreted string literal.

## Premise

⛔ **`TestGeneratedManPageIsCurrent` COMPARES GENERATED OUTPUT WITH GENERATED
OUTPUT.** It reads the tracked file and re-renders it from the CLI, so a
generator that emits the wrong bytes agrees with itself and the gate stays green.
The check is correct for what it is for, which is drift, and structurally blind
to this.

⚠ **The tree's own `control-bytes` rule did not catch it either**, because that
rule walks the tracked set and the generated page was still untracked when the
generator was written. A rule over the tracked set is blind for exactly as long
as a new file stays untracked, which is the window a generator lands in.

## Approach

A raw string literal in `renderManPage`, and `TestGeneratedManPageIsText`, which
asserts the property the drift check cannot: every byte is text or a newline.

## Prove

```powershell
go test . -run TestGeneratedManPageIsText
```

---

## Closing

**Closed 2026-09-12T11:30:00Z.** The case fails against the old generator and
passes against the new one, and `control-bytes` covers the file from here on
because it is tracked now.

---

## WSL-66. `--workspace .` resolved against a directory the caller could not see

**Source** [Issue 29](https://github.com/Azathothas/ToolKit/issues/29), task 7,
in the operator's words: an agent "end up copying/modifying dirs/projects they
shouldn't be able to".
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

The executable is installed under the home directory and is run from anywhere.
A relative `--workspace`, `--script`, `--artifacts` or transcript path resolved
against the process's working directory, which a sandbox can reset to the drive
root or the home directory without the caller knowing.

⛔ **Both directions are expensive.** A workspace copy walks whatever it was
pointed at, and an artifact delivery WRITES into it.

## Premise

Read from the code and confirmed by running it: `run` and `matrix` called
`filepath.Abs` on the caller's value before any configuration was loaded, so
nothing but the working directory could have been the base.

## Approach

Two halves, because neither is enough alone.

1. ⭐ **An anchor.** The configuration is loaded first, and a relative path
   resolves against the directory holding the nearest or named project
   configuration. A relative `jobs.workspace` always resolves against its own
   file. With no project configuration there is nothing to anchor to, and the
   working directory stays the base.
2. ⭐ **A refusal.** `AssertProjectPath` in `internal/toolkit/anchor.go` refuses
   a resolved filesystem root, home directory or system directory.

⛔ **A refusal and not a correction.** Guessing which directory the caller meant
is how a tool acts on the wrong tree and reports success.

⚠ **The line is the home directory ITSELF, not everything under it.** The tool
lives under the home directory on this host, so most real projects are below it
and refusing the subtree would refuse the ordinary case.

The resolved host path is printed when the workspace is copied, so a caller can
see which tree actually travelled.

## Prove

```powershell
go test ./internal/toolkit/ -run 'ProjectPath|HomeDirectory'
go test . -run TestRelativePathsResolveAgainstTheProject
```

---

## Closing

**Closed 2026-09-12T12:20:00Z.** The anchor has a case that starts outside the
project directory; the refusal has one for each shape it refuses and one for the
ordinary trees it must not. The copy line now reads
`workspace: N entries, X copied from HOST to GUEST`.

---

## WSL-67. A provider's Linux-only CLI, run from Windows as if it were native

**Source** [Issue 30](https://github.com/Azathothas/ToolKit/issues/30), part 1,
filed by the operator on 2026-09-12. **Implementation checkpointed
2026-09-12T14:18:33Z.** The provider-neutral base is built and driven; the
provider's own installer and authenticated smoke remain.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** open

---

## Problem

Several AI providers ship a Linux-only CLI and support no other harness. Running
one from Windows today needs glue nobody wants to own. The operator's case, in
their own terms:

- install a provider CLI into a named base, for example `wsl-toolkit-muse`;
- that base carries the tooling the agent needs: `grep`, `ripgrep`, `codegraph`,
  `podman`;
- the agent works on ONE Windows checkout, for example
  `C:\Users\AjamX\Downloads\some-repo`, which must look like a native Linux
  directory and must be READ-WRITE, including `git commit` and `git push`;
- ⛔ **it must reach no other directory on the Windows host** unless that is
  granted explicitly;
- the base runs systemd;
- a terminal multiplexer is present, so a session can be detached and reattached
  and cannot be quit by accident;
- `tools/windows/wsl-toolkit/examples/` holds `muse-code/` and `common/`, with
  bootstrap scripts, a multiplexer configuration, and an end-to-end guide.

## Premise

⭐ **Measured against a disposable Arch base on 2026-09-12.** The base was
created, transitioned through one grant, zero grants, and one grant again, and
then removed through the marker-verified removal path.

What already exists:

| the ask | what is there today |
| --- | --- |
| a named, long-lived base | ⭐ `--instance NAME` gives an isolated distribution and state directory, and `base ensure` provisions it |
| podman inside it | ⭐ the base is rootless podman, which is what it is for |
| a Linux view of a Windows directory | ⭐ `base.mounts[]` grants an explicit directory below `/workspaces`, read-only by default and read-write only when requested |
| systemd | ⭐ `base.systemd` installs it where the package family supports it and verifies PID 1 after restart |
| arbitrary tooling in the base | ⭐ `base.toolset = "developer"` installs and verifies bash, a build chain, curl, git, jq, Node/npm, OpenSSH, ripgrep, tmux and unzip |
| a multiplexer | ⭐ the common example installs tmux and a checked-in configuration that keeps sessions and panes alive |
| `examples/` | ⭐ `common/` and `muse-code/` now exist with a pinned CodeGraph bootstrap and an end-to-end guide |

⛔ **THIS ENTRY COLLIDES WITH `WSL-63` AND THE COLLISION IS THE INTERESTING
PART.** `WSL-63` made `/mnt/*` read-only in the base, on the operator's ruling,
because a job with a wrong path in it could destroy the real checkout. This ask
needs ONE Windows directory to be writable and the rest to be unreachable. A
global `base.automount` setting cannot express that: `ro` blocks the commit,
`rw` grants the whole drive.

⭐ **So the seam is per-path, not per-base**, and that is a design question this
entry must answer before any of the rest of it is built.

## Approach

The first candidate was selected and built:

1. ⭐ **`automount off`, `interop off`, and explicit DrvFS mounts in an owned
   `/etc/fstab` block.** Everything else under the Windows drives is absent.
   Config validation requires this pairing and confines guest targets below
   `/workspaces`; host sources are canonicalized and pass the existing project
   path safety boundary.
2. **A base preset that installs a named tool set**, extending
   `cmd_base_preset.go` rather than forking a second provisioning path.
3. **systemd as a per-base option**, which `provision.sh` already has the shape
   for and currently pins off.
4. **The multiplexer**, with a configuration whose detach and quit keys are
   stated in the guide. The operator's constraint is that it must not quit by
   accident, which is a configuration decision and not a package choice.
5. **`tools/windows/wsl-toolkit/examples/`**, with `common/` and one provider
   directory.

⛔ **Do not install a provider's CLI by piping a URL into a shell inside this
repository's own scripts.** `docs/security/remote-ops.md` and
`docs/consumers.md` both carry the pinning rule. An example may SHOW the
provider's documented command; a script in this tree that runs it must pin and
verify.

## Decision

⭐ **Ruled by the operator's explicit requirement:** `automount off` plus only
the explicit DrvFS grants, with Windows interop also off. A read-only global
automount was rejected because it still exposes every drive; copying was
rejected because the provider must operate on the real checkout. The guest
runs provider work as the unprivileged base user. The root shell warns that
root can mount more host paths manually; this is access minimisation, not a WSL
security boundary.

## Consumers

None directly. ⚠ A change to `base.automount`'s meaning would reach anything
built on `WSL-63`, so a per-path model is added BESIDE that setting rather than
replacing it.

## Prove

Driven on a disposable base, not inferred from configuration:

- with one read-write grant, the unprivileged user saw exactly that DrvFS
  mount, read a sentinel and wrote a file that arrived in the Windows checkout;
- Windows interop was absent, systemd was PID 1, Podman was 6.1.1, the
  developer commands were present, and 31 QEMU handlers were registered;
- changing the same instance to zero grants made verification refuse the stale
  mount, reprovision removed it, and the unprivileged user could not mount C:;
- changing back to the sole ToolKit checkout grant let the common bootstrap
  verify both npm SHA-512 digests and install CodeGraph 1.5.0 and tmux 3.7c;
  tmux reported `remain-on-exit on` and `exit-empty off`;
- four new mutation rows were planted and proved individually: drive automount
  (3 cases), Windows interop (3), guest target confinement (7), and host-source
  root refusal (1).

⚠ **Still open:** wire these provider-profile cases into the main acceptance
runner; drive the `developer` package map on non-Arch package families; and run
the provider's installer plus an authenticated Muse smoke. The official Muse
documentation is login-gated, so the example deliberately leaves download,
review and execution of that installer as an operator step rather than running
an unpinned remote script.

---

## WSL-68. A base that can reach nothing on the host at all

**Source** [Issue 30](https://github.com/Azathothas/ToolKit/issues/30), part 2.
**Partially implemented and measured 2026-09-12T14:18:33Z.**
**Category** wsl-toolkit-go, **Priority** P2, **Effort** M, **Status** open

---

## Problem

The operator wants a base to hand to other people, or to run an experiment in,
where every action is confined to that base. No file on the Windows host is
reachable. Features such as `fuse` and podman are available only if that
containment can be guaranteed with them present.

## Premise

⭐ **The filesystem and interop doors were measured on a live zero-grant base.**
The others are intentionally still open work:

- ⭐ **interop.** `base.interop = "off"` writes both interop switches off and
  verification checks the effective guest environment after restart.
- ⛔ **the network.** The guest reaches the Windows host at its own address, and
  `HostAddress` exists precisely to report it. A sealed base has to answer what
  that means for anything listening on the host.
- ⚠ **`/init` and the WSL service.** A distribution talks to the Windows side by
  design, and how much of that survives `interop=false` was not measured.
- ⚠ **podman and fuse inside a sealed base.** The ask makes these conditional on
  the guarantee, so the guarantee is the deliverable and the features follow it.

⛔ **A claim of containment that has not been attacked is not a claim.** This
entry does not close on reading the settings. It closes on a probe that TRIES
each door and reports what it found.

## Approach

1. Enumerate the doors above on this host, by running things rather than reading
   about them.
2. A `sealed` base preset that closes the ones that can be closed.
3. ⭐ **A probe that attacks its own base** and reports each door as open or
   closed, in the shape `ready` and `base status` already use, so the answer is
   something an agent reads rather than something a page promises.
4. Say in the manual what is NOT sealed, with the measurement beside it.

⛔ **Do not describe this as a security boundary until the probe says so.** WSL
is not a sandbox by design, and a page that claims isolation the kernel does not
provide is worse than no page.

## Consumers

None. This is a new preset.

## Prove

Partial live result: after the same base was changed to zero grants, the
verifier caught and removed its stale project mount; the unprivileged account
had no user-directory DrvFS mounts, could not mount C:, had no Windows interop,
and retained working systemd. WSL's internal read-only
`/usr/lib/wsl/drivers` mount remained and is explicitly excluded from the user
grant allowlist.

⛔ **This does not close the entry.** Network reachability was not attacked;
guest root can manually request a host mount; and WSL's own service boundary
has not been enumerated. A `sealed` preset, an attacking probe that reports
each door, and a precise threat model remain. Until then the documentation
calls the implemented shape **zero grants**, never a sandbox or security
boundary.
