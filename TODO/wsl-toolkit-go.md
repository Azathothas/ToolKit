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
  executable  C:\Users\USER\Downloads\ToolKit\.tmp\wsl-toolkit.exe
  state       C:\Users\USER\AppData\Local\wsl-toolkit
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
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** done

---

## Problem

Several AI providers ship a Linux-only CLI and support no other harness. Running
one from Windows today needs glue nobody wants to own. The operator's case, in
their own terms:

- install a provider CLI into a named base, for example `wsl-toolkit-muse`;
- that base carries the tooling the agent needs: `grep`, `ripgrep`, `codegraph`,
  `podman`;
- the agent works on ONE Windows checkout, for example
  `C:\Users\USER\Downloads\some-repo`, which must look like a native Linux
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

## Amendment, 2026-09-12: the scripts left this tool, and the package map was driven

⭐ **The operator ruled three things after the first implementation**, and each
changed the shape of this entry rather than adding to it.

### 1. The common scripts were in the wrong place

`examples/common/bootstrap.sh` and `examples/common/tmux.conf` are now
[`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh) and
[`../scripts/common/tmux.conf`](../scripts/common/tmux.conf).

⚠ **Neither was ever about `wsl-toolkit`.** A general-purpose bootstrap under one
provider example's directory can only be found by somebody who already knows that
example exists. [`../docs/consumers.md`](../docs/consumers.md) carries the move as
a break, with the exposure window: both files existed at the old path for one
commit and were never in a release.

### 2. The hardcoded digests came out

The first version pinned CodeGraph **1.5.0** and three SHA-512 values written into
the file. ⛔ **The registry was on 1.6.0 the following day.** A pin that goes
stale in a day is a script that installs the wrong thing or refuses to run.

⭐ **Version and digest are now resolved at run time** from the registry, through
two calls that each answer one bare value so that `sh` needs no JSON parser:

```sh
npm view '@colbymchenry/codegraph' dist-tags.latest
npm view '@colbymchenry/codegraph@1.6.0' dist.integrity
```

⚠ **THAT IS A WEAKER CHECK AND THE FILE SAYS SO.** The digest now comes from the
same registry as the bytes, which proves transport and not authorship - the same
property [`../docs/consumers.md`](../docs/consumers.md) already records about this
repository's own `SHA256SUMS`. `--expect-integrity` and `--expect-sha256` put the
stronger check back for a caller who holds a value, and every run prints what it
resolved so a caller can become that.

### 3. The operator added BSD and the language toolchains

`bootstrap.sh` now knows **twelve** package managers rather than one: apk, apt,
dnf, emerge, pacman, tdnf, xbps, yum and zypper on Linux; `pkg` on FreeBSD and
DragonFly, `pkgin` on NetBSD, `pkg_add` on OpenBSD. `soar` and `nix` are used as
user-level providers for an account with neither root nor passwordless sudo, and
⛔ neither is ever installed, because installing either means piping a remote
script into a shell.

The `agent` toolset carries the languages the operator named: bash, Rust and
cargo, Go, Nim, Python and PowerShell. ⚠ **PowerShell is in the repositories of
three of thirteen images**, so it has an upstream route: the release tag comes
from the `releases/latest` redirect, and the tarball is verified against the
`hashes.sha256` the release publishes.

## What the drive found, and none of it was visible in the source

⭐ **Six defects, each found by running the thing rather than reading it.**

| what | why it mattered |
| --- | --- |
| ⛔ `awk` is absent on Photon, and so is `tr` | the table lookup was `awk -v want=...`, so Photon reported no row for ten logical names and installed nothing. The file now depends on the shell and the package manager and almost nothing else. |
| ⛔ `fail` inside `$( )` counts in a subshell | a run whose digest step never completed reported `failures=0` and exited 0. `fetch_verified_npm` answers through a global now. ⚠ This is the class the whole repository is built against: a guard that cannot make the process fail. |
| ⛔ `set --` clobbered the function's own arguments | the registry was asked for the integrity of `@` and answered nothing, which is what produced the silent success above. |
| ⚠ a bulk install that fails installs nothing and names nothing | six of twelve images failed over one absent package each and reported all eighteen as missing. One transaction first, then one package at a time, with the package manager's own message. |
| ⚠ PowerShell publishes its digests as UTF-16LE, CRLF, with a byte order mark | the run downloaded and hashed the tarball correctly, found no digest in a file full of them, and refused. |
| ⚠ `--version` is the wrong flag for `go`, `tmux`, `unzip` and `ssh` | the report printed four empty values beside tools it had just confirmed were on `PATH`. |

## Prove

⭐ **`wsl-toolkit matrix --images all`, the tool proving its own example.** The
`agent` toolset, which is 25 logical names including all six languages:

```text
13 ran, 2 failed, 0 unreached, 0 timed out, in 5m45s
alpine ok   arch ok       debian ok    debian12 ok  fedora ok
opensuse ok photon ok     rocky8 ok    ubuntu2204 ok
void-musl ok wolfi ok
chimera exit 1            gentoo exit 1
```

⛔ **Both failures are outside this script, and neither is worked around.**

- `gentoo`: the stage3 image carries no portage tree, so an install needs a sync
  this script will not start on a caller's behalf.
- `chimera`: its repository is momentarily inconsistent between
  `openssl3-3.6.0-r0` and `openssl3-devel-3.6.4-r0`, so `openssh` cannot be
  installed beside the build chain. ⭐ The run names that conflict now, which is
  what the loud retry was added for.

One image was driven to completion including the extra tool:

```text
alpine  requested=25  present=25  skipped=  absent=  codegraph=1.6.0  failures=0
        bash 5.3.9  cc 15.2.0  cargo 1.96.1  go 1.26.8  nim 2.2.0
        node v24.18.1  pwsh 7.6.1  python 3.14.7  rustc 1.96.1
        rg 15.1.0  fd 10.2.0  tmux 3.7c
```

PowerShell from the upstream release, on a distribution with no package for it:

```text
debian  powershell resolves to 7.6.6, powershell-7.6.6-linux-x64.tar.gz
        powershell tarball sha256 ddbc4a2d...adf103bc, abbreviated here because
        the tree refuses a long hex identifier in a published file
        powershell matches the digest its release publishes
        version.pwsh=PowerShell 7.6.6
```

⭐ **CI's own shellcheck, not this host's.** `ubuntu:24.04` in a container reports
**0.9.0**, which is the binary CI installs and two releases behind the 0.11.0 on
this machine. All 24 tracked scripts are clean under it.

## What is still open

⭐ **This list shrank after it was first written**, because the FreeBSD route was
found rather than assumed. The item is kept, as a measurement.

1. ⭐ **The FreeBSD path is driven, and the route that works is the consumer's
   own.** `bsd run --network` plus `fetch` of the raw URL puts the file in the
   guest; ⚠ `bsd run --script` flattens a script into one `;`-joined console line,
   so a 45 KB file does not survive it, and `bsd run -c` is bounded by the
   console's line length. Measured on FreeBSD 15.1:

   ```text
   freebsd on FreeBSD amd64, libc, privilege=root, provider=pkg
   pkg install -y bash ca_root_nss coreutils curl git jq node npm ripgrep tmux
   requested 18  present 10  skipped 8  absent 0  failures 0
   bash 5.3.15  curl 8.21.0  git 2.54.0  jq 1.8.2  node v24.19.0
   npm 11.18.0  ripgrep 15.2.0  tmux 3.7b
   ```

   The eight skipped names are the ones FreeBSD base already provides, and the
   report proves each: clang 19.1.7, less 692, OpenSSH 10.0p2, bsdtar 3.8.7,
   xz 5.8.3.

   ⚠ **`--toolset languages` filled the guest disk.** `go 1.25.14` and
   `python3 3.12.14` installed; `rust` and `nim` did not, and the kernel logged
   `pid (pkg) ... on /: filesystem full` on a 4.8 GiB root. ⭐ Both names are
   correct and were confirmed without installing: `pkg rquery` answers rust
   1.96.1 and nim 2.2.10. ⛔ **This is an image-size limit and not a table
   defect**, and a session that wants those two needs a larger guest disk.

   ⭐ **FreeBSD PACKAGES POWERSHELL, 7.5.5_1**, which the first table did not
   know. It is the fourth system that does, beside Alpine, Wolfi and Photon.
2. ⛔ **`pkgin` on NetBSD and `pkg_add` on OpenBSD are written and have never
   been run.** There is no image for either here, and the header says so rather
   than letting the twelve-manager count imply otherwise.
3. Nix as a user-level provider is written and not driven. Soar is also still in the
   script and is removed under the ruling below. Neither is on a catalogue image,
   and ⛔ this script may not install a user-level provider.
4. The provider-profile scenarios still are not in the main acceptance runner.
5. The Muse installer and an authenticated smoke still need operator access.

⚠ **`base.toolset = "developer"` in `provision.sh` is a SECOND package map**, in
the Go-embedded provisioner, and it still knows six families rather than twelve.
It is not merged with this one: the provisioner runs as root inside a distribution
during `base ensure` and installs the engine, and this runs as the ordinary
account afterwards. ⭐ **The duplication is real and is recorded here rather than
resolved**, because merging them means the Go module embedding a file that
consumers also fetch by URL, and that is a decision rather than a refactor.

## Amendment, 2026-09-13: reconciled against issues 30, 32 and 33

| item above | where it stands |
| --- | --- |
| 1, the FreeBSD path | driven. The disk limit it found is `WSL-72` |
| 2, `pkgin` and `pkg_add` never run | open |
| 3, the user-level providers | Soar removed and Nix driven, in the amendment below |
| 4, the provider-profile scenarios outside the acceptance runner | open |
| 5, the Muse installer and an authenticated smoke | closed by `WSL-69` |
| the second package map | `WSL-70` |

⚠ **Two premises of issue 30 moved.** The operator ruled on 2026-09-13 that
herdr replaces Zellij as the agents' multiplexer, which is `WSL-76`; tmux keeps
its generic configuration. And Muse is not Linux-only: Meta serves a Windows
installer, which `WSL-78` records.

Issue 30 is resolved by this entry and `WSL-68`. `WSL-70`, `WSL-71` and `WSL-72`
came out of the work on it. Issue 32 is `WSL-74` to `WSL-78`, and issue 33 is
`WSL-79`.

## Ruled by the operator, 2026-09-14: how item 2 closes

⭐ **Both managers are driven, and one that cannot be driven here is removed.**
`pkgin` runs in an official NetBSD image and `pkg_add` in an official OpenBSD
image, each booted under QEMU on this host from a download whose digest is checked
first, and both images are removed afterwards. A manager that cannot be driven on
this host leaves `scripts/common/bootstrap.sh`, and because that file is fetched by
URL the changelog says so.

## Ruled by the operator, 2026-09-14: remove Soar and keep Nix

⭐ **Nix is the one user-level provider.** Remove Soar from `bootstrap.sh`, its
arguments, its detection, its package route and the live documentation. Drive Nix
as an unprivileged account with neither root nor passwordless sudo. Keep the rule
that this repository does not install it. Historical measurements and reference
sweeps keep the names they measured.

## Ruled by the operator, 2026-09-14: Nix is loaded and set up, and never installed

⭐ **The operator asked that the bootstrap cover their own Nix setup, and do it
better**, and answered three questions in chat:

- **Installing:** "1, and do it all properly, and include all scripts in our repo,
  and test/setup flakes and sane defaults". Option 1 was: find an installed Nix
  that is not on `PATH` and load it without sourcing a profile, and when Nix is
  absent print the install command and exit 2. The logic lives in this repository
  and fetches no script.
- **`NIXPKGS_ALLOW_*`:** "always on, and also add some more if they make the
  experience better".
- **A GitHub token:** process only, when set: through `NIX_CONFIG`, written nowhere.

⚠ The operator's own `install_nix.sh` was read and not run. It removes `/etc/nix`,
`/nix` and the account's Nix state first, installs through `curl | bash`, appends
to `/etc/sudoers`, and on riscv64 sets `require-sigs = false` with two extra
substituters.

## Amendment, 2026-09-14: Soar is removed, and Nix is driven as an unprivileged account

⭐ **Built.** Soar leaves `bootstrap.sh`, its `--user-provider` values, its
detection, its install route and `scripts/README.md`; `--user-provider soar` exits
2 naming `nix none`. The Nix route:

1. `load_nix_environment` puts `~/.nix-profile/bin`, or the XDG state profile,
   `/nix/var/nix/profiles/default/bin` and `/run/current-system/sw/bin` on `PATH`
   when they exist, and sets `NIX_SSL_CERT_FILE` from the list Nix's own profile
   script reads. It sources nothing, and runs again after an install.
2. A name resolves through the shared table's new `nix` key, with no `os:` key:
   `bashInteractive`, `cacert`, `gcc,gnumake`, `openssh`, `gnutar`, `rustc`,
   `powershell`, and `-` for `npm` and `sudo`. `package_for` takes the provider and
   the system as optional arguments for it, so the system route reads the same
   globals as before.
3. `nix_route` answers `flakes` for a profile `nix profile` made or a new one, and
   `channels` for a profile with a `manifest.nix`, because `nix profile` rewrites a
   `nix-env` profile and `nix-env` then refuses it. Flakes install with `nix profile
   install --impure nixpkgs#ATTR`; channels with `nix-env -f '<nixpkgs>' -iA ATTR`,
   which also reads a NixOS account's channel.
4. `nix_run` gives every Nix command `NIXPKGS_ALLOW_BROKEN`, `_INSECURE`, `_UNFREE`
   and `_UNSUPPORTED_SYSTEM` as 1, `NIX_PAGER=cat`, and a `NIX_CONFIG` of
   `experimental-features = nix-command flakes`, `fallback = true`, `connect-timeout
   = 20`, `download-attempts = 5`, `max-jobs = auto` and `warn-dirty = false`, then
   `access-tokens = github.com=` and `GITHUB_TOKEN` when it is set, then the caller's
   own `NIX_CONFIG`, which wins.
5. `ensure_nix_flakes_config` appends `experimental-features = nix-command flakes` to
   the account's `nix.conf` when the file names no experimental features, and leaves
   a file that names them otherwise.
6. One transaction, then one attribute at a time with Nix's own message, as the
   system route does; `nix-channel --update` first on channels for a channel the
   account owns. The report carries `nix_route` and `nix_flakes_config`.

### ⛔ What driving found

The first drive, as an account reaching Nix through its daemon in
`docker.io/nixos/nix:latest`, installed logical names as attributes: `nix-env -iA
nixpkgs.ca-certificates` and `nixpkgs.tar` failed, and the report still read `present=4`
because the account's `PATH` reached root's profile. Every drive after it gave the
account a `PATH` of base tools with no Nix in it.

### The drives, all in `docker.io/nixos/nix:latest` with Nix 2.35.2 unless named

| drive | result |
| --- | --- |
| a new account, `--toolset minimal` with 13 more names, its `PATH` reaching no Nix | exit 0 in 24 s through flakes; 17 requested, 16 present, `npm` skipped; `nix.conf` gained the flakes line; the profile a `manifest.json` under `~/.local/state/nix/profiles` |
| the same run again | exit 0 in 10 s, `nix_flakes_config=present`, nothing failed |
| an account whose profile `nix-env` made | exit 0 in 9 s through channels, 4 of 4 present, `nix-env -f <nixpkgs> -iA cacert curl git gnutar` |
| a stand-in `nix` in `alpine:latest` recording what it was given | `nix profile install --impure nixpkgs#jq`, all four allow variables 1, `NIX_PAGER=cat`, the six defaults, the token as `access-tokens`, and the caller's `max-jobs = 2` last; the token in none of the run's own output |
| a copy with one row for nixpkgs' `hello-unfree`, as shipped and with `NIXPKGS_ALLOW_UNFREE=0` | shipped: installed; flag off: exit 1 and not installed |
| every attribute the agent toolset resolves, evaluated | 25 of 25 exist, from `bashInteractive` 5.3p15 to `xz` 5.8.3 |
| HEAD's `bootstrap.sh` and the tree's, `--dry-run --toolset agent`, on `golang:1.25` and `alpine:latest` | the resolved lines identical on both |

### Still open

1. **`pkgin` on NetBSD and `pkg_add` on OpenBSD**, as ruled. The operator approved in
   chat on 2026-09-14 downloading `NetBSD-11.0-amd64-live.img.gz`, 503,277,153 bytes
   from cdn.netbsd.org against its `SHA512`, and `install79.img`, 839,352,320 bytes from
   cdn.openbsd.org against `SHA256` and `SHA256.sig`. Both downloads were stopped part
   way at the session's checkpoint and removed. ⚠ QEMU 11.1.0 on this host has no
   `sga` device, so neither boot loader's screen reaches the serial console on its
   own; SeaBIOS's serial console or keys through QEMU's monitor are the next things to
   try.
2. **The provider-profile scenarios in the acceptance runner.**
3. The three reviews and the closing.

## Amendment, 2026-09-15: the provider profiles are cases in the acceptance runner

⭐ **Item 2 of the list above is built.**
[`../tools/windows/wsl-toolkit/acceptance.ps1`](../tools/windows/wsl-toolkit/acceptance.ps1)
builds `wsl-toolkit-accp` under a state directory of its own, grants it a checkout under
its `.tmp` scratch directory as the operator ruled, and drives the two profiles in
[`../tools/windows/wsl-toolkit/examples/common/access-profiles.md`](../tools/windows/wsl-toolkit/examples/common/access-profiles.md)
as five cases. They are behind `-Quick`, because they build a distribution, so a full run
carries 96 cases and `-Quick` still 89.

| case | what it reads |
| --- | --- |
| a provider profile mounts its one checkout read-write and nothing else of Windows | the probe exit 0 with automount and interop off, systemd, passwordless sudo and one `rw /workspaces/project` grant; then, as the account: that grant is the one DrvFS mount, a sentinel written on Windows reads back, a file written in the guest arrives on Windows, no drive is under `/mnt`, interop is absent, systemd is PID 1, the developer toolset's thirteen commands are present, `mount -t drvfs C:` is refused and `sudo -n true` is granted |
| a grant taken out of the profile is refused, then unmounted by base ensure | the probe exit 1 naming the stale mount; `base ensure` exit 0; the probe exit 0 with no grant, and the account reads no sentinel and writes nothing |
| passwordless sudo taken out of the profile is refused, then removed by base ensure | the probe exit 1 naming the account's sudo, `WSL-85`; after `base ensure`, `sudo -n true` is refused |
| a live grant mounts the checkout without a restart, and a revoke unmounts it | `base grant --json` answers live; the account reads and writes through the grant, and after `base revoke` reads nothing; PID 1's start tick is the same before and after, `WSL-75` |
| the profile base is removed with its disk | `base remove --yes` exit 0; `base status` answers not registered, and the disk is gone |

Every guest fact comes from one probe script the account runs through `base exec`, and
no case reads the configuration back as evidence of the guest.

⭐ **Measured on 2026-09-15:**

- the five alone, from a copy of the runner cut down to them: 5 of 5 in 105.1 s;
- ⛔ **the same five against a build with `WSL-85`'s check taken back out**: the sudo case
  red, `before ensure the probe exited 0`, and the live grant case red over an account
  still granted sudo, exit 1, so the cases fail on the defect they carry;
- the full runner on the tree's build: exit 0, 96 of 96 cases in 465.3 s from
  2026-09-15T01:33:26Z, leaving the same four distributions and no scratch directory.

⚠ **Driving the profiles by hand first found `WSL-85`**, filed and closed the same day,
and every expected line in the cases was read off that drive before it was written.

⭐ **For item 1, the boot loader's screen has a route.** With no disk, SeaBIOS's banner
and its boot messages reached `-serial stdio` under `-M q35,graphics=off`, and nothing
did under a plain `-M q35`, measured on 2026-09-15. The two downloads wait for the
operator's approval, asked in chat the same day.

## Amendment, 2026-09-15: asked a fourth time, and not answered

⛔ **`pkgin` and `pkg_add` did not move, and the reason is the only one.** The two
images were asked for again at the start of the session of 2026-09-15T06:16:51Z, in
chat and as a file, each named with its byte count, its mirror and the digest file it
would be checked against, and with an explicit alternative: a denial closes this entry
by REMOVING both managers from `bootstrap.sh` under the second half of the ruling
above, which is a complete outcome rather than a failure. Neither answer came.

⚠ **Nothing was downloaded and nothing was worked around.** That is four sessions.
Saying so plainly is the outcome, and this entry stays open carrying exactly one item
plus its closing reviews.

## Closing, 2026-09-15: NetBSD drove both arms, and OpenBSD is ruled enough

**Closed 2026-09-15.** ⭐ **The operator approved the two downloads on 2026-09-15,
after four sessions of asking**, and then ruled part way through the drive: "we
don't care about bsd that much, we already have one of them that works and that's
enough". ⛔ **That ruling is what closes this entry**, not a claim that both
operating systems were driven. What is true is better than it reads: **both
package-manager arms ran, because NetBSD has both.**

### The images, and what verifying them was worth

| file | bytes | checked |
| --- | --- | --- |
| `NetBSD-11.0-amd64-live.img.gz` | **503,277,153** | ⭐ SHA512 matched its published `SHA512` |
| `install79.img` | **839,352,320** | ⭐ SHA256 matched, **and `signify-openbsd -V` answered `Signature Verified`** |

⭐ **The OpenBSD signature is authorship, not transport, and that took one extra
step.** The `SHA256.sig` was checked against `openbsd-79-base.pub` fetched from
**`raw.githubusercontent.com/openbsd/src`** - a different origin from
`cdn.openbsd.org`, which served the file - and the two copies of the key are
**byte-identical**, SHA-256 `B7EE8E79…7DD7E4B1`. ⚠ Debian's independent
`signify-openbsd-keys` 2025.1 stops at **openbsd-78**, so it could not supply a
7.9 key; the GitHub mirror is what made this stronger than a same-origin check.
⭐ Both images, both overlays and the installed disk were removed afterwards,
about 6.5 GiB.

### ⭐ `pkgin`, driven

On NetBSD 11.0 amd64 booted under QEMU here, root over the serial console,
`bootstrap.sh` fetched into the guest over the driver's own TFTP and checked by
`cksum` against the tree's copy:

```text
provider=pkgin  toolset=developer
requested=18  present=9  skipped=build file less npm openssh procps tar unzip xz
absent=  failures=0        BOOTSTRAP EXIT 0
```

### ⭐ `pkg_add`, driven, on the same NetBSD with `pkgin` hidden

```text
provider=pkg_add  requested=5  present=4  skipped=tar  absent=  failures=0
bootstrap:   PKG_PATH set to https://cdn.NetBSD.org/pub/pkgsrc/packages/NetBSD/amd64/11.0/All
/usr/pkg/bin/jq                                       PKG_ADD EXIT 0
```

⚠ **So `pkg_add` is not an undriven arm**, which is what the 2026-09-14 ruling
was written to prevent. What is undriven is `pkg_add` **on OpenBSD**, where
`/etc/installurl` supplies the repository instead of `PKG_PATH`. OpenBSD 7.9 was
installed and booted here - shell after 64.5 s, `/usr/sbin/pkg_add` present,
`/etc/installurl` reading `https://cdn.openbsd.org/pub/OpenBSD` - and the operator
stopped it there.

### ⛔ Three defects in `bootstrap.sh`, none visible in the source

1. ⛔ **A stock NetBSD had no package manager at all, and the message named the
   one it was standing on.** `pkgin` is not installed on NetBSD 11.0; `pkg_add` is
   in base at `/usr/sbin/pkg_add`. The detection looked only for `pkgin`, so the
   run exited **2** with `no package manager found; looked for apk apt dnf emerge
   pacman pkg pkg_add pkgin tdnf xbps yum zypper and nix` - a list containing
   `pkg_add`, on a system where `command -v pkg_add` answers. ⭐ Fixed: NetBSD
   tries `pkgin`, then `pkg_add`. `pkg_add -I pkgin` installed pkgin in **13.5 s**.
2. ⛔ **NetBSD's `pkg_add` has no default repository and OpenBSD's has one.**
   With no `PKG_PATH`, `pkg_add -I jq` answered `no pkg found for 'jq', sorry.` and
   a five-name run reported **five failures and one name absent**. ⭐ Fixed:
   `netbsd_pkg_path` sets it from `uname -m` and `uname -r`, for NetBSD only, and
   ⚠ **leaves a `PKG_PATH` the caller already exported alone**.
3. ⛔ **The table had no NetBSD column, so it asked pkgsrc for what NetBSD base
   already provides.** `tar`, `procps`, `openssh`, `npm`, `file`, `less`, `unzip`
   and `xz` - four not in the repository at all, and a run therefore exited 1 with
   `absent=` **empty**, a report saying everything was present while failing. ⭐
   Fixed: eight rows gained `os:netbsd=-`, confirmed by `pkg_info -Fe` naming each
   as BASE rather than guessed.

### ⛔ And five in the scratch driver, which had never booted a guest

`.tmp\bsd67-driver` is untracked and stays that way; it booted its first guest
today, after these:

| what | the symptom it produced |
| --- | --- |
| ⛔ **both boot loaders take LF, not CR** | the typed line echoed and was never submitted, and the driver reported "no boot prompt" about a loader that was simply waiting. NetBSD's menu and its `boot>`, and OpenBSD's `boot>`, all three |
| ⛔ `-no-reboot` against a first-boot resize | the NetBSD live image resizes its own filesystem and reboots; QEMU exited and the driver reported "qemu exited before a login prompt" about a guest that was working correctly |
| ⛔ a prompt that ends in `)` | `System hostname? (short form, e.g. 'foo')` ends in a **parenthesis**, and the prompt test accepted only `?`, `]`, `:` or `>`. The install sat at that question with the rule for it unused in the table |
| ⛔ the success line checked only on exit | the installer's last answer is `reboot`, so with reboots allowed QEMU never exits; the loop read the boot loader's prompt as an unknown question and stopped to ask about a finished install |
| ⛔ `Set-StrictMode` against an omitted field | the result JSON omits `error` when there is none, so **every successful request failed in the client** after the guest had already run the command correctly |

⭐ **The installer's one refusal was correct and is kept.** Installing the sets
from the install media reached `Directory does not contain SHA256.sig. Continue
without verification? [no]`, and the driver refused. The sets now come from the
mirror over http, where the installer fetches `SHA256.sig` beside them and checks
every one against the key on its own media: `CONGRATULATIONS! Your OpenBSD install
has been successfully completed!`

### The three reviews

⭐ **1. The door sweep - what other door reaches this code?** `detect_provider`
has one caller and `netbsd_pkg_path` one, in `install_packages`'s `pkg_add` arm.
What the enumeration missed and grepping found:

- ⛔ **`detect_provider` lives INSIDE the shared package-table block**, so the
  NetBSD fallback is generated into `internal/toolkit/packages.sh` and reaches the
  base provisioner too. ⚠ Harmless - the provisioner runs in a Linux WSL
  distribution and never on NetBSD - but it is a change to an embedded file, and
  `check.sh package-table --fix` regenerated it in the same change.
- **`netbsd_pkg_path` is OUTSIDE that block**, so the export does not reach the
  provisioner. That is the correct side of the line: `PKG_PATH` is a bootstrap
  concern.
- **`--provider pkg_add` forces the arm on any kernel.** The guard is the kernel,
  not the provider, so forcing `pkg_add` on Linux sets no `PKG_PATH` and fails as
  it did before. A case covers it.
- ⚠ **Found, recorded, not acted on:** `PROVIDERS` still lists twelve names in one
  string used for both the refusal message and `--list-providers`. The message
  that misled here was correct about the list and wrong about the machine, and no
  change makes a list say what is installed.

⭐ **2. The guard mutation - can the new guards fail?** Each planted by hand in
`golang:1.25` with **`-count=1`**, because `go test` serves a cached result for a
shell file the build cache does not track:

```text
unmutated                                      exit=0  0 failing case(s)
planted: the base pkg_add fallback removed     exit=1  1 failing case(s)
planted: the kernel guard removed              exit=1  1 failing case(s)
planted: the callers own PKG_PATH ignored      exit=1  1 failing case(s)
restored                                       exit=0  0 failing case(s)
```

⭐ **The first plant was REFUSED, and that is the planter working.**
`if have pkg_add; then printf 'pkg_add'; return 0; fi` occurs **twice** - once in
the NetBSD arm and once in OpenBSD's - and `write-file.mjs replace --expect 1`
refused an ambiguous anchor rather than mutating the wrong arm. A hand-rolled
`sed` would have taken the first and reported a guard proved.

⭐ **3. The claim audit - which sentence is not backed by an artefact?**

- ⛔ **"Both managers are driven" would have been false as the 2026-09-14 ruling
  meant it**, and the closing above says so instead of eliding it: both *arms* ran,
  on one operating system, and `pkg_add` on OpenBSD did not.
- ⛔ **A first draft of the OpenBSD verification would have claimed a signature
  check that proved only transport.** The key and the file came from the same
  mirror; fetching the key from a second origin and comparing is what makes the
  sentence true, and Debian's independent key set stopping at 78 is why that was
  necessary.
- **The 13.5 s, 108 s and 64.5 s figures** are single measurements on one host
  under WHPX with one processor and 2 GiB, and they are quoted with those
  conditions rather than as properties of the systems.
- ⚠ **Not measured, and said so:** every `os:netbsd=-` row is justified by
  `pkg_info -Fe` answering BASE for that tool on NetBSD 11.0. `npm` is the one
  exception - it is marked `-` because it arrived with `node` from pkgsrc at
  `/usr/pkg/bin/npm`, not because base provides it.

### Still open

⛔ **Nothing, and one thing is deliberately not done.** `pkg_add` on OpenBSD is
undriven by the operator's ruling of 2026-09-15. ⚠ The 2026-09-14 ruling's second
half - remove a manager that cannot be driven here - is **not** invoked: both
managers were driven, so neither leaves `bootstrap.sh`.

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

## Decision

⭐ **Measured on 2026-09-14, before the question was put: every WSL distribution on
this host shares one network namespace.** Through `base exec`, `wsl-toolkit` and
`wsl-toolkit-podbox` both answered `net:[4026531833]` and `eth0` at the same
`172.23.102.192/20`. ⛔ So a firewall rule written inside a sealed base would change
the network of every distribution here, the podman machine and the operator's own
bases included.

What a sealed base's network is:

- **A. No network**: the sealed account's processes run in an empty network
  namespace of their own. Recommended, as the one door this tool can close without
  touching the shared namespace.
- **B. The internet only, with the host and private ranges refused.**
- **C. Left open**, reported open by the probe, and the base never called sealed.

⭐ **Ruled by the operator on 2026-09-14: B**, on the condition the question carried:
only if it can be done with no rule in the shared namespace. If it cannot, the work
stops and the operator is asked again.

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

## Amendment, 2026-09-15: the doors, attacked rather than read

⭐ **Approach step 1 is done: every door was TRIED, on a live zero-grant base**,
`wsl-toolkit-b68`, an arch base with `automount off`, `interop off`, no systemd, no
passwordless sudo, no grant, and the unprivileged account `sealed`. ⛔ Nothing below is
a setting read back. A door is OPEN because something got through, and closed because
the attempt was refused.

| door | attacked as the account | verdict |
| --- | --- | --- |
| Windows drives | no `/mnt/<letter>` in `/proc/mounts` | ⭐ closed |
| mounting one | `mount -t drvfs C:` answered `must be superuser to use mount` | ⭐ closed to the account |
| WSL's own driver mount | `/usr/lib/wsl/drivers` present, 9p, a write refused | ⭐ read-only |
| Windows interop | no `WSLInterop` handler, `cmd.exe` not found, `WSLENV` unset, no Windows directory on `PATH` | ⭐ closed |
| passwordless sudo | `sudo -n true` refused | ⭐ closed |
| the internet | `1.1.1.1:443` connected | ⛔ open |
| the Windows host | `445` and `3389` refused, and **ICMP answered** | ⛔ reachable |
| a private network namespace | `unshare -n` refused, `unshare -Un` SUCCEEDED | ⛔ available |
| ⛔ **the shared tmpfs** | wrote `/mnt/wsl`, and another distribution read it | ⛔ **open, both ways** |

### ⛔ The door this pass found, and it is not in the entry's list

**`/mnt/wsl` is one `tmpfs rw`, mounted `drwxrwxrwt`, shared by every distribution in
the WSL2 utility VM.** Measured on 2026-09-15, in both directions:

- the zero-grant base's unprivileged account wrote `/mnt/wsl/.wsl68-channel`, and
  `wsl-toolkit`, a different distribution, read its contents;
- `wsl-toolkit`'s root wrote a file there, and the zero-grant account read it.

⛔ **And uids are not namespaced across it.** The file written by uid 1000 in one
distribution lists as owned by uid 1000's name in the other - `sealed` in one listing
and `toolkit` in the other, for one inode. The sticky bit is the one thing that does
hold: the sealed account could not remove the other distribution's file.

⚠ **`/mnt/wsl/podman-sockets` is NOT a door, and the first reading of it was wrong.**
It holds `podman-root.sock` and `podman-user.sock` for `podman-machine-default`, and
both are **zero-byte regular files**, not sockets: `curl --unix-socket` against each
exits **7**, could not connect. A first pass reported them as answering, because the
check read a PIPELINE's status rather than curl's own.

### Closing the shared tmpfs: it works, it needs root, and it costs DNS

| question | measured |
| --- | --- |
| can it be unmounted in one distribution only? | ⭐ yes. As root, `umount /mnt/wsl` exit 0; `wsl-toolkit` still had it mounted with its contents |
| does the account then see it? | ⭐ no. A new session read 0 mounts, an empty directory, and a write refused |
| can the ACCOUNT unmount it? | ⛔ no, exit 32, and the mount stayed. It belongs in the provisioner, which has root |
| does it survive a restart? | ⛔ no. After `wsl --terminate` it was mounted again, so it is a boot-time action and not a one-off |
| ⛔ **what does it cost?** | **DNS.** `/etc/resolv.conf` resolves to `/mnt/wsl/resolv.conf`, so closing the tmpfs takes the resolver with it: a new session answered `getent hosts` exit 2. A sealed base has to write a real `/etc/resolv.conf` first, which WSL's `generateResolvConf = false` is for |

### The network, and the condition the ruling carries

⭐ **The ruling of 2026-09-14 is option B, "only if it can be done with no rule in the
shared network namespace".** What is measured:

- the shared namespace is `net:[4026531833]`, the same one `wsl-toolkit` and
  `wsl-toolkit-podbox` answered with, so a rule written there reaches every
  distribution on the host, the podman machine included;
- ⭐ **the account can make its own**, with no privilege: `unshare -Un` succeeded,
  and `pasta --config-net` put the probe in `net:[4026532318]` while the shared one was
  **unchanged** before and after. So a rule written inside it is not a rule in the
  shared namespace, and the ruling's condition CAN be met by this route;
- `pasta`, `passt` and `slirp4netns` are all present on the arch base, so nothing has
  to be fetched;
- ⛔ **but the flag alone does not refuse the host.** Inside `pasta --config-net` the
  internet answered (exit 0) and so did a ping to the Windows host; **`--no-map-gw`
  changed neither**. Refusing the host and the private ranges therefore needs a rule
  INSIDE the private namespace, which is exactly what the ruling permits and is the
  work that remains.

### Still open

⭐ **Step 1 of the Approach is complete and is the amendment above.** What is left, and
none of it needs the operator:

1. **A real `/etc/resolv.conf` and the tmpfs closed at every start.** Both belong in
   the provisioner, which already runs as root and already restarts the distribution.
2. **The account's processes in their own network namespace**, through `pasta`, with
   the Windows host and the private ranges refused by a rule inside that namespace.
   ⚠ This changes how `base exec` and `base shell` start a command, which is the
   invasive part and the reason it is named rather than begun.
3. **The probe as a command rather than a script.** The attack above lives in
   `.tmp\s16\seal-attack.sh` and answers in the shape this entry asked for; it is not
   yet a registered command with tests, mutation rows and a manual entry.
4. **The manual paragraph** saying what is NOT sealed, with these measurements beside
   it.

⛔ **Until all four are done the documentation calls this shape zero grants, never a
sandbox or a security boundary**, and the shared tmpfs above is the reason that
sentence is not merely caution.

---

## WSL-69. Muse Code installed, authenticated and driven end to end

**Source** the operator, 2026-09-12, ruling that this be a task of its own rather
than a line inside `WSL-67`. [Issue 30](https://github.com/Azathothas/ToolKit/issues/30)
part 1 is the ask it completes.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

⛔ **`examples/muse-code/README.md` documents an end-to-end procedure that nobody
has ever run.** It tells a reader to create a named base, grant one checkout,
bootstrap the tooling, install the provider CLI, authenticate, and work in the
granted directory. Every step before the CLI is driven and proved; the three that
are the point of the issue are not.

What a user sees today: a guide whose last third is written from what the provider
documents rather than from what happened.

## Premise

⭐ **Measured on 2026-09-12, so the gap is exactly three steps wide.** The base,
the grant, the toolset and the multiplexer are all driven, in `WSL-67`'s Prove
section. What has never run:

1. the provider's own installer, inside the base;
2. `muse login`, which needs a Meta subscription the repository does not hold;
3. Muse editing, committing and pushing the granted Windows checkout.

⚠ **Read rather than measured:** that the installer at the documented URL works on
a base built by this tool. Its documentation is behind a Meta login, so nothing in
this tree has seen it.

## Approach

1. ⭐ **Operator-assisted, and the entry says which parts need them.** The install
   and the authenticated smoke need their credentials; everything around those two
   is this session's.
2. Build the base from the one-checkout profile in
   [`../tools/windows/wsl-toolkit/examples/common/access-profiles.md`](../tools/windows/wsl-toolkit/examples/common/access-profiles.md),
   with `--instance muse`, against a **throwaway** git checkout rather than a real
   one, so the first write test cannot damage anything.
3. Run [`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh) with
   `--toolset agent` as the ordinary account.
4. ⛔ **The installer is downloaded, read, and then run from the file that was
   read.** Never a URL piped into a shell from a script in this tree;
   [`../docs/security/remote-ops.md`](../docs/security/remote-ops.md) and
   [`../docs/consumers.md`](../docs/consumers.md) both refuse it. Record the
   resolved version and a digest of the file that was actually run, so the guide
   can name what it was proved against.
5. Drive the three things the issue asks for, from inside a tmux session: Muse
   reads the checkout, writes a file that appears on the Windows side, and `git
   commit` plus `git push` succeed from the guest.
6. ⛔ **Assert the negative in the same pass.** Muse must not reach any other
   Windows directory. Name three paths outside the grant and show each is absent.
7. Rewrite the guide's last third from what happened, and delete anything in it
   that measurement contradicts.

⛔ **Do not widen the grant to make a step work.** If Muse needs a second
directory, that is a finding about the access model and belongs in `WSL-67`, not a
second mount added quietly to get a green run.

## Consumers

None. `examples/` is read by people, not fetched by a script, and no row of
[`../docs/consumers.md`](../docs/consumers.md) names it. ⚠ A change to
`scripts/common/bootstrap.sh` discovered while doing this is a different matter and
does reach that file's own unregistered callers.

## Prove

⛔ **The acceptance, and it is a command.** Run from the granted checkout inside
the base, as the ordinary account:

```bash
sh -c 'muse --version && git -C /workspaces/project commit --allow-empty -m "muse smoke" && git -C /workspaces/project push && ls /mnt/c 2>&1; echo "rc=$?"'
```

Passing is all four of:

- `muse --version` prints a version and exits 0;
- the commit and the push both exit 0, and the commit is visible from Windows;
- `ls /mnt/c` fails, because `automount` is off and interop is absent;
- `wsl-toolkit --instance muse base status --probe --json` reports exactly one
  grant, whose source is the throwaway checkout.

⚠ **Read each exit code from the process that produced it, unpiped.** A `&&`
chain reports the last one, which is the trap
[`../docs/AGENTS.md`](../docs/AGENTS.md) absolute 5 names.

## Amendment, 2026-09-13: the base the guide needs, and the mask that locked it out

⭐ **The operator ruled six things on 2026-09-13**, first written down in an issue
30 comment, and each changed what this entry builds rather than adding a step.
This is their home in the tree.

| ruling | what it became |
| --- | --- |
| Muse is a trusted agent with high authority | `base.passwordless_sudo`, off by default. The provisioner writes one tool-owned `/etc/sudoers.d/wsl-toolkit-<user>` rule, `USER ALL=(ALL:ALL) NOPASSWD: ALL`, validates the candidate with `visudo -cf` before an atomic move, and removes only that file when the setting goes off |
| its account is `muse`, not `toolkit` | `base.user` is the one managed account of a named instance. It owns a 0700 home with persistent `~/.config`, `~/.cache`, `~/.local/share` and `~/.local/state`, and the verifier proves each is writable |
| agents with different authority never share a distribution | changing `base.user` on an existing instance is refused with the command that does it, `wsl-toolkit base recreate`, rather than migrated in place |
| a low-authority agent gets an instance of its own | the zero-grant profile in [`access-profiles.md`](../tools/windows/wsl-toolkit/examples/common/access-profiles.md) is named `malaria`, with no sudo and no configured Windows path |
| Zellij 0.45.1 replaces tmux for Muse | tmux stays in the toolsets and keeps its generic configuration. Zellij was chosen for background sessions, stable pane ids, `list-panes --json`, targeted `write-chars` and `send-keys`, and `dump-screen` |
| the agent reaches the base through one seam | `wsl-toolkit base exec`: exactly one of `-c` or `--script`, the configured account's home unless `--dir` names an absolute guest path, a 30 minute default deadline, `--root` as an explicit variant, the payload on stdin, and the guest's exit status forwarded |

⚠ **Passwordless sudo is authority, not containment.** Guest root can mount a
Windows path the configuration never granted, and both pages that name the
setting say so.

### ⛔ What the first live build would have hit

The rulings were implemented and unit-tested in the first session of the day and
never built live, because the exact sudoers line had not been approved yet.
Reading that work in the second session found three defects and a published
fingerprint:

1. ⛔ **The mask taken for the sudoers candidate leaked into the rest of the
   provisioner.** `umask 077` ran at the top level of `provision.sh`, so every
   later file and directory inherited it. Replayed on Arch with GNU mkdir 9.11,
   in an ephemeral container, in the order the provisioner runs:

   | | sudo off | sudo on |
   | --- | --- | --- |
   | `/etc/fstab` | 644 | 600 |
   | `/workspaces` | 755 | 700 |
   | the unprivileged account reaches `/workspaces/project` | yes | no |

   The rebuild the rulings require would have failed verification over a mount
   that was present. Its unit test asserted only that strings exist in the
   script, which is why it stayed green. ⭐ Fixed at both ends: the provisioner
   sets `umask 022` itself, the candidate takes `umask 077` inside a subshell,
   and `TestTheProvisionerScopesEveryRestrictiveUmask` refuses any other shape.
2. ⚠ **The verifier called an unreachable mount absent.** `[ -d ]` over a target
   whose parent the account cannot enter fails exactly as a missing mount does.
   It now walks the path from the root and names the directory it cannot enter.
3. ⚠ **`base exec` discarded a failure to start `wsl.exe`** and exited 2 with no
   reason, which reads exactly like a guest script that ran `exit 2`. The guest's
   own status is still forwarded silently; every other failure is returned.
4. `examples/common/zellij.md` published an absolute Windows home path carrying a
   username, three times. It names `%LOCALAPPDATA%` now. The gate did not see
   it, and `TOOL-25` is why.
5. ⚠ **The verifier reported a warning as the engine version.** Found while
   re-provisioning the ordinary base: podman wrote a stale `pause.pid` notice to
   stderr after the restart, `podman --version 2>&1` merged it, and the base's
   engine field read `pause.pid file refers to PID 47 ...`. It reads stdout alone
   now.

⚠ **Verification now asks more of every base, so an older one reports unusable
until it is re-provisioned.** Measured on the ordinary `wsl-toolkit` base built
2026-09-10: `base status --probe` answered `does not own a writable XDG directory
at /home/toolkit/.cache`, and `base ensure` re-provisioned it in place in 13 s
and answered healthy. [`../docs/consumers.md`](../docs/consumers.md) carries it.

### What is proved so far

⭐ **Every guard this work added was planted and went red**, nine rows through
`repo mutate --only`: the guest-home default, the account rebuild refusal, the XDG
directories, the sudoers validation, the scoped umask, the provisioner's own
mask, the unreachable-parent message, the start-failure error, and the engine
version read from stdout alone.

⭐ **Driven on the throwaway base `wsl-toolkit-muse`**, from the one-checkout
profile with `user: muse` and `passwordless_sudo: true`:

- against the base still built for `toolkit`, `base status --probe` exited 1 and
  `base ensure` exited 2, both naming `wsl-toolkit base recreate`, and
  `/etc/fstab`, `/etc/wsl.conf` and the account list were unchanged afterwards;
- `base recreate --yes` rebuilt it in 71.7 s and verification passed as `muse`;
- as `muse`: the account's own home, all four XDG directories under it writable,
  `sudo -n true` exit 0, umask `0022`, `/workspaces` 755, `/etc/fstab` 644, `/etc/subuid` 644,
  and the sudoers fragment 440 inside a 750 `/etc/sudoers.d`;
- `base exec -c 'exit 7'` exited 7 with no error line, and a relative `--dir` was
  refused with exit 2;
- ⭐ the live plant: `chmod 700 /workspaces` made `base status --probe` report
  `this account cannot enter /workspaces, so explicit mount /workspaces/project
  is unreachable`, and `chmod 755` gave back a healthy base;
- Zellij 0.45.1-1 from pacman, then `bootstrap.sh --toolset agent --without
  nim,powershell,tmux --no-tmux-config` as `muse`: requested 22, present 21,
  `cargo` skipped because `rust` provides it, absent none, CodeGraph 1.6.0,
  failures 0;
- a commit made from the guest and pushed to the checkout's local bare remote is
  visible from Windows, and CodeGraph indexed the committed program: 1 file,
  4 nodes, 5 edges.

⚠ **Measured in the first session and not re-run in the second:** a native
Windows Zellij 0.45.1 client completed an authenticated attach to the WSL 0.45.1
server over `http://127.0.0.1:8082`, and its token was revoked afterwards. The
same version refused `zellij web --create-token --token-name NAME` as a mutually
exclusive pair.

⛔ **Still open at this checkpoint:** the operator's two Meta steps, installing
the CLI from a saved and inspected file and running `muse login`; the agent
driving Muse to read, write, commit and push; the three negative paths; and the
acceptance command above. All four are answered in the closing below.

---

## Closing

**Closed 2026-09-13T04:19:42Z.** The operator installed Muse and signed in; the
agent drove everything else.

⭐ **The operator's two steps**, from the operator's own terminal and read back
off the base afterwards. `install.sh` was 314 lines and 9,314 bytes, SHA-256
`5196d820…632a0ca` (shortened, because this tree refuses a long hex identifier),
and it installed Muse Code 1.1.1 (`1.1.1-R2514.1`, a 273 MB download) into `~/.local/bin` with no
root. `muse login` was a device-code sign-in and saved its credential at mode 0600
in the persistent home; its contents were never read. ⚠ **The `less` review step
does not appear in the operator's transcript**, so the file was read after it ran
rather than before: it fetches a launcher from `api.meta.ai` and checks a SHA-256
only when the server advertises one, which proves transport and not authorship.

⭐ **Muse, driven headless by the agent** through `base exec` with `muse exec
--json --workspace /workspaces/project --approval-mode never`: it read
`src/inventory.py`, ran it, wrote `MUSE_SMOKE.md`, committed `65c47fd`, and pushed
to the checkout's local bare remote, in 52 seconds with exit 0. The file's two
lines were `muse-smoke-ok` and `47`, matching the program's own output.

⭐ **Muse's interactive screen, driven by the agent** through Zellij's CLI in the
same base: `new-pane` answered `terminal_1`, `list-panes --json` showed Muse
running in `/workspaces/project`, and after `write-chars` and `send-keys ENTER`,
`dump-screen` read back `Ran command ... ✓` and `47` ten seconds later. ⚠ The
first start in a checkout asks whether to trust the workspace, and a question typed
before that is answered is dropped rather than queued. The guide says so.

⛔ **The negative pass found a sixth defect, and it is why `ls /mnt/c` could not
fail.** Muse reported `ls /mnt/c` and `ls /mnt/d` succeeding with empty output. As
the account and as root: nine empty 0777 directories, `c d f l m p r t y`, one per
host drive letter, created at the fresh import's first start, before provisioning
wrote `automount off`. None was a mount. The provisioner now leaves `/`, unmounts a
live drive mount when automount is off, and removes the empty point; the verifier
refuses one that is still there. Proved three ways: three mutation rows went red;
`base ensure` on the Muse base refused `/mnt/c exists even though automount is
off`, re-provisioned in place with `drive mount points removed: 9`, and was healthy
after the restart in 20.6 seconds; and WSL did not create them again. The Muse
install, the credential and the checkout were untouched by it.

The acceptance command, run from the granted checkout as `muse` through `base exec
--dir /workspaces/project`, with `PATH` from the profile the installer wrote:

```text
To .smoke-remote.git
   65c47fd..02d68aa  main -> main
Muse Code 1.1.1 (1.1.1-R2514.1)
[main 02d68aa] muse smoke
ls: cannot access '/mnt/c': No such file or directory
rc=2
```

⚠ **The `&&` chain reports only its last status, so each part was read from its
own process as well.** The two full commit identifiers were printed and were
equal; they are shortened below for the same reason as the SHA-256.

```text
muse --version rc=0
HEAD subject: muse smoke
## main...origin/main
local HEAD  02d68aa
remote main 02d68aa
pushed rc=0
ls /mnt/c rc=2
ls /mnt/d rc=2
ls /mnt/c/Windows/System32 rc=2
cmd.exe on PATH rc=1
```

From Windows, `git log` over the throwaway checkout and its bare remote both show
`02d68aa muse smoke` above `65c47fd`, and `MUSE_SMOKE.md` reads `muse-smoke-ok`,
`47`. The probed status:

```text
probe exit=0
healthy=True user=muse passwordless_sudo=True automount=off interop=off grants=1 problems=0 engine=podman version 6.1.1
grant: rw /workspaces/project <- the throwaway checkout under .tmp/wsl69-e2e/project
```

All four pass conditions hold: the version exits 0; the commit and the push exit 0
and are visible from Windows; `/mnt/c` fails; and the probe reports exactly one
grant, to the throwaway checkout.

---

## WSL-70. Two package maps become one, and the Go module embeds it

**Source** the operator, 2026-09-12, ruling **one shared table** on the fork
recorded in `WSL-67`'s amendment.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** done

---

## Problem

⚠ **The same knowledge is written twice and the two copies already disagree.**
[`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh) knows twelve
package managers and every distribution-specific spelling measured on 2026-09-12.
`tools/windows/wsl-toolkit/internal/toolkit/provision.sh` knows six families and a
`developer` toolset written by hand. A distribution whose package name changes has
to be fixed in two places, and nothing fails if only one is.

## Premise

⭐ **Measured, not assumed:** the provisioner detects `apk`, `pacman`, `apt`,
`dnf`, `tdnf` and `xbps` and nothing else, and its `developer` list is a literal
per-family `case` arm. The bootstrap's table already carries every value those arms
carry, plus `yum`, `zypper`, `emerge` and the three BSD managers, plus the
`os:<ID>` overrides that the `case` form cannot express at all.

⚠ **The two are not interchangeable, and that is why this is L rather than S.**
The provisioner runs as root inside a distribution during `base ensure` and
installs the container engine; the bootstrap runs as the ordinary account
afterwards and installs a tool set. Merging the TABLE is the work. Merging the
SCRIPTS is not, and must not be attempted.

## Approach

1. ⭐ **The table moves into one file and both readers read it.** The seam is
   `package_table()` in `scripts/common/bootstrap.sh` and the per-family `case`
   arms in `internal/toolkit/provision.sh`.
2. ⚠ **`go:embed` cannot reach outside its own package directory.** This
   repository already solved that shape once: `RULES.md` section 4 describes
   `wsl-toolkit.ps1` being built into two tracked copies with a gate rule that
   rebuilds and compares both byte for byte. ⭐ **Follow that precedent rather
   than inventing a second one**: the embedded copy is GENERATED, and a `check`
   rule refuses the two disagreeing.
3. The provisioner keeps its own engine packages. Only the tool-set names come
   from the shared table.
4. Extend the provisioner's detection to the same twelve managers, and ⛔ **say in
   its own header which of them have had a base built from them.** Four presets
   exist; twelve managers do not mean twelve proved bases.

⛔ **Do not make the provisioner fetch the bootstrap at run time.** A base build
that needs the network for its own package map is a base that cannot be built
offline, and it puts a consumer-facing URL on the critical path of the tool.

## Decision

**Ruled by the operator on 2026-09-12: one shared table.** The alternative,
leaving both and recording the risk, was rejected. ⚠ **The cost the ruling
accepts, written down so it is not rediscovered:** that file is fetched by URL
from outside this tree, so a change to it now reaches the compiled tool AND those
callers in one commit, and the gate has to hold both ends.

## Consumers

⭐ **This is the row that matters.**
[`../docs/consumers.md`](../docs/consumers.md) records `scripts/common/bootstrap.sh`
as meant to be fetched with no consumer row yet. After this entry the file is also
a build input to the published executable. Not breaking by that page's definition:
no path moves and no exit code changes meaning. ⚠ But it raises the cost of every
later edit to the table, and the entry's closure says so.

## Prove

```bash
sh scripts/common/check-gate.sh
```

Passing is:

- a new gate rule that regenerates the embedded table and compares it byte for
  byte, and that goes RED when one copy is edited alone. ⛔ Plant that edit and
  read the exit code, unpiped: a guard never seen to refuse is a guard nobody
  knows works;
- `wsl-toolkit base ensure` still builds from all four presets;
- `wsl-toolkit matrix --images all` with `--toolset agent` is no worse than the
  13 ran / 2 failed measured on 2026-09-12.

---

## Amendment, 2026-09-13: one home and a checked copy, and nothing reads the copy yet

⭐ **The first half is built: the table has one home, and a gate rule holds its
copy.** In [`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh), the
block between `# >>> shared package table: begin` and `# <<< shared package table:
end` carries `package_table`, `toolset_names`, `have`, `split_on`,
`commas_to_spaces`, `in_list`, `package_row`, `package_for`, `PROVIDERS`,
`detect_provider` and `detect_os_id`. `USER_PROVIDERS`, `detect_user_provider` and
`first_line` moved below the end marker, because nothing inside needs them.

`tools/check` gains `package-table`, the gate's twentieth check. It regenerates
`tools/windows/wsl-toolkit/internal/toolkit/packages.sh` from the block under a
generated-file banner and compares byte for byte. `check package-table --fix`
rewrites the copy, and `--fix` is refused for any other check and for the gate.
[`RULES.md`](RULES.md) section 4 names it as the third generated file.

⚠ **The copy did not pass ShellCheck on its own at first.** The block reads the
caller's `OS_ID` and `PROVIDER` and sets `PROVIDERS` for the caller, so alone it
drew SC2153 and SC2034. Two targeted directives inside the block carry the reason.

⭐ **Proved:**

- a copy edited alone and a block edited alone each made `check package-table`
  exit **1**, read unpiped, naming line 49; both restored to exit 0;
- three mutation rows went red: the byte comparison, a doubled begin marker, and
  an end marker before its begin;
- ⭐ **the block move changes nothing a caller of the fetched file sees.** HEAD's
  `bootstrap.sh` and the tree's gave byte-identical `--help`, `--list-names`,
  `--list-providers`, `--dry-run --toolset agent --codegraph none --json` and
  `--dry-run --toolset developer --codegraph none` output, with the same exit
  codes, on alpine 3.22 and debian 13. ⚠ debian's JSON dry run exits 1 under both;
  the cause was not read.

⛔ **Not done, and this is most of the entry:**

1. the provisioner does not read the copy. `base.go` does not embed
   `packages.sh`, and `provision.sh` still carries its per-family `case` arms;
2. the provisioner still detects six managers, and its header does not yet say
   which have had a base built from them;
3. `base ensure` from all four presets, and `matrix --images all --toolset agent`
   against 13 ran and 2 failed. Neither was run.

---

## Amendment, 2026-09-15: the provisioner reads the table, and two presets do not build

⭐ **Items 1 and 2 of the list above are built.**

- `base.go` embeds `packages.sh` and provisioning sends it ahead of `provision.sh`, through
  `provisionRequest`, the one place the run is assembled.
- `provision.sh` detects its package manager with the table's `detect_provider` and reads
  `detect_os_id`, so it knows the same twelve managers. It installs a container engine
  through apk, apt, dnf, pacman, tdnf and xbps, as before, and refuses emerge, yum and zypper
  by name. ⚠ Its header says a base has been built from four of the twelve.
- The `developer` section resolves `bash build curl git jq node npm openssh ripgrep tmux
  unzip` through `package_for`, and names a package this system does not carry. The commands
  it promises and verifies are unchanged.
- The generated banner, `bootstrap.sh`'s comment above the block, `RULES.md` section 4,
  [`../docs/consumers.md`](../docs/consumers.md) and the scripts README say the provisioner
  reads the copy.

⭐ **The table gives the four presets what their lists installed, byte for byte.**
`TestTheProvisionerResolvesItsDeveloperPackagesThroughTheSharedTable` runs the provisioner's
own developer section in `/bin/sh` after the table, with each manager stood in for:

| family, system | the install |
| --- | --- |
| apk, alpine | `apk add --no-cache bash build-base curl git jq nodejs npm openssh-client ripgrep tmux unzip` |
| pacman, arch | `pacman -S --noconfirm --needed bash base-devel curl git jq nodejs npm openssh ripgrep tmux unzip` |
| apt, debian | `apt-get install -y -qq --no-install-recommends bash build-essential curl git jq nodejs npm openssh-client ripgrep tmux unzip` |
| dnf, fedora | `dnf -y --setopt=install_weak_deps=False install bash gcc gcc-c++ make curl git jq nodejs npm openssh-clients ripgrep tmux unzip` |
| tdnf, photon | `tdnf install -y bash gcc make curl git jq nodejs openssh-clients tmux unzip`, naming npm and ripgrep as not carried |
| xbps, void | `xbps-install -Sy bash base-devel curl git jq nodejs openssh ripgrep tmux unzip`, naming npm |
| apk, chimera | `apk add --no-cache bash base-devel curl git jq nodejs openssh tmux unzip`, naming npm and ripgrep |

⚠ **The last three differ from the lists they replace**, which had never built a base:
they are the table's values, measured on those images on 2026-09-12.

⭐ **The gate rule holds both ends.** With the new banner, a copy edited alone and a block
edited alone each made `check package-table` exit 1 naming line 50, read unpiped, and the
restored files exit 0.

### ⛔ Two presets do not build, and neither is this change

`base ensure` with `toolset developer`, automount and interop off, on throwaway instances
under this repository's `.tmp`, on 2026-09-15:

| preset | answer |
| --- | --- |
| arch | exit 0 in 51.5 s; `package manager: pacman on arch`; the probe healthy; all thirteen developer commands present |
| alpine | exit 0 in 44.2 s; `package manager: apk on alpine`; healthy; all thirteen present |
| debian | exit 2 in 123.2 s: the developer packages installed, then the QEMU binary-format installer's rootful run answered `netavark: nftables error: unable to execute nft: No such file or directory` |
| fedora | exit 2 in 97.8 s: built, then verification answered `a container did not run as agent (exit 125)`, naming podman's shared-mount warning |

- ⭐ **debian fails the same way before this change.** `HEAD`'s build, `4c573cf`, with no
  toolset, answered the same `nft` error in 33.6 s. `base presets` still carries its
  2026-09-09 figure.
- ⭐ **fedora's own error, read with a diagnostic build that printed verification's whole
  stderr:** `newuidmap: write to uid_map failed: Operation not permitted` and `cannot set up
  namespace using "/usr/sbin/newuidmap": should have setuid or have filecaps setuid`. The
  working arch base's `newuidmap` carries `cap_setuid=ep`; the imported Fedora rootfs's does
  not. Fedora was never built on this host before.

Both are proposed to the operator as `WSL-86`, and this entry's prove waits on it.

### The agent matrix, with and without CodeGraph

`matrix --images all --workspace . -c 'sh /work/scripts/common/bootstrap.sh --toolset agent ...'`,
on 2026-09-15:

| run | answer |
| --- | --- |
| `--codegraph none` | 13 ran, 2 failed, 0 unreached, 0 timed out, in 340.6 s: chimera, `apk could not install openssh`, and gentoo, with no portage tree. ⭐ The same two as on 2026-09-12, in 5m45s |
| CodeGraph latest, the toolset's default | 13 ran, 7 failed, in 517.3 s: chimera and gentoo; debian, debian 12, ubuntu 22.04 and void-musl with every package present and then `npm did not write exactly one archive into ` and an empty directory; rocky 8, whose npm 6.14.11 answered `npm could not fetch` |

⚠ **The 2026-09-12 figure does not name its command.** The run beside it in `WSL-67` says
one image was driven "including the extra tool", so the comparison is made without
CodeGraph.

⛔ **The CodeGraph failures are `bootstrap.sh`'s and not the table's.** Under dash,
measured in `debian:latest`, `fetch_verified_npm` sets its directory to an empty string,
where busybox and bash keep it. Filed as `WSL-87`, approved by the operator in chat on
2026-09-15.

### Still open

1. `base ensure` from all four presets, which waits on `WSL-86`.
2. The three reviews and the closing.

---

## Closing

**Closed 2026-09-15T05:07:50Z.** The table has one home in
[`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh), a gate rule holds the
generated copy byte for byte, and the base provisioner reads that copy for its detection
and its `developer` names. The two items the amendment of 2026-09-15 left open are done:
`base ensure` builds from all four presets, and the agent matrix is no worse than the
figure the entry set.

⚠ **The two presets that did not build were not this entry's doing, and proving that took
a separate entry.** `WSL-86` carries the `nft` package and the dropped id-mapping
capability, and its closing carries the four builds. This entry's prove is met by them.

⭐ **The prove's three conditions, each measured:**

| condition | measured |
| --- | --- |
| a gate rule that regenerates the embedded table and compares it byte for byte, and goes RED when one copy is edited alone | `check package-table`, the gate's twentieth check. A copy edited alone and a block edited alone each made it exit 1 naming the line, read unpiped; both restored, exit 0. Three mutation rows hold it |
| `base ensure` still builds from all four presets | arch, alpine and debian exit 0 with `toolset developer`, fedora exit 0 with `toolset none`, each probe healthy. `WSL-86`'s closing carries the table, the times and the conditions |
| `matrix --images all` with `--toolset agent` no worse than 13 ran / 2 failed | 13 ran and 3 failed with CodeGraph on, and 13 ran and 4 failed on a second run whose extra failure is fedora's mirror timing out. The comparison figure is below |

⚠ **The 2026-09-12 figure this is measured against was taken with `--codegraph none`, and
that is the comparison that holds.** The run beside it in `WSL-67` says one image was
driven "including the extra tool", so the 13 ran and 2 failed is the no-CodeGraph shape.
⛔ **It was not re-measured this session, and the reason is readable rather than assumed:**
`bootstrap.sh` calls `install_codegraph` only inside `if [ "$CODEGRAPH" != none ]`, and
`npm_packs_to_a_directory` and `fetch_verified_npm` have no other caller, so a
`--codegraph none` run executes none of this session's changes to that file. The
byte-identical `--help`, `--list-names` and `--list-providers` comparison in `WSL-87`'s
closing is the measured half of the same statement.

### The three reviews

⭐ **1. The door sweep - what other door reaches the shared table?** Three readers, and
the third is the one an enumeration from memory leaves out:

- `bootstrap.sh` itself, which every caller fetching the file by URL runs;
- the base provisioner, through the generated `packages.sh` that `base.go` embeds;
- ⛔ **`fetch_verified_npm`, which is not about packages at all.** It calls `split_on`
  from inside the block to name its npm work directory, so a change made to `split_on`
  for the table's sake changes a path in the codegraph install. Found by `WSL-87`'s door
  sweep and recorded in `PROGRESS.md`; nothing is built on it.

The gate rule is the guard on the first two agreeing, and `check package-table --fix` is
the one way to move the copy.

⭐ **2. The guard mutation - can the gate rule fail?** Proved twice, at each end and
after the generated banner changed: a copy edited alone and a block edited alone each
made `check package-table` exit 1 naming the line, read unpiped, and the restored files
exit 0. Three mutation rows hold the byte comparison, a doubled begin marker and an end
marker before its begin.

⛔ **What this lens could NOT prove, said rather than implied:** that the provisioner's
own engine packages are right, because they are deliberately not in the table.
`WSL-86` found two of them wrong, on the two presets nobody had built.

⭐ **3. The claim audit.** The entry's ruling accepted a named cost: an edit to the table
now reaches the published executable AND every caller of the fetched file in one commit.
That is true today and the record says so in three places -
[`RULES.md`](RULES.md) section 4, [`../docs/consumers.md`](../docs/consumers.md) and the
scripts README - each of which was re-read against the tree rather than assumed.

⚠ **The comparison this entry's prove asks for is not like with like unless it is
stated.** The 2026-09-12 figure of 13 ran and 2 failed does not name its command, and the
run beside it says one image was driven "including the extra tool". The comparison is
therefore made with `--codegraph none`, and both numbers are recorded with their flag.

---

## WSL-71. A portable shell profile this tree owns

**Source** the operator, 2026-09-12: write a portable proper one here, as a task
for a later session.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** M, **Status** done

---

## Problem

A base built by this tool gives an interactive account a bare default shell.
Everything the operator relies on daily is in a `.bashrc` that lives in another
repository and re-fetches itself from the network, which a guest with `interop`
off and no configured egress may not be able to reach, and which
[`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh) is forbidden
from pulling into a shell.

## Premise

⭐ **Read on 2026-09-12, and recorded in sweep 2 of
[`../docs/reference-sweeps/findings.md`](../docs/reference-sweeps/findings.md).**
The operator's own file is 19,433 bytes of prompt, colours, aliases, functions and
a self-update path. Three of its mechanisms were already taken into the
bootstrap's detection; the rest is interactive-shell configuration this tree has
none of.

⚠ **The one-home rule was the objection, and the ruling overrides it with a
distinction:** a file this tree OWNS and can keep portable is not a second copy of
the operator's file. So the thing to write is not a copy.

## Approach

1. ⭐ **Portable first, and that is the whole constraint.** It must work under
   `bash`, and degrade rather than error under `dash`, `ash` and FreeBSD `sh`,
   because those are what the thirteen catalogue images and the BSD guest actually
   run. ⛔ `shellcheck -s sh` runs over every tracked `.sh` here, so a `.sh` file
   cannot use arrays, `local` or `[[`. Decide the file's extension with that in
   mind and say why in its header.
2. ⭐ **The WSL guard is the one mechanism worth carrying over**, because it is
   the one that is about this tree's own subject: a shell whose working directory
   is under `/mnt/c` is slow and, in a base with `automount` off, wrong. Move to
   the home directory and say so once.
3. `PATH` for `~/.local/bin`, which the bootstrap already writes to `~/.profile`;
   this file reads it rather than restating it.
4. ⛔ **No self-update path.** A profile that fetches and overwrites itself is the
   one thing in the reference this repository cannot have: it makes the file's
   content untrackable and puts a network fetch in every shell start.
5. ⛔ **No aliases for tools the toolsets do not install**, and no prompt colour
   scheme. An alias to a missing program is an error on every shell start, and a
   prompt is taste rather than a tool.
6. `bootstrap.sh` installs it the way it installs `tmux.conf`: from beside itself,
   and it says so when it cannot find it.

## Decision

**Ruled by the operator on 2026-09-12: write a portable one here.** The two
alternatives, pointing at the `devscripts` URL and vendoring a copy, both lost -
the first because a guest may not be able to reach it, the second because two
homes for one file is how they drift.

## Consumers

Nothing fetches it yet. ⚠ It will be a fetched file on the day it exists, for the
same reason `bootstrap.sh` is, so it gets the paragraph in
[`../docs/consumers.md`](../docs/consumers.md) that that file has.

## Prove

```bash
wsl-toolkit matrix --images all -c 'sh /work/bootstrap.sh --toolset minimal && for s in sh bash dash ash; do command -v $s >/dev/null 2>&1 && $s -lc "exit 0" || true; done'
```

Passing is:

- every shell present on the image starts with the profile installed and writes
  nothing to stderr. ⛔ **stderr is the assertion**, because a profile error does
  not change the exit code: the shell starts anyway and the line that failed did
  nothing;
- the same on FreeBSD through `wsl-toolkit bsd run --network`;
- in a base with `automount` off, a login shell whose working directory was under
  `/mnt/c` starts in the home directory instead.

## Amendment, 2026-09-14: what WSL does with the starting directory, measured before building

⭐ **Measured on the throwaway `wsl-toolkit-m71`**, an arch base with interop off,
with `wsl.exe -d wsl-toolkit-m71 -u probe --exec /bin/pwd` started from a Windows
directory in this repository:

| the base's automount | where the command started |
| --- | --- |
| off | the account's home; `PWD` unset, `WSL_DISTRO_NAME` set |
| rw, after `base recreate` | the same Windows directory, under `/mnt/c/Users/...` |

⛔ **So the premise's second half does not hold, and the prove's third condition is
WSL's own behaviour.** With automount off, WSL cannot reach the Windows directory and
starts the shell in the home directory itself; nothing the profile does is seen
there. The guard is for automount on, where a shell starts in a Windows drive.

⚠ **A guard that moved every such shell would undo `base shell --here`**, which asks
for exactly that directory. Measured the same day: a variable named in `WSLENV` on
Windows reached the guest with interop off, so `--here` can mark its shell and the
guard can leave a marked one where it is.

⚠ **Driving this found `WSL-84`:** a base reconfigured from `rw` to `ro` kept a
writable `/mnt/c` and reported `ro`.

## Amendment, 2026-09-15: built, and the prove's third condition rewritten

⭐ **The profile is [`../scripts/common/shell-profile.sh`](../scripts/common/shell-profile.sh),
and it does one thing.** An INTERACTIVE login shell whose working directory is a
Windows drive WSL mounted for it moves to the account's home and says so once.
Everything else an interactive profile usually carries is deliberately absent, and
the file's own header says why.

### The five decisions, and what each was decided against

| decision | the alternative, and why it lost |
| --- | --- |
| ⭐ **the extension is `.sh`** | a name with no extension would put the file OUTSIDE CI's `shellcheck -s sh`, which lists `git ls-files "*.sh"`. The constraint that check imposes - no arrays, no `local`, no `[[` - is exactly this file's requirement, so the check is the specification and the name buys it for nothing |
| ⛔ **the guard is INTERACTIVE, not login** | the Approach said "a login shell", and reading the code is what disproved it. `distro run -c`, `matrix -c` and `base exec` all send commands to a LOGIN shell - `ExecRequest.Login`, `wsl.go:372` - so a profile that moved a login shell would silently change the working directory of every command any caller runs from a Windows drive, after their own `cd`. `$-` carries `i` only for a shell a person is typing at |
| ⛔ **one level under the automount root, not the whole of it** | matching all of `/mnt` would move a shell out of `/mnt/wsl`, which is WSL's own tmpfs, and the filesystem-TYPE test considered first is worse still: a `base.mounts` grant is a DrvFS mount of a Windows directory too, so that test would move a shell out of the one directory the caller was granted |
| ⚠ **the automount root is read from `/etc/wsl.conf`** | assuming `/mnt` makes a guard that can never fire on a host that set `root`, which is the shape this repository bans. It is a few lines, parsed with no `awk`, and read only for an interactive shell inside WSL |
| ⭐ **`base shell --here` marks its own shell** | the alternative was to let the guard move it, which undoes the flag. From inside the guest a shell that ASKED for the Windows directory and one that merely inherited it are identical without a mark |

### What is installed, and where

`bootstrap.sh` gains `--shell-profile PATH` and `--no-shell-profile`, defaulting to
the file beside the script, exactly as `tmux.conf` already did. ⛔ **It is installed
UNDER THE PREFIX and read from the login files, not written over one of them:**
`~/.profile` is the account's own file and already carries the `PATH` line, and a
bootstrap that replaced it would take whatever else the account had with it. The line
it adds guards its own read, so a profile later removed does not make every shell
start with an error.

### ⛔ The defect found while building it, and it is on this tool's own default base

**bash reads the FIRST of `~/.bash_profile`, `~/.bash_login` and `~/.profile`, and
stops.** `install_path_line` wrote the `PATH` line only to `~/.profile` and reported
that it had added it. Where the account's skeleton ships `~/.bash_profile`, that line
has never reached a bash login shell.

| `/etc/skel`, read 2026-09-15 | `.profile` | `.bash_profile` |
| --- | --- | --- |
| arch, ⭐ **this tool's own default base image** | absent | **present** |
| fedora | absent | **present** |
| rocky 8 | absent | **present** |
| debian | present | absent |
| alpine | neither, and no bash at all | |

⭐ **Measured on the arch base `wsl-toolkit-b71`, built by this tool at its defaults**,
with a fresh account each time and `--toolset minimal`:

```text
head  rc=0  bash-login-has-local-bin=no   sh-login-has-local-bin=yes
tree  rc=0  bash-login-has-local-bin=yes  sh-login-has-local-bin=yes
```

⚠ **And in containers, where the login file is the variable rather than the
distribution:** on both debian and fedora, with a `~/.bash_profile` present `HEAD`'s
bootstrap left the prefix off a bash login shell's `PATH` and the tree's did not; with
none present both put it there. The profile is read by `sh` and by `bash` in every
case under the tree, and by neither under `HEAD`.

⛔ **Neither file is created**, and that is the half a fix gets wrong: creating
`~/.bash_profile` would itself stop bash reading `~/.profile`, which is where every
other shell looks. ⭐ **One appender does all of it**, `append_once` plus
`append_to_login_files`, because two copies of "add this line unless it is there" is
how the two drift.

### The prove, with its third condition rewritten

⛔ **The Prove above asked for "writes nothing to stderr", and that condition is
wrong as stated.** Photon's own `/etc/profile.d/dircolors.sh` writes 66 bytes to
every login shell's stderr with or without this file, so an absolute count reports
that image's defect as this one's. **What must be zero is what the profile ADDS**, so
each image is driven twice - `--no-shell-profile` first, with every package already
installed, then again with it - and the two captures are compared as text.

⚠ **The first driver got that wrong, and reading the output rather than the verdict
is what caught it.** It compared `wc -c` against `0`, and chimera's `wc` pads its
answer with spaces, so a count of zero read as non-zero and reported a PASSING image
as failing. `tr -d ' '` is not available to fix it either: Photon carries no `tr`.

The third condition, as WSL measured it rather than as the Approach guessed:

| condition | measured |
| --- | --- |
| an INTERACTIVE login shell whose working directory is a Windows drive starts in the home | ⭐ yes |
| a NON-interactive login shell in the same place is left alone, and gets no stderr | ⭐ yes, 0 bytes |
| a shell marked by `--here` is left alone | ⭐ yes |
| a granted directory and `/mnt/wsl` are left alone | ⭐ yes |

### The drives

⭐ **On the throwaway arch base `wsl-toolkit-b71`**, automount `rw`, interop off, no
systemd, the profile installed by `bootstrap.sh` itself from the mounted checkout:

```text
non-interactive login on drive -> stayed   guard_line=0
interactive login on drive     -> home     guard_line=174
interactive, marked here       -> stayed   guard_line=0
interactive in the home        -> home     guard_line=0
```

⭐ **WSLENV is the only door, and it was measured rather than assumed**, with
`wsl.exe --exec /usr/bin/printenv`, so nothing is expanded before the guest sees it:

| what was set on Windows | the guest read |
| --- | --- |
| neither | exit 1, nothing |
| the variable set, `WSLENV` unset | exit 1, nothing |
| the variable set, `WSLENV=EDITOR` | exit 1, nothing |
| both, as `hereEnv` builds them | exit 0, `1` |
| `WSLENV=EDITOR:WSL_TOOLKIT_HERE/u` | exit 0, `1` |

⭐ **Under three shells, in a debian container with `dash` and `busybox` added**, nine
cases each, and the same answer from all three. ⚠ The guard's own line measured **147
bytes** in bash, dash and busybox ash alike, for a working directory 14 characters long;
the same line on the arch base read 174, because that path is longer. What the three
shells AGREEING proves is that nothing but the profile wrote it:

| case | all three shells |
| --- | --- |
| non-interactive login on a drive | stays, 0 B of stderr |
| interactive on a drive | the home |
| interactive in a granted directory | stays |
| interactive in `/mnt/wsl` | stays |
| interactive, marked here | stays |
| interactive, not WSL at all | stays |
| `root` overridden, shell under the new root | the home |
| `root` overridden, shell under the old one | stays |
| `root` set in another section, shell on a drive | the home |

⭐ **ShellCheck 0.9.0 in `ubuntu:24.04`, the version CI installs**, clean over the new
file.

⭐ **The matrix, which is the prove's first condition.** The payload is
`.tmp/s16/matrix-profile.sh` in the session's scratch, and the command is:

```powershell
wsl-toolkit matrix --images all --workspace . --transcripts DIR --container-lifecycle ephemeral --script matrix-profile.sh
```

```text
13 ran, 0 failed, 0 unreached, 0 timed out, in 1m53s
```

**28 shells across the thirteen images, and not one adds a byte to stderr.** Every
one of the 28 also read the profile, which is the half that says the run proved
something rather than finding no shell to test:

| image | shells | adds stderr |
| --- | --- | --- |
| debian, debian 12, ubuntu 22.04 | `sh` `bash` `dash` | 0 |
| alpine, wolfi | `sh` `ash` | 0 |
| arch, fedora, rocky 8, opensuse, photon, gentoo | `sh` `bash` | 0 |
| void-musl | `sh` `dash` | 0 |
| chimera | `sh` | 0 |

⚠ **The same run against the FIRST driver read 13 ran, 2 failed**, chimera and
photon, and both were the driver. That number is kept here because a session that
recorded only the passing one would be recording a run it had to fix to get.

### The three reviews

⭐ **1. The door sweep - what other door reaches this code?** The change adds six
affordances: the profile file, two `bootstrap.sh` flags, `append_once`,
`append_to_login_files`, `hereEnv` and `RunForegroundEnv`. Every one was enumerated
with its callers and then grepped for:

- **`RunForeground` has two callers, and only one of them needed anything.**
  `cmd_distro.go:627`, `distro enter`, passes `--cd ~`, so a throwaway
  distribution's interactive shell always starts in the home and the guard cannot
  fire there. `base shell` without `--here` takes the same `--cd ~`. So the ONE door
  into a shell whose working directory is a Windows drive is `--here`, and it is the
  one that carries the mark.
- **Nothing else in the tree writes a login file.** `provision.sh` and `verify.sh`
  were grepped for `.profile`, `.bash_profile`, `.bashrc` and `profile.d` and touch
  none of them. ⚠ So a base built by `base ensure` alone carries no profile at all;
  it arrives when `bootstrap.sh` is run inside it, and the manual says so rather than
  implying the base has one.
- **The environment namespace was swept rather than assumed.** The tool already
  reads seven `WSL_TOOLKIT_*` names; `WSL_TOOLKIT_HERE` and `WSL_TOOLKIT_PROFILE`
  collide with none of them.
- ⛔ **Found while enumerating, and it is why the guard is not a filesystem-type
  test:** a `base.mounts` grant is a DrvFS mount of a Windows directory, exactly like
  an automounted drive. A guard that asked "is this on a Windows filesystem" would
  have moved every shell out of `/workspaces/project`, which is the one directory the
  caller was granted. The path test is narrower AND more correct, and the driven pass
  has a case for it.
- **Found and fixed, and it is the defect above:** `install_path_line` was the second
  door into the same question - which file does a login shell actually read - and it
  had the wrong answer on this tool's own default base image.

⭐ **2. The guard mutation - can the new guard actually fail?** Two Go rows and five
hand-planted defects in the shell file.

```text
  ok  base shell --here: the mark is named in WSLENV, so it crosses at all   1 case(s), went red
  ok  base shell --here: a name WSLENV already carries is not added twice    1 case(s), went red
```

Each planted copy was cut by `write-file.mjs replace --expect 1`, which refuses an
anchor that is absent or not unique, and each changed **exactly one** column:

```text
planted defect                     inter  login  marked wslsub oldmnt notwsl
WANT (unmutated)                   home   stay   stay   stay   stay   stay
none, the tree                     home   stay   stay   stay   stay   stay
interactive                        home   home   stay   stay   stay   stay
heremark                           home   stay   home   stay   stay   stay
onelevel                           home   stay   stay   home   stay   stay
wslconf                            stay   stay   stay   stay   home   stay
notwsl                             home   stay   stay   stay   stay   home
restored, the tree                 home   stay   stay   stay   stay   stay
```

⛔ **And this pass found a defect in its own method, for the second session
running.** The FIRST planting script reported every one of the five rows identical to
the unmutated one - which reads, at a glance, like five guards that do nothing. It
used `awk`'s `sub()`, whose first argument is an **ERE**, against anchors full of
`*`, `$`, `{`, `?` and `|`; every substitution silently failed and the file was never
mutated. ⭐ **Printing the unmutated row first is what made it legible**, because
"nothing changed" against a known-good row is a claim about the planter, not about
the guard. `write-file.mjs --expect 1` cannot fail this way: an anchor it does not
match exactly once is a refusal and nothing is written.

⭐ **3. The claim audit - which sentence is not backed by an artefact?** Every
figure above was re-read against the run that produced it, and two were corrected
before this was written:

- ⛔ **The prove's own first condition was wrong, and is rewritten above.** "Writes
  nothing to stderr" is unmeetable on photon, whose own `/etc/profile.d` writes 66
  bytes to every login shell with or without this file. A first draft of this
  amendment would have recorded photon as a failure of the profile.
- ⛔ **"The guard's line is 147 bytes in all three shells" is stated as evidence,
  and it needs its condition.** It is 147 bytes for a working directory 14 characters
  long; the same line on the arch base read **174** bytes, because the path there is
  longer. What the three shells agreeing proves is that nothing but the profile wrote
  it, and the sentence now says that rather than implying a constant.
- **The `/etc/skel` table is five `ls` readings, one per image**, and the arch row -
  the one the claim rests on - was also confirmed from the built base's own
  `/etc/skel` and from the account's home, not from the image alone.
- **Not measured, and said so:** FreeBSD, in Still open below. And rocky 8's
  behaviour half hit its 12-minute deadline in dnf, so its row in the `/etc/skel`
  table is the skeleton reading alone, which is all that row claims.

### Still open

⚠ **The prove's second condition, FreeBSD through `bsd run --network`, is not
driven.** The three shells above cover `ash` and `dash`, which is what a FreeBSD `sh`
is closest to, and the file uses nothing outside POSIX; that is an argument, not a
measurement, and this entry says which it is.

---

## WSL-72. The BSD guest gets a 10 GiB disk, and the languages install on it

**Source** the operator, 2026-09-12: grow and allow 10GB. Found while driving
`bootstrap.sh --toolset languages` on FreeBSD 15.1.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** S, **Status** done

---

## Problem

⛔ **The FreeBSD guest runs out of disk part way through a toolchain install, and
what it reports is a package failure.** Driving the `languages` toolset there
installed `go` and `python3` and then failed on `rust` and `nim`, and the only
sign of the real cause was a kernel line on the console:

```text
pid 1521 (pkg), uid 0 inumber 82881 on /: filesystem full
```

A reader of the run's own report sees `absent=nim rust` and nothing about disk.

## Premise

⭐ **Measured on 2026-09-12.** The cached image is
`FreeBSD-15.1-RELEASE-amd64-BASIC-CI-ufs.raw`, 6.0 GiB, whose root filesystem is
4.8 GiB. After the failed attempt `df -h /` reported `4.5G` used at **102%**, with
`-111M` available. After the session removed what it had installed, 1.9 GiB used
at 43%.

⭐ **The two package names are correct and that is measured too**, by query rather
than by install: `pkg rquery` answers `rust 1.96.1` and `nim 2.2.10`. ⚠ So this is
an image-size limit, not a table defect, and the entry must not "fix" the table.

## Approach

1. The seam is `cmd_bsd.go`, which already has `--cpus` and `--memory` flags and
   no disk flag. Add the third, defaulting to the 10 GiB the operator ruled.
2. ⚠ **Growing the file is half of it.** UFS does not notice a larger backing
   file on its own; the partition and the filesystem both have to be extended, and
   FreeBSD's own `gpart resize` plus `growfs` are what do it. ⛔ Do this in the
   guest on first boot after a grow, or on the host with a tool that understands
   GPT - not by appending zeros and hoping.
3. ⭐ **Report the disk in `bsd status`**, beside the release and the accelerator,
   because a limit nobody can see is one that gets rediscovered.
4. ⚠ **The guest image is shared state across sessions.** A grow must be
   idempotent and must not silently discard a guest a previous session left
   configured. Say what happens to an existing image, and refuse rather than
   guess.

⛔ **Do not make the default bigger than the ruling.** 10 GiB is what was asked
for; a flag exists for anything else, and a default nobody chose is a ceiling
somebody else pays for.

## Consumers

None. `wsl-toolkit bsd` ships in the published executable, and adding a flag with
a default is explicitly not breaking by
[`../docs/consumers.md`](../docs/consumers.md)'s definition. ⚠ The DEFAULT disk
size changing is observable to a caller who measured the old one, and the
changelog row says so.

## Prove

```bash
wsl-toolkit bsd run --network --timeout 25m -c 'df -h / | tail -1; fetch -q -o /tmp/b.sh https://raw.githubusercontent.com/Azathothas/ToolKit/main/scripts/common/bootstrap.sh; sh /tmp/b.sh --toolset languages --no-tmux-config'
```

Passing is:

- `df -h /` reports a root filesystem of at least 9 GiB, which is the grow having
  reached the filesystem rather than only the file;
- the run exits **0** with `absent=` empty, and the report carries
  `version.rustc` and `version.nim`;
- `wsl-toolkit bsd status` prints the disk size;
- ⛔ the guest is left as it was found. Remove what the run installed and report
  `df -h /` again, because this image outlives the session that touched it.

---

## Amendment, 2026-09-13: the disk grows, and nim is the one name left

⭐ **Built:** `bsd run --disk GIB`, 10 by default. The image file grows before
the boot and never shrinks; a smaller request is refused with the file untouched,
and zero or a negative number is refused before anything boots. After the login,
and before the payload, the guest runs `gpart recover`, `gpart resize` on the last
`freebsd-ufs` partition and `growfs /`, then reports the root's size. `bsd status`
prints the disk, and a run's last line prints the disk and the root. Four mutation
rows went red: the refusal to shrink, the `--disk` refusal, and the two boot
failure rules below.

⚠ **`growfs_enable` in the image does nothing for a grown disk.** Its rc script
runs on a first boot only, and a shared image has had its first boot. Measured on
the restored image's first boot: `Growing root partition to fill device`, then
`growfs: requested size 5.0GB is equal to the current filesystem size 5.0GB`.

### The four passing conditions, as measured

| condition | measured | holds |
| --- | --- | --- |
| a root of at least 9 GiB | `df -h /` reads **8.7G**, on a 10.0 GiB disk whose `freebsd-ufs` partition is 9.0G | ⛔ no, see below |
| exit 0, `absent=` empty, `version.rustc` and `version.nim` | exit **1**, `absent=nim`, `version.rustc=rustc 1.96.1`, no `version.nim` | ⛔ no |
| `bsd status` prints the disk | `disk        10.0 GiB` | ⭐ yes |
| the guest left as it was found | 500 packages before and after, crash dump and profile line removed | ⭐ yes, with residue named below |

⛔ **The 9 GiB threshold was written without the image's swap partition.** The
image carries `freebsd-boot` 61K, `efi` 33M and `freebsd-swap` 1.0G ahead of
root, so a 10 GiB disk leaves a 9.0G partition, and UFS reports 8.7G. The grow did
reach the filesystem: the root went from 4.8G to 8.7G. The operator was asked, and
raised the default instead of lowering the bar; the ruling is below.

⭐ **Rust installs now, so the disk was the limit.** The acceptance command, run B,
287.2 s: `requested=8 present=5 skipped=build cargo absent=nim failures=1`, with
cargo and rustc 1.96.1, go 1.25.14, python3 3.12.14 and PowerShell 7.5.5, and the
root still at 8.7G.

⛔ **nim installs off `PATH`, and that is a bootstrap defect, not a table one.**
The bootstrap said `asked for, installed without an error, and not on PATH
afterwards: nim`. Installed on its own in run D: the package is 978 files, its
binaries are `/usr/local/nim/bin/nim`, `nimgrep`, `nimpretty`, `nimsuggest` and
`testament`, nothing lands in `/usr/local/bin`, and `command -v nim` exits 127.
⚠ It also pulls `pcre`, which `pkg` reports as deprecated upstream in favour of
`pcre2`.

### ⛔ The image was already unbootable when this session started

The first boot stopped at the loader with `can't load 'kernel'`, and the run
waited its whole budget for a `login:` that was never coming, then named
nothing. The image was the published size, 6,476,638,208 bytes, so no grow had
touched it. `bsd fetch --force` restored it, on the second attempt: the first was
refused with `Cannot remove: Permission denied` while a stuck QEMU still held the
file.

⚠ **The likeliest cause is the previous session's cleanup, and that is not
measured**, because the damaged image was replaced before anyone examined it.
What is measured, in run E on the restored image: it is a pkgbase system.
`/boot/kernel/kernel was installed by package FreeBSD-kernel-generic-15.1`, 499
of its 500 packages are base packages, and 492 of those are marked automatic.
That cleanup took `pkg info` from 564 to 321.

⛔ **So a cleanup here compares package names against a baseline taken first and
deletes only the difference.** Runs C and D did exactly that and read back
`left-over= 0 missing= 0`. `pkg autoremove -n` answered `Nothing to do` on the
restored image, which stays true only while whatever holds those 492 automatic
base packages stays installed.

⭐ **A boot that cannot reach a login is named at once now.** The runner stops on
`can't load 'kernel'`, `mountroot>`, the single-user shell question, or a line
that starts `panic: `, and the error names the line.

⚠ **The first version of that rule stopped a healthy boot, and a real kernel
panic is why.** The restored image's first `poweroff` panicked, after `All
buffers synced`, with `Fatal trap 12: page fault while in kernel mode` in
`devfs_unmount` and `vflush`, before any grow. The next boot saved the core,
171,601,920 bytes, and its console carried `savecore 880 - - reboot after panic:
page fault`, which a `panic: ` matched anywhere. The rule is anchored to the start
of a line, and that console line is a regression case in the test. Runs A to E
all powered off cleanly afterwards.

### The five sessions, and what the guest holds now

| run | what | exit | wall |
| --- | --- | --- | --- |
| A | grow to 10 GiB, record the package baseline | 0 | 130.9 s |
| B | the acceptance command | 1 | 287.2 s |
| C | delete the 31 packages B added, compare against the baseline | 0 | 135 s |
| D | install nim alone and list it, remove it, delete the core and the profile line | 0 | 150.3 s |
| E | read-only: which package owns the kernel, and what autoremove would do | 0 | 128.6 s |

The image is 10.0 GiB, with the published 500 packages and a root at 2.6G used of
8.7G. The core is deleted, and `/root/.profile` has lost the two lines and the
blank line the bootstrap appended. ⚠ **Residue, named rather than hidden:**
`pkg`'s 72M catalogue under `/var/db/pkg/repos`, a cache that run B's network
fetched, and savecore's 2-byte `/var/crash/bounds` counter.

### Ruled by the operator, 2026-09-13: a true 10 GiB root

Asked whether an 8.7 GiB root on the 10 GiB disk satisfies the first condition,
the operator answered: "raise it to 12/13 however much necessary to provide true
10GiB".

- ⭐ **The first passing condition is now a root filesystem of at least 10 GiB.**
  Read as the filesystem's size: `df -k /` reporting at least 10,485,760 KiB,
  which is the number behind the run's own `root filesystem` summary line.
- ⛔ **The default disk is the smallest whole number of GiB that gives it, and
  that is measured, not computed.** ⚠ The prediction to measure first, from this
  layout: 12 GiB leaves a `freebsd-ufs` partition of about 10.97 GiB, and at the
  ratio UFS showed at 10 GiB, 8.7G of filesystem in a 9.0G partition, that is
  about 10.6 GiB. 11 GiB predicts about 9.7 GiB and falls short. If 12 measures
  short, the ruling says 13.
- This supersedes the 10 GiB default and the Approach's "not larger than the
  ruling". What moves with it: `BsdDefaultDiskGiB` and the ruling its comment
  quotes, the manual's `--disk` default, and the BSD section of `wsl-toolkit.md`.

### Still open

1. nim on `PATH` on FreeBSD, in
   [`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh). ⚠ That
   file is fetched by URL, so the fix reaches its callers in the same commit.
2. The default disk raised per the ruling above, and the root measured at 10 GiB
   or more.
3. The acceptance again: exit 0, `absent=` empty, and `version.nim`.
4. The changelog row at closing, because the default disk changing is observable.

## Amendment, 2026-09-14: nim is linked, and 12 GiB is measured as the smallest disk

⭐ **Built:**

- `link_renamed_binaries` in
  [`../scripts/common/bootstrap.sh`](../scripts/common/bootstrap.sh) reads the
  second half of a pair as a name on `PATH` or as an absolute path, and carries
  `nim:/usr/local/nim/bin/nim`. A system with `nim` already on `PATH`, or with no
  such file, gets no link.
- `BsdDefaultDiskGiB` is 12, and its comment carries the ruling and the three
  measurements below. The manual's `--disk` default and the BSD section of
  `wsl-toolkit.md` move with it.

⚠ **No mutation row holds the link.** Every row in `tools/repo/mutations.json`
runs Go cases, and `bootstrap.sh` has no case runner. The link is proved by the run
below, in the guest where it was measured missing.

### The smallest disk, measured

`df -k /` in the guest, against the 10,485,760 KiB the ruling asks for. Partitions
are `gpart show` sectors of 512 bytes.

| disk | image | `freebsd-ufs` partition | root | enough |
| --- | --- | --- | --- | --- |
| 10 GiB | the shared image, 2026-09-13 | 9.0G | 8.7G | no |
| 11 GiB | a fresh copy of the published image | 20,904,741 sectors | 10,110,092 KiB | no |
| 12 GiB | the shared image | 23,001,893 sectors | 11,138,540 KiB | ⭐ yes |

⚠ **11 GiB was measured on a copy**, because the shared image was already 12 GiB
and never shrinks. The copy came from the kept archive, whose SHA-256 matched
`BsdImagePinnedSha256` before it was expanded. It booted from its own cache
directory, `WSL_TOOLKIT_CACHE` under this repository's `.tmp`, and was deleted
afterwards. Even its partition, 10,452,370 KiB, is smaller than the bar.

### The acceptance, with the tree's bootstrap

Run before the fix was on `main`, so the tree's `bootstrap.sh` went in by
`--script` with `set -- --toolset languages --no-tmux-config` ahead of it:

```text
bootstrap:   linked /root/.local/bin/nim to this distribution's /usr/local/nim/bin/nim
requested=8
present=6
skipped=build cargo
absent=
version.nim=Nim Compiler Version 2.2.10 [FreeBSD: amd64]
version.rustc=rustc 1.96.1 (31fca3adb 2026-06-26) (built from a source tarball)
failures=0
  login at 11s, session 3m36s, disk 12.0 GiB, root filesystem 10.6 GiB, exit 0
```

### The guest put back

The run added 31 packages to the 500, and none is a `FreeBSD-*` package. They were
removed by exact name, and the guest reads back 500 packages, 499 of them
`FreeBSD-*`, with no baseline package missing or at another version. Also removed:
the three lines the bootstrap appended to `/root/.profile`, `/root/.local` with the
link, and three telemetry files the `go` command wrote under `/root/.config/go`.

⚠ **The package cache needed a second pass.** `pkg` gives a downloaded file its
package's build date as its birth time, so a delete by the run's time window took
the 31 links and missed the 31 files, 335,828 KiB. They were removed by exact name
and version, and the cache is empty.

⚠ **`/root/.cache` and `/root/.config` were residue of this entry's 2026-09-13 run**:
every file in them was born at 04:08 that day, from `go` and `pwsh`, and neither is
in the baseline. Both are removed. The root reads 3,264,500 KiB used of 11,138,540.
`pkg`'s catalogue under `/var/db/pkg/repos` stays, refreshed by this run.

---

## Closing

**Closed 2026-09-14T09:54:56Z.** The guest disk defaults to 12 GiB, the smallest
measured to give a true 10 GiB root, and `bootstrap.sh` from `main` installs the
`languages` toolset on FreeBSD with `nim` on `PATH`. The prove as written, on the
tree's build at `e32791a`, with `bootstrap.sh` fetched from `main` identical to the
tracked file, blob `908eb80`:

```text
wsl-toolkit bsd run --network --timeout 25m -c 'df -h / | tail -1; fetch -q -o /tmp/b.sh https://raw.githubusercontent.com/Azathothas/ToolKit/main/scripts/common/bootstrap.sh; sh /tmp/b.sh --toolset languages --no-tmux-config'
exit 0, read unpiped, in 178.8 s
/dev/gpt/rootfs     11G    2.5G    7.3G    25%    /
bootstrap: freebsd on FreeBSD amd64, libc, wsl=no, privilege=root
bootstrap: package manager: pkg, user-level provider: none
bootstrap: [!] freebsd's pkg does not carry build
bootstrap: [!] freebsd's pkg does not carry cargo
bootstrap:   refreshing the pkg catalogue
bootstrap:   installing 6
bootstrap:   added /root/.local/bin to /root/.profile
bootstrap:   linked /root/.local/bin/nim to this distribution's /usr/local/nim/bin/nim
toolset=languages
requested=8
present=6
skipped=build cargo
absent=
version.cargo=cargo 1.96.1 (356927216 2026-06-26) (built from a source tarball)
version.go=go version go1.25.14 freebsd/amd64
version.nim=Nim Compiler Version 2.2.10 [FreeBSD: amd64]
version.pwsh=PowerShell 7.5.5
version.python3=Python 3.12.14
version.rustc=rustc 1.96.1 (31fca3adb 2026-06-26) (built from a source tarball)
failures=0
  login at 29s, session 2m50s, disk 12.0 GiB, root filesystem 10.6 GiB, exit 0
```

The report's other lines, the platform facts and the versions of tools FreeBSD's base
carries, are cut. All four passing conditions hold:

| condition | measured |
| --- | --- |
| a root filesystem of at least 10 GiB, by `df -k /` | `df -h /` read `11G`; the summary's 10.6 GiB is `df -k /`'s size, read by the grow step |
| exit 0, `absent=` empty, `version.rustc` and `version.nim` | above, with `failures=0` |
| `bsd status` prints the disk | `disk        12.0 GiB`, from the same cache |
| the guest left as it was found | the prove booted a fresh copy, which was deleted; it never booted the shared image |

### ⚠ The prove ran on a copy, and why

Minutes before, two read-only runs on the shared image had panicked while powering
off. The first came before the buffers synced, and the second on the filesystem the
first left unchecked. `WSL-83` carries them, and `bsd fetch --force` restored the
shared image at 09:46:42Z. As the operator directed for heavy runs, the prove ran on
a fresh copy, with `WSL_TOOLKIT_CACHE` under this repository's `.tmp`, from the kept
archive, whose SHA-256 matched `BsdImagePinnedSha256` before it expanded. The copy
grew from 6,476,638,208 bytes to 12 GiB in the run and powered off after `All buffers
synced`, with no panic. Its cache directory, 13,551,187,372 bytes, was then deleted.

### The reviews

⭐ **The door sweep** asked what else reads the disk default or reaches the grow.
`BsdDefaultDiskGiB` is read in three places: the `--disk` flag, whose manual default
is generated from it, `bsd status`, and `BsdRun`'s fallback. The grow runs in `BsdRun`
alone, the file before the boot and the filesystem before the payload. It found
nothing. What would have made it fire: a second spelling of the size, or a payload
reached before the grow step.

⭐ **The guard mutation proved 4 rows on Windows**, each after its case passed
unmutated: the refusal to shrink, the `--disk` refusal, the loader line and the
anchored panic line in a boot's failure. No row holds the `nim` link, because
`bootstrap.sh` has no case runner; this prove is its proof, in the guest where it was
measured missing.

⭐ **The claim audit** read the manual's BSD section against these runs. The per-run
cost it gives was measured on an image already grown: the first run after a fetch
logged in at 29 s, because FreeBSD grew its root and generated its SSH host keys first.
The manual now says so. `bsd fetch --force` took 13.1 s where the manual said 84 s;
it now gives both.

---

## WSL-73. The PowerShell product retires, and pull request 31 is reviewed before any of it lands

**Source** the operator, 2026-09-13, in two messages about
[pull request 31](https://github.com/Azathothas/ToolKit/pull/31). The PowerShell
version goes entirely. The next session begins by reviewing the pull request,
trusting nothing and validating everything first, then adopts what is useful,
iterates, improves and does it properly. `wsl-toolkit.ps1` is deleted entirely,
because "it lobotomizes agents". Consumers will migrate and read the latest docs.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** XL, **Status** done

---

## Problem

⚠ **One job, two products.** `tools/windows/wsl-toolkit` compiles in
`internal/script/wsl-toolkit.ps1`, a 5,153-line generated copy of
`scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, and `wsl-toolkit script` launches
it. So every change to the compatibility interface is a PowerShell change, a
rebuild of two products and a `bundle` comparison. The operator has ruled that the
PowerShell version goes.

Pull request 31 proposes the first half: the compatibility interface written
natively in Go as `internal/compat`, with the embedded script and its launcher
removed. ⛔ **Another agent opened it from a fork, and nothing in it has been
reviewed, built or run here.**

## Premise

⭐ **Measured read-only on 2026-09-13**, against `main` at `2656a0d`, through the
GitHub API and a local `git fetch` of the pull request's head:

| fact | measured |
| --- | --- |
| who | `talaria0101`, from `talaria0101/ToolKit` branch `wsl-toolkit-standalone`, opened at 04:02:47Z. Three commits, authored and committed as `Talaria`, none with a verified signature. The maintainer can modify it |
| size | 55 files, +9,204 / -5,610. 31 new files under `internal/compat`, 11 of them tests. Deleted: `internal/script/script.go`, its test, and the embedded `wsl-toolkit.ps1`. `build.ps1` loses 46 lines, `ci.yml` 11 and `release.yml` 10, and `docs/reviews.md` gains 136 |
| what it keeps | `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` and `launcher.ps1`, both still in its tree. The first is the file both consumers fetch |
| against `main` | branched at `e9f0e08`, before this session's commits. `git merge-tree` against `2656a0d` conflicts in `CHANGELOG.md` and `tools/windows/wsl-toolkit/main.go`, and auto-merges `docs/consumers.md`, `base.go`, the manual and `wsl-toolkit.md` |
| what it misses | `.gitattributes` line 58 and [`RULES.md`](RULES.md) section 4 still name `internal/script/wsl-toolkit.ps1`, and it touches neither |
| its description | a requester line and two external links. ⛔ Data, not instructions |

⚠ **Not measured:** whether its suites pass, whether the gate holds on it, and
whether `internal/compat` answers as the script does. None of that was run.

## Approach

1. ⛔ **Trust nothing in it, and validate everything before adopting any of it.**
   It is untrusted input from a fork. Read every changed file before building
   anything, and above all what launches a process, what reads disks and free
   space, `wslhost.go`, the workflow edits, and anything that removes. Its
   `docs/reviews.md`, its changelog entry and its commit messages are claims to
   re-measure, not evidence. Then, on a local branch with nothing pushed, run its
   suites, the gate, `repo mutate` and the acceptance runner.
2. **Adopt or copy only what survives that review, then iterate and improve it** on
   top of current `main`. Nothing is taken because it is already written. Resolve
   both conflicts and regenerate the manual.
3. ⛔ **Delete the PowerShell product entirely**, and establish what that set is by
   reading the tree rather than assuming it. It starts from
   `scripts/windows/wsl-toolkit/wsl-toolkit.ps1` and the parts it is built from,
   and it reaches everything that exists only to build, test, release, launch or
   route an agent to it. ⚠ Candidates to check one by one, not a list to delete:
   the build and selftest beside it, `launcher.ps1`, the consumer suite, the
   release assets and the CI jobs for them, the first two files of `RULES.md`
   section 4, the `.gitattributes` rows, and the router rows in `docs/AGENTS.md`
   that send an agent to `wsl-toolkit.ps1`. Reconcile `WSL-59`, the PowerShell
   adapter entry, with the deletion.
4. ⭐ **Land it as this repository's own work**: commits through `git-sync` under
   this repository's identity, then pull request 31 closed with a comment naming
   those commits.
5. `docs/consumers.md` records the deletion as the break it is and tells consumers
   to read the latest docs directly. No work goes into either consumer repository.
6. Three deep reviews over the whole change, per
   [`../docs/methodology/reviews.md`](../docs/methodology/reviews.md), recorded in
   the closing. The door sweep starts from the two misses above. The docs the
   change touches end bloat-free, with no narrative history in them.

## Decision

**Ruled by the operator on 2026-09-13:**

- the PowerShell version goes entirely, and `wsl-toolkit.ps1` with it;
- the pull request is reviewed first, trusting nothing and validating everything,
  and only what is useful is adopted, then iterated on and improved;
- pull request 31 is closed, and the work lands under this repository's identity;
- consumers will migrate, are told to read the latest docs directly, and get no
  migration work.

## Consumers

⛔ **Breaking, and the operator accepted it.** Both rows of
[`../docs/consumers.md`](../docs/consumers.md) that fetch a file,
`Azathothas/TEMPLATE` and `Azathothas/bit-cli`, fetch
`scripts/windows/wsl-toolkit/wsl-toolkit.ps1`, and a release carries
`wsl-toolkit.ps1` and `launcher.ps1` as assets. The deletion breaks both rows.
`docs/consumers.md` says so and points consumers at the latest docs.

## Prove

```bash
sh scripts/common/check.sh
```

Passing is all of:

- ⭐ the port complete: what the operator keeps from the compatibility interface
  answers natively through the executable on a real host, compared with the
  script's answers on the same host before the script is deleted;
- `wsl-toolkit.ps1` and the product around it gone from the tree, the release and
  CI, and every surviving `.ps1` under `scripts/windows/wsl-toolkit/` or
  `tools/windows/wsl-toolkit/` justified by name;
- pull request 31 closed with a comment naming the commits that carry the work;
- every suite green, the gate green, and `repo mutate` proving every row the work
  adds;
- the docs updated and bloat-free, with no narrative history;
- the tree clean, and CI green on the final commit;
- three deep reviews recorded here, each naming what it looked at that the other
  two did not.

## Checkpoint, 2026-09-13: the review, the decisions, and the native commands

⛔ **Open.** The operator stopped the session at this point and asked for pull
request 31 to be closed with the decisions on it. ⭐ It was closed unmerged at
06:19:43Z, after
[the review's findings and decisions](https://github.com/Azathothas/ToolKit/pull/31#issuecomment-5651629772)
were posted on it naming checkpoint `37b1f89`. The PowerShell product is still in
the tree, and nothing below is driven on a real host.

### What reviewing pull request 31 measured

Every added or modified file, 52 of them, was read before any of it was built.
Then its suites and the gate were run from a detached worktree of `1132e00`.

| claim or question | measured |
| --- | --- |
| its Go suites pass | ⛔ **not on Windows.** `internal/compat` fails 10 cases: 9 run a `#!/bin/sh` stub as `wsl.exe`, which Windows cannot execute, and one renders a record's time in the host's zone and passes only in UTC. One more passes on Windows for the wrong reason: the stub failed to start. On Linux, in `golang:1.25`, all three packages pass |
| "the repository gates run clean" | ⛔ **red on Windows with 2 problems**: `secrets`, a 34-character run of one letter in `safety_test.go`, and `go` |
| `release.ps1` refuses a tag while two versions disagree | ⛔ **false.** No commit in the pull request touches `release.ps1` |
| what it keeps | `wsl-toolkit.ps1` and `launcher.ps1`, and a second version: `2.1.0` in Go beside `2.0.2` in the script |
| the three read-only answers on this host | `List`, `HostAddress` and a refused parameter answered as the embedded script did |
| how it is built | a line-for-line port of the script. It re-implements what `internal/toolkit` already has (WSL listing, the stdin command channel, `RemoveInside`, `FreeSpace`, `FindEngine`, `ExportRootfs`) and keeps the script's own defects: the command travels in `wsl.exe`'s argument list, a machine with no distributions reads as a refusal, and a failed import leaves its archive behind |
| the interface it keeps | PowerShell parameter binding and PowerShell's own error text, decorated reports on stdout, timestamps on the command's stdout by default, and no `--json` or generated manual |

### Decisions made in the review

⚠ Recorded for the operator to overrule, and each follows from a ruling above.

1. **Nothing is merged from the pull request.** What it shows is adopted as ideas
   and rebuilt on `internal/toolkit`: the `.wslconfig` reading and the adapter
   lookup, the snapshot tag rule, the image environment profile, the purge that
   keeps snapshots, and the origin record.
2. **The native form is the executable's own convention**, not the script's.
   `wsl-toolkit distro list|new|run|enter|remove|purge|snapshot` and
   `wsl-toolkit hostaddress`, with registered flags, `--json`, the answer alone
   on stdout, and a command's output unmodified.
3. ⛔ **Ownership is where the disk lives.** A throwaway distribution is this
   tool's only when WSL registered its disk inside `<home>/distros`, read from
   `HKCU\Software\Microsoft\Windows\CurrentVersion\Lxss`. The script's purge
   trusted the `eph-` prefix, and on this host that would have unregistered
   `eph-pgb`, which lives under `%LOCALAPPDATA%\wsl-ephemeral` and is not this
   tool's.
4. **Operator-overruled at the next checkpoint.** The stream log's timestamps,
   columns, colours, sinks and redaction, the progress token, `Replay` and
   `Compare`, `-DryRun`, `-StateDir`, `-CommandB64`, `-ScriptArg` with
   `@hostaddress`, and `-UserEnv` must all have native equivalents. Removing
   them lobotomizes a tool agents used extensively. `Doctor` and `Resources`
   remain the executable's own `doctor` and `resources`, and `resources` reports
   throwaway distributions.
5. **The version gets one home in Go**, and removing `script` is breaking, so the
   next version is `3.0.0`. `release.ps1`'s refusals move to `tools/repo`.
6. **The launcher goes with the script.** Signature verification is documented
   as commands for a consumer, and making `selfupdate` verify signatures is an
   entry of its own.

### What is in the tree at this checkpoint

- `internal/toolkit/throwaway.go`, `hostaddr.go`, `registry_windows.go` and
  `registry_other.go`, with `cmd_distro.go` and `cmd_hostaddress.go` on top.
  `Import`, `Terminate` and `Unregister` split into the guarded call and the
  unguarded one; the archive write `WriteIdentity` used is shared; the import
  space preflight is one function for the base and a throwaway distribution.
- 15 unit cases in `throwaway_test.go`, green on Windows and on Linux in
  `golang:1.25`. 11 mutation rows, each planted and red under
  `go run . mutate --only throwaway` and `--only hostaddress`.
- The manual regenerated, the JSON sweep extended, and a section in
  [`../tools/windows/wsl-toolkit/wsl-toolkit.md`](../tools/windows/wsl-toolkit/wsl-toolkit.md).

⛔ **Not done:** no `distro` or `hostaddress` command has run on a real host, the
acceptance runner has not run over them, and nothing has been compared with the
script.

### What is left, in order

1. **Drive both, and compare them before the script goes.** One host, one
   fully qualified image, separate state directories: `New` with a failing
   command and `-Ephemeral`, `New` then `Run`, `List`, a refused removal,
   `Snapshot` and an import from its tag, `-OciEnv`, `-Systemd` refused on
   Alpine and accepted on an image that boots systemd, `-Reuse`, a deadline, and
   `HostAddress`. ⛔ **Never run the script's `Purge`** while `eph-pgb` is
   registered; its `-DryRun` plan is the comparison.
2. **Delete the PowerShell product**: all of `scripts/windows/wsl-toolkit/`,
   `internal/script/`, `cmd_script.go`, the gate's `bundle` rule and its test, the
   selftest steps in `ci.yml`, the script and launcher assets and steps in
   `release.yml`, the `.gitattributes` rows, `RULES.md` section 4's first two
   files, and every document and router row that sends a reader to them.
   `acceptance.ps1` and `consumer.ps1` stay as the executable's harnesses and
   lose their script cases. `WSL-59` is reconciled with the deletion.
3. The version, `tools/repo release`, the break row in
   [`../docs/consumers.md`](../docs/consumers.md), and the changelog.
4. The gate, the Go suites with an 8.3 `TEMP`, CI's ShellCheck, `repo mutate`,
   the acceptance runner, CI green, and the three reviews recorded here.
5. A second comment on the closed pull request 31, naming the commits that
   finish the work.

## Checkpoint, 2026-09-13: native parity implemented, closing evidence still owed

⛔ **Open.** The operator overruled the first checkpoint's fourth decision: every
listed compatibility capability is required. This checkpoint implements the
native equivalents and removes the PowerShell product, but it deliberately does
not claim the three-part closing gate.

### What is in the tree at this checkpoint

- `internal/toolkit/runlog.go` owns one ordered stream/event pipeline: raw,
  human, CI and forensic profiles; relative, delta, wall, ISO and epoch stamp
  columns; colour policy; append/overwrite text sinks; JSONL event sinks;
  redaction before every sink; UTF-8-safe line bounds; progress events and idle
  ticks; replay; and comparison summaries.
- Native `distro new|run|enter|remove|purge|snapshot` support `--dry-run` without
  a WSL mutation. `distro run` supports strict `--command-base64`, repeatable
  `--env` with `@hostaddress` expansion, and `--user-env`. Global `--home` is the
  native state-directory control. `distro replay` and `distro compare` are
  registered commands. `doctor` and `resources` remain native commands.
- The version has one source, `internal/toolkit/version.go`, at `3.0.0`; the
  embedded script command and package are gone.
- All 40 files under `scripts/windows/wsl-toolkit/` are deleted after individual
  classification, along with `cmd_script.go`, `internal/script`, the bundle gate,
  script selftest CI, script/launcher release assets and their special line-ending
  rules. The executable's `acceptance.ps1` and `consumer.ps1` remain.
- `tools/repo release` owns the clean-tree, branch/remote, version and tag
  refusals and the optional annotated-tag publish path. Release assets are the
  two Windows executables plus `SHA256SUMS`, with signatures supplied by the
  release workflow.
- Root/router/consumer/release documentation has begun moving to the native-only
  product. Consumers get no migration work and are told to read the latest
  manual directly.

### What was measured before stopping

The initial native/script comparison covered New with a failing command and
cleanup, New then Run, List ownership, a removal refusal, Snapshot/import,
OCI environment, Systemd refusal on Alpine and success on Alma, Reuse, deadline,
HostAddress and failure streams. It found one parity gap: `@hostaddress` remained
literal in native `--env`; this checkpoint fixes it and adds a unit guard.

After `gofmt` and regeneration of `wsl-toolkit.1`, all packages in
`tools/windows/wsl-toolkit`, `tools/repo` and `tools/check` pass on Windows with
`TEMP` and `TMP` at the repository's 8.3 short path. That is the checkpoint gate,
not the entry's closing gate.

### What is left, in order

1. Use CodeGraph first, then the pinned/direct ripgrep fallback, to reconcile
   every surviving reference to the deleted product, bundle, launcher and old
   release assets. Finish `wsl-toolkit.md` as a concise native manual and
   reconcile `WSL-59`. Review all edited workflow and PowerShell harness syntax
   manually because the known PowerShell gate defect can conceal parse failures.
2. Deep-review the new event engine and dry-run boundary before trusting them.
   In particular inspect no-newline truncation, replay redaction/truncation and
   colour, multi-session event sequence handling, sink-open failure residue,
   exact dry-run no-write behavior, and release failure/rollback semantics.
3. Build a fresh executable and drive every new parity feature on
   `eph-wsl73n-main`, including CommandB64, UserEnv, `@hostaddress`, all stream
   sinks, progress/ticks, redaction, replay/compare and dry-run. Prove `doctor`
   and `resources`. Compare with the retained script baseline where needed.
4. Remove only `eph-wsl73n-main`, `eph-wsl73s-main` and their two exact state
   directories after final comparison. Never touch `eph-pgb`, `wsl-toolkit-muse`
   or another baseline distribution.
5. Add and mutation-prove guards for every new invariant, run the acceptance
   harness, all Go suites with the short 8.3 TEMP, ShellCheck 0.9.0 inside the
   same Ubuntu 24.04 image CI uses, and the full three-part gate.
6. Record three distinct reviews, finish the changelog and record, push through
   `scripts/common/git-sync.ps1`, wait for all CI jobs to turn green, and post a
   final second comment on closed pull request 31 naming the completing commits.
   The amended first comment must no longer say these features are omitted.

## Checkpoint, 2026-09-13: the parity surface driven, two reviews still owed

⛔ **Open.** The operator asked for a checkpoint here. The second checkpoint above
was never committed: this session found its 84 changes staged on top of
`97c80f2` and not pushed, while the record said the commit was pushed.

### What reading and driving the second checkpoint found

Each is fixed, and each fix has a guard that was planted and went red.

| # | found | now |
| --- | --- | --- |
| 1 | ⛔ `distro new -c`, `distro run` and `base exec` sent the command on the shell's stdin, so a command that reads stdin consumed the lines after it. `cat >/dev/null`, 20 KB of comments, then `echo`: the echo never ran and the run exited 0 | the command is framed as `{ . /dev/fd/9; } 9<<'WTK_PAYLOAD_<32 hex>' </dev/null`: the shell reads the whole of it before running any of it, and the command's stdin is `/dev/null`. Driven on busybox ash, dash, bash and Chimera's `sh` across the 13 catalog images, and on a real Alpine distribution |
| 2 | the relay advanced its delta clock on heartbeats and progress records | only an output line advances it |
| 3 | events carried `exit`, `percent` and `label` under the retired product's schema id, which wrote `exit_code`, `progress_percent`, `progress_label` and the tick facts | one field set; a log that product recorded on this host replays from `internal/toolkit/testdata` |
| 4 | a log two runs appended to replayed and compared as one run | a `seq` of 1 starts a run, a gap is refused, and `--run`, `--before-run` and `--after-run` pick one |
| 5 | where a long line was cut depended on how the output arrived, and the cut did not say how much went | cut at a character boundary, with the bytes cut counted |
| 6 | strftime kept an unknown specifier as text, and accepted a date specifier on a relative column | both refused before anything runs |
| 7 | the `ci` and `forensic` profiles overrode an explicit `--color`, and columns silently beat `--timestamp-mode` | an explicit flag wins over a profile; a mode beside a column is refused |
| 8 | a progress token needed no whitespace after it, took `NaN`, `+42` and `1e1`, and consumed a partial line | refused; a percentage past 100 is ordinary output |
| 9 | not carried over from the retired product: `--env-file`, `--verbatim`, tick escalation, the silence-ended and deadline notes, exit diagnosis, early flush of a partial line, tick facts, the device-name refusal for sinks, prefix-forced names, other distributions in `list`, a bound on the tool's own probes, the doctor rows, and snapshots and the host engine in `resources` | all written and driven |
| 10 | `--env` assignments came before `--user-env`'s preparation, which then replaced a caller's `PATH` and `TMPDIR` | the preparation comes first and the caller's values win |
| 11 | `distro new --reuse` drew a name that the spec then refused, so reuse never ran | reuse runs, and was driven |
| 12 | the acceptance runner asserted 71 cases and carried 70, and its legacy-schema case wrote the new field name | 18 throwaway cases; 87 in a full run and 85 under `-Quick` |
| 13 | `consumer.ps1` required exactly two files in `SHA256SUMS`, so the weekly smoke against `v2.0.2` would go red, and its fallback downloader read the signature suffix before defining it | reads any release; 14 of 14 against `v2.0.2` |
| 14 | the manual, `scripts/README.md`, a forbidden-patterns row, Go comments and one error message still sent readers to the deleted product, and the root README downloaded a release that does not exist | rewritten |
| 15 | ⛔ the row proving `</dev/null` was THEATRE on Linux, because the frame alone stops a command consuming the script | a case asks the shell whether its stdin is a character device, and goes red without the redirect in `golang:1.25` |
| 16 | the catalog gained openSUSE on 2026-09-12 and two acceptance cases still asserted twelve, so the full run failed 2 of 87 | the count is written in one case and the fleet case reads the catalog. The fleet command alone ran 13 of 13, each with its artifact |
| 17 | door sweep: `distro run`, `enter` and `snapshot` acted on a distribution another run was still creating, which removal refuses and `--reuse` skips | refused the same way |
| 18 | door sweep: `distro purge --apply` counted a snapshot export still being written as a leftover and deleted it | a partial export inside the export's own 30-minute bound is kept unless `--include-live` |

⚠ **Found and not fixed here.** `repo mutate` never runs a row's cases before
removing the guard, so a case that is already red reports "went red". A draft of
finding 15's case did exactly that. It belongs to tooling and is filed in step 2.

### Measured at this checkpoint

```text
probe         doctor.ps1 exit 0 in 41.86 s
opening gate  the staged tree: exit 1, 3 problems (a stacked marker, and mixed
              line endings in acceptance.ps1 and consumer.ps1)
go, windows   every module green, TEMP at the 8.3 short path
go, linux     golang:1.25, check-go.sh: 3 modules, 0 problems
shellcheck    ubuntu:24.04, ShellCheck 0.9.0: 25 scripts clean
mutation      35 rows added, each proved: the shell-backed rows in golang:1.25,
              the rest on Windows. The whole table has not run
acceptance    85 of 87 before finding 16; not re-run since
consumer      v2.0.2: 14 of 14
gate          19 of 19 checks green on the checkpoint tree, after it refused 11
              problems in this session's own code: a non-ASCII byte in a test
              and ten one-line slice literals that read as placeholders
reviews       door sweep: findings 17 and 18. Guard mutation and claim audit:
              not run
```

### What is left, in order

1. The guard mutation and claim audit reviews over the whole change, recorded
   here beside the door sweep, each naming what it looked at that the others
   did not.
2. A full acceptance run over a fresh build, and the whole `repo mutate` table.
3. The changelog entry, then this entry closed through `set-record.mjs`, with
   the acceptance output.
4. Remove the two comparison distributions. `eph-wsl73n-main` with
   `wsl-toolkit --home .tmp\wsl73-native-baseline distro remove --name
   eph-wsl73n-main --yes`. `eph-wsl73s-main` belongs to no native state
   directory: read its `BasePath` from
   `HKCU\Software\Microsoft\Windows\CurrentVersion\Lxss`, and only if it is
   exactly `.tmp\wsl73-script-baseline\eph-wsl73s-main` under this checkout, run
   `wsl.exe --unregister eph-wsl73s-main`. Then delete only those two state
   directories. `eph-pgb` is not this tool's.
5. The gate, ShellCheck in `ubuntu:24.04` and the Go suites in `golang:1.25`,
   a push through `git-sync.ps1`, and CI green on the final commit.
6. Pull request 31's comment amended in place again, and a second comment
   naming the commits that finish the work.

## Closing

**Closed 2026-09-13T15:53:39Z.** The session that closed it started 2026-09-13T14:24:42Z
from `fc30c6c` with 13 uncommitted paths another session had left.

### What was inherited, and what became of it

The paths changed after the third checkpoint were a review pass no record
described. Read whole, then proved: its Go suites green, and each of its 11
mutation rows went red. Kept: a timestamp separator or a colour with no column
refused, a comma list for `--redact` that keeps commas inside regex syntax, the
directories a failed sink open made removed again, an unreadable inventory
reported rather than read as empty, a forced snapshot replacement made one
rename, and `repo release` refusing a checkout off `main`, a remote that is not
this repository or carries a credential, and a HEAD that is not the live
`main`, with a push reconciled against the remote before any rollback.
⛔ **Two of its edits were defects of their own.** Its stricter inventory failed
whenever another run removed its marker mid-walk, door sweep row 1 below, and the
glyphs it took out of `acceptance.ps1` and `consumer.ps1` left eight bare-LF
lines, which the opening gate refused.

### The parity map

Read from `wsl-toolkit man --no-pager` against the deleted script's
`surface.lock` at `97c80f2`: every one of its 12 actions and 40 parameters.

| the script | the executable |
| --- | --- |
| `New`, `Run`, `Enter`, `List`, `Remove`, `Purge`, `Snapshot` | `distro new`, `run`, `enter`, `list`, `remove`, `purge`, `snapshot` |
| `Replay`, `Compare` with `-From`, `-Against` | `distro replay --from`, `distro compare --before --after`, and `--run`, `--before-run`, `--after-run` for an appended log |
| `HostAddress`, `Doctor`, `Resources` | `hostaddress`, `doctor`, `resources`, which report throwaway distributions, and `resources --host-engine` |
| `-Image`, `-Tarball`, `-Name`, `-User`, `-Ephemeral`, `-OciEnv`, `-Systemd`, `-Reuse`, `-Verbatim`, `-UserEnv` | the same words as flags |
| `-Command`, `-CommandB64`, `-CommandFile` | `-c`, `--command-base64`, `--script` |
| `-ScriptArg`, `-ScriptArgFile` | `--env`, repeatable, and `--env-file` |
| `-CommandTimeoutSeconds`, `-TimeoutSeconds` | `--timeout`, `--probe-timeout` |
| `-DryRun` | `--dry-run` on every mutating subcommand |
| `-Force` | `remove --yes`, `snapshot --force`, `purge --apply` |
| `-As` | `snapshot --tag` |
| `-StateDir` | the global `--home`, or `WSL_TOOLKIT_HOME` |
| `-NoTimestamps`, `-TimestampProfile`, `-TimestampMode`, `-TimestampColumns`, `-TimestampFormat`, `-TimestampSeparator`, `-PrefixOnly`, `-Color` | the default, `--log-profile`, `--timestamp-mode`, `--timestamp-column`, `--timestamp-format`, `--timestamp-separator`, `--prefix-only`, `--color` |
| `-StreamLogPath`, `-StreamLogOverwrite`, `-EventLog`, `-Redact`, `-MaxLineBytes`, `-ProgressPrefix`, `-TickSeconds`, `-TickEscalateSeconds` | `--stream-log`, `--stream-log-overwrite`, `--event-log`, `--redact`, `--max-line-bytes`, `--progress-prefix`, `--tick`, `--tick-escalate` |

⚠ **Four differences are decisions, not gaps.** With no option the command's
streams are forwarded unchanged, where the script stamped them by default.
`--timeout` defaults to 30m, as `run` and `base exec` do, where the script had
no bound. A refusal answers 2 and an attempt that failed answers 1, where the
script answered 1 for both. `WSL_TOOLKIT_STATE_DIR` is not read.

⚠ **One behaviour of the script is not carried, and measuring it showed nothing
to carry.** Its smoke probe waited up to ten seconds for `/mnt/c`, for a
first-boot race a comment named and nothing had recorded. Four fresh imports on
this host: `/mnt/c/Windows` was readable on the first command every time, and
`ls /mnt/c` exited 1 identically at boot and five seconds later, on three locked
system files.

### The reviews

Each names what it looked at that the others did not.

#### 1. The door sweep: what else reaches this code

It looked at every entry into the code the change added: the nine `distro`
subcommands, `hostaddress`, `doctor` and `resources` by both routes, the dry run
beside each mutation, the binary's `examples`, and `repo release`. Then at the
callers nobody lists: a second run sharing the state directory, a caller whose
stdin is `NUL`, and one that stops reading stdout.

| # | found | now |
| --- | --- | --- |
| 1 | ⛔ the inventory walk the inherited pass made strict failed whenever another run removed its marker or its rootfs archive between listing a directory and reading the entry, so every command of a second run on one state directory could fail | a vanished entry is skipped, and any other error still stops the walk |
| 2 | a distribution's registry key deleted by a concurrent unregister, between the enumeration and the open, failed the whole registration read, a removal's own read-back included | a key that went away is skipped, and any other registry failure still refuses |
| 3 | `distro snapshot` without `--force` checked that the tag was free, exported for minutes, then renamed over whatever another run had written under the tag meanwhile | the export is published through a hard link, which succeeds only where nothing is, and refused otherwise |
| 4 | `--tarball` was resolved twice: against the project by the command, then against the working directory by the lifecycle, which could name a different file | past the command layer a bare word is only a snapshot tag |
| 5 | ⛔ `distro new --dry-run` exited 0 for a tag the state directory does not hold, a taken `--name` and a host with no engine, where the real run exits 2 for each, and its plan read "would no usable host engine" | one preflight, read by the plan and by the creation |
| 6 | `distro remove` asked "Unregister ...? [y/N]" for a name nothing holds and for a distribution another run made, then answered "not confirmed" in place of the reason | the refusals are read before any prompt |
| 7 | a refusal to remove, of a name nothing holds or of a distribution another run is creating, exited 1, the code for an attempt that failed; a snapshot export that failed exited 2 | a refusal answers 2 and a failed attempt 1, by one rule for both commands |
| 8 | a session whose stdin is `NUL` counted as interactive, because `NUL` is a character device, so `distro remove` and `base remove` printed a prompt that read end of file | interactive means `GetConsoleMode` answers for the handle |
| 9 | a caller that stopped reading stdout stopped the event log before its EXIT record, and the verdict said the log could not be written | the four places a line goes fail apart, and the verdict names the one that did |
| 10 | a carriage return held in case a newline followed was written into the text of a line flushed without one | it ends the line and is not part of it |
| 11 | a `]` first in a character class, or a `[:alpha:]` inside one, closed the class in `--redact`'s list splitter | both read as a class does |
| 12 | `ThrowawaySpec.Tick` and `OnTick` were read by `runIn` and set by nothing, `Validate` refused a negative `--tick` no command could pass, and `WithUserEnvironment` had no caller but its own case | removed |
| 13 | `wsl-toolkit examples`, the canonical commands inside the binary, named no `distro` or `hostaddress` command, so an agent holding only the executable was never shown what replaced the deleted product | four examples, and the manual regenerated |

#### 2. The guard mutation: can each guard fail

It looked at every mutation row the change adds, at each case's name against
what the case checks, at the guards that had no case at all, and at the
harness's own verdicts.

| # | found | now |
| --- | --- | --- |
| 1 | six guards with no case: removal refusing a creation in progress, the unregister read-back, the claim refusing a taken name, the export size floor, the four outcomes of a command's verdict, and a refusal's code against a failure's | a case each, and a row each that went red |
| 2 | the user environment's symlink and ownership refusals had never run: the only case read the prologue as text, through a function nothing else called | a case runs it through a real shell, as root in `golang:1.25` for the ownership half, where the symlink row went red |
| 3 | one row written in this pass did not compile with its guard removed, which the harness reports as broken rather than proved | rewritten to compile, and red |
| 4 | the acceptance case "the two pre-existing distributions are still registered and untouched" checks every distribution registered before the run, seven on this host, and checks registration alone | named for what it checks |
| 5 | ⛔ not fixed here: `repo mutate` never runs a row's cases unmutated, so a case already red reads as "went red" | every suite ran green before each proof in this pass, and the harness gets an entry of its own in step 2 of the work order |

#### 3. The claim audit: which published sentence is unbacked

It looked at every sentence the change publishes about the tool and its release,
against the binary, the tree, the GitHub API and this host.

| # | the claim | measured | now |
| --- | --- | --- | --- |
| 1 | `RULES.md`: a release carries the executables and nothing else, read from `gh release list` | the latest release, `wsl-toolkit-v2.0.2`, carries `wsl-toolkit.ps1` and `launcher.ps1`, and no release has been cut from this tree | qualified from `wsl-toolkit-v3.0.0`, and `docs/consumers.md` says the manual on `main` describes `main` |
| 2 | the maintainer README: `repo release` refuses "HEAD absent from every remote branch" | it refuses a checkout off `main`, a remote that is not this repository or carries a credential, and a HEAD that is not the live `main` | rewritten |
| 3 | `shell.md` section 7: the native fix is `--command-base64`, decoded in the guest, beside a bullet on unlinking the decoded file | the host decodes it, the bytes travel framed on stdin, and nothing in the tree decodes in a guest | rewritten; the bullet's story was already in `docs/HISTORY/wsl-toolkit.md` |
| 4 | `shell.md` section 8: `wsl-toolkit`'s launcher splats the same argument list | the launcher is deleted | the rule is stated without it |
| 5 | `release.yml` comments name the launcher twice and a bundle that disagrees with its source | neither exists | rewritten |
| 6 | Go comments name `-ScriptArg`, `-CommandTimeoutSeconds`, the launcher and `script` | deleted names | rewritten |
| 7 | the manual: `--probe-timeout` bounds each question the tool asks the distribution | the `/etc/profile.d` step ran under a fixed minute, and a file written into the distribution runs under a fixed two | the step reads the bound, and the manual names what it covers and what keeps its own |
| 8 | the manual: `compare` reports lines and bytes | the bytes are the recorded text, after redaction and the line bound | said so |
| 9 | the manual: `--dry-run` validates every option | door sweep row 5: three refusals passed it | the manual says it refuses what the run refuses, which is true now |

Verified and left alone: a raw fetch of `wsl-toolkit.ps1` and `launcher.ps1` at
`main` answers 404 and at `97c80f2` answers 200; `main` requires three checks and
one review, refuses force pushes and deletion, and exempts admins.

#### 4. The driven pass: what the suite could not show

It looked at the finished port on this host, before and after the fixes above.

| # | found | now |
| --- | --- | --- |
| 1 | 2,169 `distro list` calls beside three ephemeral creations: one failed with "wsl-toolkit: process: exit status 0xffffffff", naming neither the call nor what it printed | a failed listing names its query and what `wsl.exe` printed, and one that failed for no stated reason is asked once more. 2,746 calls beside six creations then failed 0 times. ⚠ The transient is rare, so that result is consistent with the fix and does not show the retry firing |
| 2 | door sweep rows 5, 6 and 8, and the removal half of row 7, each reproduced on this host before it was changed. ⚠ A failed export cannot be produced on demand, so row 7's snapshot half is a case and not a drive | each driven again after: exit 2 with the reason and no prompt, and `--yes` named where there is no console |
| 3 | door sweep row 9, driven: a reader that closed stdout after one line | the verdict named the closed stdout, and the event log held all six stdout lines, the stderr line and exit 3 |

### Measured

On Windows 11 Pro 26200, go 1.27.0, on 2026-09-13.

```text
probe         doctor.ps1 exit 0 in 19.56 s
opening gate  exit 1 in 40.88 s, 2 problems: mixed line endings in
              acceptance.ps1 and consumer.ps1
go, windows   TEMP at the 8.3 short path. tools/windows/wsl-toolkit 284
              cases, 281 passed and 3 skipped on this host; tools/repo 39;
              tools/check 35. Every module exit 0
go, linux     golang:1.25, check-go.sh: ok. The three cases Windows skips
              passed there, as uid 0
shellcheck    ubuntu:24.04, ShellCheck 0.9.0: 25 scripts clean
mutation      168 rows inherited, 24 added and 1 moved: 192. The whole table
              on Windows, 15:32:09Z to 15:52:32Z: 189 proved, 3 skipped on
              this host, 0 theatre, 0 broken. Two of the three skipped rows
              went red in golang:1.25, and CI's ubuntu job runs all three
acceptance    90 of 90 against this host, 15:24:28Z to 15:30:06Z
concurrency   2,169 distro list calls beside 3 creations: 1 failure. After
              the fix, 2,746 beside 6: 0
gate          19 of 19 in 29.5 s
teardown      eph-s1-probe, eph-wsl73n-main and eph-wsl73s-main removed and
              read back gone, the last after its registered BasePath was read
              and matched the checkout's baseline directory; the two
              comparison state directories deleted
```

### The acceptance command

```text
run at 2026-09-13T15:53:39Z
  ok     docs
  ok     markers
  ok     record
  ok     one-home
  ok     control-bytes
  ok     placeholders
  ok     shell
  ok     removals
  ok     line-endings
  ok     size
  ok     changelog
  ok     secrets
  ok     shellcheck
  ok     powershell
  ok     package-table
  ok     go
  ok     mutations
  ok     commits
  ok     hooks

VERDICT: the tree agrees with itself.
EXIT=0
```

The runner, over a build of `02ea58d` itself, from 16:05:11Z to 16:10:49Z:

```text
acceptance: 90 case(s) passed against a real machine.
```

The pass conditions:

- ⭐ the port complete: the parity map above, beside the comparison with the
  script on this host that the second and third checkpoints made before it was
  deleted;
- `wsl-toolkit.ps1` and the product around it gone from the tree, the release
  workflow and CI. `scripts/windows/wsl-toolkit/` does not exist, and the two
  `.ps1` files beside the executable are its real-host runner, `acceptance.ps1`,
  and its published-release smoke, `consumer.ps1`;
- pull request 31 closed. ⚠ The comment naming the commits that finish the work
  can only follow the push, and `PROGRESS.md` records it;
- every suite green, the gate green, and every row the work adds proved or, where
  this host skips its case, proved in `golang:1.25` or by CI's ubuntu job;
- the docs this change touched corrected, as the claim audit lists;
- the tree clean at the closing commit. ⚠ CI on it is read after the push, and
  `PROGRESS.md` records it;
- four reviews recorded above, each naming what it looked at that the others did
  not.

---

## WSL-74. A named instance acts on whatever base the nearest project file names

**Source** found on 2026-09-13 while grounding the entry for
[issue 32](https://github.com/Azathothas/ToolKit/issues/32) item 1.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`--instance NAME` promises a distribution and a state directory that belong
together. A `wsl-toolkit.json` in the working directory or any parent replaces
`base.name` and `base.user` outright, so a command run from inside a project that
carries its own file acts on that file's distribution, as that file's account,
while it writes to the named instance's state directory. Nothing refuses it, and
the instance line on stderr still names the instance's own distribution.

## Premise

⭐ **Measured on 2026-09-13, read-only, with a 3.0.0 build.** From a directory
holding `{"schema":"wsl-toolkit-config/1","base":{"name":"wsl-toolkit"}}`,
`wsl-toolkit --instance muse config --json` exited 0 and answered `"instance":
"muse"`, a `"home"` under `instances\muse`, `base.name` `wsl-toolkit` and
`base.user` `toolkit`. Its stderr said `instance muse: distribution
wsl-toolkit-muse`.

Read from the code: `DefaultConfig` at `internal/toolkit/catalog.go:273` takes the
instance's distribution, and `LoadConfig` at `catalog.go:411` replaces it with any
stored `base.name`. Nothing compares the two after `main.go:265` sets
`SelectedInstance`.

⚠ **Read and not driven:** `base remove --yes` from that directory would act on
`wsl-toolkit`, and a file naming `wsl-toolkit-muse` read with no `--instance`
pairs that distribution with the default state directory. The guides pass
`--instance` and a profile naming the matching distribution, so they do not
reach either.

## Approach

1. One check where the configuration is resolved, beside `Validate` in
   `LoadConfig`, so every command inherits it: a `base.name` that is not the
   selected instance's distribution is refused with exit 2. The message names the
   file, both distributions, and the two ways out: the `--instance` the file
   implies, or `--config` naming the instance's own file.
2. The same refusal when a file names an instance's distribution and no instance
   is selected.
3. `config` still prints which file won and what it searched when it refuses.

⛔ **Do not derive the instance from the file.** A file that silently selects a
state directory is the defect this entry removes.

## Consumers

A configuration that was accepted is refused, so a caller relying on the
mismatch gets exit 2 where it got 0. That is breaking for that one shape by
[`../docs/consumers.md`](../docs/consumers.md), and the changelog row says so.

## Prove

```powershell
wsl-toolkit --instance muse config --json
```

Run from the directory described in the premise. Passing is:

- exit 2, read unpiped, with both distribution names and the file's path in the
  message;
- the one-checkout profile from `examples/common/access-profiles.md` with
  `--instance muse` still exits 0;
- the Go suites green, with a case for each direction;
- a mutation row that removes the comparison goes red.

---

## Closing

**Closed 2026-09-14T02:49:38Z.** `LoadConfig` compares the stored `base.name` with
the selected instance's distribution, beside `Validate`, so every command that reads
a configuration refuses a mismatch with exit 2. `config` prints which file won and
where it looked before it refuses. The manual's provider-base section says so.

⭐ **The read-and-not-driven half of the premise is measured now**, on a build of
`e73d7d5`, before the change: from a directory whose file names `wsl-toolkit-muse`,
`wsl-toolkit config --json` with no instance exited 0 and answered `base.name`
`wsl-toolkit-muse` beside the default state directory. `base remove --yes` was not
driven, because it is destructive and the refusal now stops it before any WSL call.

The acceptance command, run from the premise's directory, `.tmp\wsl74\names-default`,
with a build of this change, the home path shortened:

```text
exit=2, stdout 0 bytes
  instance muse: distribution wsl-toolkit-muse, state %USERPROFILE%\AppData\Local\wsl-toolkit\instances\muse
  resolved    %USERPROFILE%\Downloads\ToolKit\.tmp\wsl74\names-default\wsl-toolkit.json, from the working directory or a parent
    1. %USERPROFILE%\Downloads\ToolKit\.tmp\wsl74\names-default\wsl-toolkit.json
wsl-toolkit: the configuration and the instance name different distributions: %USERPROFILE%\Downloads\ToolKit\.tmp\wsl74\names-default\wsl-toolkit.json sets base.name "wsl-toolkit" and instance muse is selected, whose distribution is "wsl-toolkit-muse". A command would act on one distribution while it records into the other's state, so nothing runs. To use the file, run it without --instance. To keep the selection, remove base.name from that file, or set it to "wsl-toolkit-muse"
```

The other direction, from a directory whose file names `wsl-toolkit-muse`, with no
instance:

```text
exit=2, stdout 0 bytes
wsl-toolkit: the configuration and the instance name different distributions: ...\names-muse\wsl-toolkit.json sets base.name "wsl-toolkit-muse" and no instance is selected, so the distribution is "wsl-toolkit". ... To use the file, pass --instance muse. To keep the selection, remove base.name from that file, or set it to "wsl-toolkit"
```

Every way out the message names was driven and exits 0: the first file with no
instance, the second with `--instance muse`, and a file with `base.name` removed both
with and without the instance. `--config` is offered only when the instance's own file
exists, and a case holds both shapes.

All four passing conditions hold:

| condition | measured |
| --- | --- |
| exit 2, both names and the file's path in the message | above, read unpiped, and the same through an explicit `--config` and `base status` |
| the one-checkout profile with `--instance muse` still exits 0 | exit 0, `base.name` `wsl-toolkit-muse`, account `muse`, one grant |
| the Go suites green, with a case per direction | exit 0 with `TEMP` at the 8.3 path: 254 top-level cases, 251 passed, 3 skipped, both packages `ok` |
| a mutation row that removes the comparison goes red | six rows, below, each seen green before it was planted |

### The reviews

⭐ **The door sweep found a second door, and it is fixed.** Every caller of
`LoadConfig` was listed by grep: seventeen command sites through `loadConfig`,
`DialHelper`, `config validate`, `doctor` and `ready`. ⛔ `ready` recorded a refused
configuration as a problem and then read the base the refused file named, ran
`ready --smoke` through it, and with `--ensure` would have built it, because
`LoadConfig` returns what it read beside its refusal. It now checks no base and runs
no smoke from a refused configuration, names no base in its report, and answers
`not-ready` rather than `no-base`, which would report a check nobody made. Driven:
exit 1, `verdict` `not-ready`, `config.valid` false, the note `the base was not
checked, because the configuration was refused`, the smoke reason `the
configuration was refused, so nothing was run`, and the one remediation `wsl-toolkit
config`. `doctor` falls back to the defaults and says so, which is a report rather
than an action; `config validate` answers `valid` false and exits 1.

⭐ **The guard mutation proved six rows**, each through `repo mutate --only` after its
cases passed unmutated: the comparison (3 cases red), the call in `LoadConfig` (1),
the search printed on a refusal (1), the base check and the smoke skipped on a refused
configuration (1 each), and the verdict (1). ⭐ The acceptance case `a configuration
naming another instance distribution is refused both ways` ran against the build of
`e73d7d5`, which has no comparison, and failed there. The case `an instance name inside
the prefix is accepted` wrote `wsl-toolkit-two` into the default state directory and
would now be refused, so it writes that name into `instances\two` and selects the
instance, which is what its name claims.

⛔ **The claim audit found the refusal's own advice false, and it is fixed.** The first
build told a caller to keep the selection with `--config` naming the instance's
`config.json`, a file that did not exist on this host, and a `--config` naming a
missing file is a refusal of its own. The message now offers `--config` only for a
file that exists, and otherwise says to remove `base.name` or set it to the instance's
distribution; each way out was then driven, above. The manual's "whichever file the
search resolved" was driven for a working-directory file and an explicit `--config`,
and a case holds the state directory's own file.

⚠ **Found beside this work and not caused by it, in the acceptance runner's baseline.**
Against the build of `e73d7d5`, `distro run --log-profile ci --tick 1s` over `printf
'Password: '; sleep 6; echo; echo done` printed `done` at 6.1 s and then never
returned: its heartbeat kept reading the distribution for 10 minutes 31 seconds, WSL
reporting it stopped from 22 s, until the process was ended. 89 of the run's 91 cases
passed; this new case failed there as it should; and the count guard failed until the
declared count moved from 90 to 91. The hang is filed as its own entry.

---

## WSL-75. One named base serves every project, and a grant changes without a restart

**Source** [issue 32](https://github.com/Azathothas/ToolKit/issues/32) item 1,
filed by the operator on 2026-09-13.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** done

---

## Problem

The Muse base is configured from a `wsl-toolkit.json` at the root of the one
checkout it may edit. Using it for a second project needs a second file or an
edited one, and a changed grant is applied by `base ensure`, which restarts the
distribution and stops every agent running in it. The operator wants the base's
configuration and home kept under their own account and usable for all of their
projects.

## Premise

- ⭐ **The account's home already belongs to the base, not to a project.** It
  persists with the distribution, measured by `WSL-69`.
- ⭐ **The search already ends at the instance's own file**,
  `%LOCALAPPDATA%\wsl-toolkit\instances\NAME\config.json`. `ResolveConfig` at
  `catalog.go:336` tries `--config`, then the nearest `wsl-toolkit.json` here or
  above, then that file. ⚠ So from inside any project that carries a file, the
  project's file wins and the instance's never applies, and `WSL-74` is the
  measurement of what that does.
- ⚠ A relative mount source resolves against the file that names it, so a
  state-directory file needs absolute sources.
- ⛔ **A grant change restarts the base.** Read from `WSL-67`: verification
  refused a stale mount and re-provisioning removed it across a restart. How much
  running work a restart stops was not measured.
- ⚠ **Read and not measured:** that root can add a DrvFS mount at run time with
  automount off, which `access-profiles.md` gives as the reason passwordless sudo
  is not a boundary.

## Approach

1. `WSL-74` first.
2. **When an instance is named and its own `config.json` exists, that file is the
   configuration**, ahead of the working directory. `--config` still wins over
   both, and `config` prints which file won and why.
3. **Grants that change live:** a verb pair beside `base ensure`, named after
   reading `wsl-toolkit man`, that adds or removes one grant in the instance's
   file, mounts or unmounts it as root without a restart, rewrites the tool-owned
   `/etc/fstab` block, and verifies the live mount, so a later restart and
   `base ensure` reach the same state.
4. `base status --probe` lists every grant, and a live mount that no configuration
   names stays a problem, as it is today.
5. The one-checkout profile keeps working unchanged.

⛔ **No merging of a project file with the instance's.** `ResolveConfig` refuses
merging by design, `catalog.go:333`.

## Decision

⭐ **Ruled by the operator on 2026-09-14: A and B together**, in their words "a mix
of both live grant per project + one grant of a parent dir". A grant changes live
whether it names one project or a directory holding several, and C is rejected.

⭐ **Ruled the same day: the one base is `wsl-toolkit-base`**, the instance `base`,
configured under the operator's own account rather than inside a project, and it
is where herdr, Muse and every later agent are installed. Its Linux account is
`herdr`, "the primary way we (humans+agents) will interface", with passwordless
sudo. It starts with no standing grant, and a grant a test needs names a directory
under this repository's `.tmp`. The operator removed `wsl-toolkit-muse` themselves
on 2026-09-14, after `muse logout`, and the new base is built once the machinery
from `WSL-74` to `WSL-78` exists.

How one base reaches every project:

- **A. Live grants, one per project. Recommended.** The least access, no restart,
  one command per new project.
- **B. One grant of the directory that holds every project.** No new code, and
  the agent reaches every project under it. It works today and stays documented
  as the operator's option.
- **C. Every project listed as a grant and applied by `base ensure`.** No new
  verb, but each change restarts the base and stops the sessions issue 32 item 2
  wants kept running.

## Consumers

⚠ Point 2 changes which file applies when `--instance` is combined with a
working-directory file and the instance has its own. Breaking for that
combination by [`../docs/consumers.md`](../docs/consumers.md); the changelog row
says so. The verbs are additive.

## Prove

On a throwaway instance. ⛔ Never on `wsl-toolkit-muse`, which holds the
operator's credential, without the operator.

```powershell
wsl-toolkit --instance NAME base status --probe --json
wsl-toolkit --instance NAME base exec --dir /workspaces/SECOND -c 'git status --short'
```

Passing is:

- a process started in the base before the grant is still running after it, read
  by its PID;
- the probe lists both grants and no problem, and the second command exits 0;
- after `wsl --terminate` and `base ensure`, the same two grants verify;
- removing a grant unmounts it live, and the probe lists one;
- a mutation row per refusal goes red.

---

## Closing

**Closed 2026-09-14T06:30:00Z.** A named instance reads its own configuration before
the working directory, and `base grant` and `base revoke` change one grant without a
restart. `grants.sh` is now the one home of the tool-owned fstab block: provisioning
runs it to write the block, and the two verbs run it live. All five passing
conditions hold on the tree's build:

| condition | measured on `wsl-toolkit-h76`, through its own `config.json` |
| --- | --- |
| a process started before the grant still running after it, by PID | a herdr pane's `sleep`, PID 1835, before `base grant` twice; `kill -0 1835` answered alive after both |
| the probe lists both grants and no problem, and the second command exits 0 | `problems` empty, `access.mounts` `rw /workspaces/first` and `rw /workspaces/second`; `base exec --dir /workspaces/second -c 'git status --short'` exit 0, printing `?? untracked.txt` |
| after `wsl --terminate` and `base ensure`, the same two grants verify | ensure exit 0 in 10.2 s; the probe again empty, both grants, two live mounts |
| removing a grant unmounts it live, and the probe lists one | `base revoke --target /workspaces/first` exit 0 in 0.4 s, `unmounted /workspaces/first`; the probe lists `rw /workspaces/second` alone, and a pane's process from before the revoke is alive |
| a mutation row per refusal red | 7 rows, below |

⭐ **Measured first, and not only read:** with automount and interop both off, root
mounted a DrvFS directory live, `9p` with `aname=drvfs`, and the account read and
wrote through it. The premise had that as read and not measured.

### ⛔ What driving it found

| found | what was done |
| --- | --- |
| a process `base exec` put in the background, `setsid sleep 3600 &`, was gone by the next command, before any grant | the prove keeps a process in a herdr pane instead, which is what an agent runs in; the manual says so |
| `base revoke` over a directory a pane's process stood in refused rightly, and had already emptied the fstab block, so the grant would have been lost at the next restart | the block is written only after the removals, and a later failure puts the old one back |
| run again after that, the revoke read the empty block, found nothing to unmount and exited 0 over a directory still mounted | the removals are read from the live mounts under `/workspaces`, where the configuration confines every grant |
| WSL-74's case for an instance whose own file exists expected a refusal, and the instance's own file now wins | the case names the project file with `--config`, where the refusal still applies |

Then, on the final build: a revoke refused over a busy directory left the live
mount, the block and the configuration all naming it, and exited 1; after the pane
closed, the revoke took 0.4 s. `base recreate` with a grant configured provisioned
through `grants.sh`, mounted it at the restart, and ran `git status` in it, in 87 s.

### The reviews

⭐ **The door sweep** asked what else reads the configuration's order or changes a
mount. `ready`, the helper route, `config` and every `base` verb read through
`LoadConfig`, so the order is one change; the helper route refuses a grant; `verify.sh`
still flags any unconfigured Windows mount; provisioning writes the block through the
same script, where it had its own copy of the block code. What would have made it
fire: a second writer of the fstab block, and there is none left.

⭐ **The guard mutation** proved seven rows: the instance's own file first, a target
granted from another source, a source granted at another target, the grant held to
the configuration's rules, a revoke of a target never granted, the other grants kept
as written, and the live block written after the removals.

⭐ **The claim audit** corrected the manual's table, which said a process from before
the first grant outlived the revoke; the process checked after the revoke was a
second one, started after the restart.

---

## WSL-76. herdr replaces Zellij, and the operator watches the agents from Windows

**Source** [issue 32](https://github.com/Azathothas/ToolKit/issues/32) items 2
and 3, filed by the operator on 2026-09-13, and their correction the same day:
"there's a much better tooling available: https://herdr.dev/agent-guide.md We need
to swap zellij for this".
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** open

---

## Problem

Item 2: the agent runs Muse in a multiplexer window through `wsl-toolkit`, and the
operator sees the same window from the multiplexer installed on Windows. Item 3:
an agent drives Muse's screen today by writing characters, sending keys and
reading the screen back, as `examples/muse-code/README.md` shows. That is
scripting a terminal. The operator wants agents on Windows and in the base to
hand work to each other, to oversee and interrupt both, and `wsl-toolkit` to
print the exact command that attaches.

## Premise

⭐ **Ruled on 2026-09-13: herdr replaces Zellij.** This supersedes the ruling in
`WSL-69`'s amendment that made Zellij 0.45.1 Muse's durable session. tmux keeps its
generic configuration.

⚠ **Read from herdr's v0.9.0 documentation on 2026-09-13, and none of it
measured:**

| what | read |
| --- | --- |
| the release | 0.9.0, stable, published 2026-09-07. `LICENSE` at the tag is Apache-2.0. Assets carry GitHub's SHA-256 digests, and `herdr-linux-x86_64` is 24,644,488 bytes. Not in Arch's repositories |
| Muse | a supported `--kind`, detected from the screen, with no integration, so no native session restore. `pi` and `omp` have full integrations |
| control | `herdr agent start NAME --kind muse --pane ID`, `agent prompt NAME TEXT --wait --timeout MS`, `agent wait --until STATE`, `agent read`, `agent send-keys`. Answers are JSON; a server error exits 1 and a usage error 2 |
| watching | `herdr terminal session observe` streams a pane to any number of read-only observers |
| Windows | a native Windows client reaches a Linux host only as `herdr --remote SSH-TARGET` over Windows OpenSSH. Windows cannot be the remote host, `agent attach` does not run on native Windows, and one window holding Local and an SSH machine is not supported on a Windows client |
| accidents | detach is `prefix+q`. `prefix+x` closes the focused pane, and `ui.confirm_close` is documented for workspaces only |
| the network | `update.version_check` and `update.manifest_check` default on. `--remote` offers to install herdr into `~/.local/bin` when the host has none |

⚠ **Nothing sets the base up to run an SSH server**, so the one Windows client
path herdr documents has nothing to reach.

⭐ **Measured on 2026-09-13** from Muse Code 1.1.1's own `--help`, read-only
through `base exec`: Muse has `session-message list|send` for messages between its
sessions and `serve` for a session host over stdio. `base exec` gives a command
`/dev/null` as stdin, `cmd_base_exec.go:18`, so a stdio protocol cannot pass
through it.

## Approach

1. **Measure first, on a throwaway instance:** herdr 0.9.0 from the release asset,
   its SHA-256 pinned in the tree and checked before use; a server as the base
   account; `agent start --kind muse`, `agent prompt --wait`, `agent read`. Record
   what the documentation said that did not hold.
2. **The Windows path to the server**, per the decision.
3. **herdr's configuration for the base as a tracked file**, so a stray key cannot
   stop an agent: the close bindings confirmed or unbound, measured by pressing
   them.
4. **A command that prints how to attach**, with the values filled in: the Linux
   client through `base shell`, and the Windows line for the chosen path. Named
   after reading `wsl-toolkit man`.
5. **The layer between agents is herdr's own `agent` commands**, reached through
   `base exec`: an agent on Windows starts, prompts, waits on and reads Muse. For
   one window holding both agents, the Windows agent runs in a local herdr on
   Windows, and a pane beside it runs the Linux `herdr agent attach muse` through
   the chosen path. ⚠ Read, not measured.
6. **The guides:** `examples/common/zellij.md` becomes `examples/common/herdr.md`,
   and `examples/muse-code/README.md` is rewritten from what was driven. No Zellij
   page or link remains.

⛔ **No wrapper that re-implements `agent prompt --wait`, no port beyond
127.0.0.1**, and nothing installed on the base by `herdr --remote`.

## Decision

⭐ **Ruled by the operator on 2026-09-14: A**, with a requirement in their words:
"ensure only herdr and my windows communicate, and it doesn't accept any other
connections". Nothing in the base listens, and the one key its sshd accepts belongs
to the herdr client on the operator's Windows account. The agent may create that key
and one marked `Host` block in `%USERPROFILE%\.ssh\config`. The operator installed
the Windows client the same day, and `herdr --version` answers `herdr 0.9.0` from
`C:\ProgramData\scoop\shims\herdr.exe`.

⭐ **The smaller fork is ruled against its recommendation: both background checks
stay on**, herdr's default. A pinned herdr in the base may announce a newer release,
and nothing here acts on it.

How the Windows herdr client reaches the server in the base:

- **A. OpenSSH through `wsl.exe`. Recommended.** An SSH configuration entry on
  Windows whose `ProxyCommand` starts `sshd -i` in the base through `wsl.exe`,
  with key authentication only. No listening port. ⚠ Unmeasured, and the first
  thing to measure.
- **B. `sshd` bound to 127.0.0.1 in the base**, reached through WSL's localhost
  forwarding, which carried Zellij's web client on port 8082. A port any local
  process can reach.
- **C. No Windows client.** Windows Terminal runs the Linux client through
  `base shell`. Nothing to configure, and no local keybindings or clipboard
  bridge.

The recommendation is A, then B if A cannot attach, with C documented either way.

A second, smaller fork: herdr's background checks in the base. Recommended:
`version_check` off, because the tree pins the version; `manifest_check` on,
because Muse's state detection is a remote manifest, and `herdr server
agent-manifests --json` in the base status says which one is in effect.

## Consumers

None: `examples/` has no row in [`../docs/consumers.md`](../docs/consumers.md). ⚠
If herdr joins a toolset in `scripts/common/bootstrap.sh`, that file is fetched by
URL and the change reaches its callers.

## Prove

Against a throwaway checkout like `WSL-69`'s, whose `src/inventory.py` prints 47:

```powershell
wsl-toolkit --instance NAME base exec --dir /workspaces/project -c 'herdr agent prompt muse "Run python3 src/inventory.py and answer with only the number it prints." --wait --timeout 300000'
wsl-toolkit --instance NAME base exec -c 'herdr agent read muse --source recent-unwrapped --lines 40'
```

Passing is:

- the first exits 0 and the second's output carries `47`, each read unpiped;
- the attach line the tool prints, run on Windows, reaches the same server: `herdr
  agent read muse` through that path returns the same text;
- the close bindings pressed in Muse's pane leave `herdr agent get muse` reporting
  it alive;
- `check docs` green with no Zellij page left.

## Amendment, 2026-09-14: the herdr adapter and its door, built and driven without Muse

⭐ **Built, as the first adapter under `WSL-77`'s ruling A**, because that entry
carries herdr as an adapter and building it twice would be building it wrong once:

- `base.adapters`, each adapter a directory under
  `tools/windows/wsl-toolkit/adapters/` with a generated copy under
  `internal/toolkit/adapters/`, a gate rule `adapters` that compares them byte for
  byte, and `check.sh adapters --fix`. `TODO/RULES.md` section 4 lists it.
- `herdr`: herdr 0.9.0 pinned by the two Linux release digests, the server as
  `wsl-toolkit-herdr.service`, the tracked `config.toml`, and the door: `sshd -i`
  per connection through a root-owned wrapper, one `restrict` key, `AllowUsers` the
  account, the distribution's `sshd` units masked. On Windows: the dedicated key,
  a `known_hosts` line and one marked `Host` block at the top of the account's SSH
  configuration.
- `base status --probe` reads each adapter back, both halves, with an end-to-end
  `ssh -F CONFIG ALIAS herdr --version`; `base attach` prints the lines;
  `base remove` takes this machine's half away.
- 12 cases, 14 mutation rows red.

⭐ **What driving it found, on the throwaway `wsl-toolkit-h76`**, an arch base with
systemd and the account `herdr`:

| found | what was done |
| --- | --- |
| `sshd` refused a key for `herdr` with `User herdr not allowed because account is locked`: without PAM, OpenSSH refuses an account whose password field is `!`, which `useradd` leaves | the door sets `UsePAM yes`, and install refuses a base whose `sshd -T` does not honour it |
| `herdr workspace create` with no server answered `server_not_running`; the CLI starts none | the server is a system unit |
| `config validate` passed a file with `base.adapters` and `base ensure` installed nothing: `LoadConfig` copies stored base fields one by one | the field is copied, and a case now fails for any base field the loader drops |
| the host half compared the door's answer with an empty version: the probe's `version` line was not among its facts | it is, and the check went green |
| `base attach` printed `--config` relative, as typed | it prints the absolute path |
| ⚠ not in herdr's documentation: its Windows client passes `HERDR_SESSION` to the remote bridge as `--session`, so a client inside a named local session attached to a new remote session of that name | recorded; the tool's line sets no session |
| ⚠ not in herdr's documentation: after prefix then q, the Windows client printed `Error: Os { code: 104, kind: ConnectionReset }` while the server logged a clean detach | recorded |
| ⚠ `herdr pane read` on Windows returned nothing for a pane running herdr's own full-screen client, so the Windows key presses could not be read back | the close keys were measured with a Linux client under tmux instead |

⭐ **Measured, and now in the manual's table:** 68.7 s for `base recreate` with the
adapter, 3.7 s for the next ensure; a 0.2 s SSH round trip; the Windows client
connected and detached through the block the tool wrote; after `wsl --terminate`, one
SSH connection restarted the distribution, the server and the workspaces in 5.4 s;
twelve minutes idle with the base still running; and in two sessions made alike,
herdr's own keys closed a pane and then a tab while the tracked file closed nothing.

### Still open

1. Muse through herdr: the prove's two commands. They need the Muse adapter from
   `WSL-77` and the operator's sign-in on `wsl-toolkit-base`, so they run once that
   base exists.
2. The attach line reaching the same server with `herdr agent read muse`, and the
   close keys in Muse's own pane, after 1.
3. `examples/muse-code/README.md` rewritten from that drive, and
   `examples/common/zellij.md` removed with every link to it.
4. The three reviews, and the closing.

## Amendment, 2026-09-15: the reference sweep, and what it corrects in this entry

⭐ **`herdrdev/herdr` was cloned and read at commit
`052779c4159ed851`, with its tracker**, alongside the two
third-party Muse plugins and nine other bridges. The sweep is
[`../docs/reference-sweeps/findings.md`](../docs/reference-sweeps/findings.md)
and its contract half is
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md). ⛔
**Nothing in it was run**, and this entry still closes on measurement taken here.

### ⛔ Three things this entry did not know, and the first one blocks the prove

1. ⛔ **herdr 0.9.0's Windows `--remote` client repaints only on
   window-activation events, and prefix commands never take effect.**
   `herdrdev/herdr#4176`. ⛔ **It was closed `not_planned`, and that is not the
   same as fixed**: nothing in the tracker says any release repairs it, so a newer
   herdr may behave identically. The record's host state says the operator
   installed **0.9.0**, and the adapter pins 0.9.0. ⚠ **The prove's second passing condition - the attach line reaching the
   same server - is against exactly that client on exactly that version.** The
   next session checks the installed version against that issue before it
   concludes anything about the door, the key or `sshd`.
2. ⛔ **A musl base is the wrong host for a herdr server.** `#4174`, open: the
   0.9.0 Linux server aborts in a musl malloc integrity check and **every pane
   child dies**, with restore bringing back empty shells. `wsl-toolkit-base.json`
   already chooses `arch`, which is glibc; this is why that is not a preference.
3. ⛔ **`events.subscribe` silently drops events above roughly 500 in flight, with
   no gap indication.** `#4178`, open. Anything built on a subscription
   reconciles against `agent list` rather than trusting the stream.

### ⛔ The upstream Muse integration was written, and closed unmerged

| pull request | state |
| --- | --- |
| `#4163` report Muse panes awaiting background agents as working | closed, ⛔ not merged |
| `#4164` Muse hook install plumbing | closed, ⛔ not merged |
| `#4165` Muse target in registry, CLI, resume and authority | closed, ⛔ not merged |
| `#4166` docs: cover Muse integration | closed, ⛔ not merged |

All four by `ohk`, closed **2026-09-15T02:38:16Z**, and ⚠ **no human comment
states a reason**. ⭐ **So this entry must not wait for upstream and must not
assume `herdr integration install muse` exists.** What it can use is the design
the closed bodies describe, which is more than any third-party plugin publishes:
`assets/muse/herdr-agent-state.{sh,ps1}` - ⭐ **a reporter with a PowerShell
half** - source `herdr:muse`, a subagent-safe session claim, `MUSE_HOOK_EVENTS`,
an install that **merges into Muse's `settings.json`**, and resume by
`muse resume <uuid>`.

### ⭐ The premise table's Muse row is right, and smaller than it reads

herdr ships `src/detect/manifests/muse.toml`, version `2026.08.26.1`, so **Muse
panes are already classified `idle`, `working` and `blocked` with no integration
at all**. The gap is lifecycle authority and session identity, not detection.
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md)
section 5 carries the manifest's measured description of Muse's UI, including the
rule that matters most:

⛔ **Every approval rule needs a PAIR of phrases**, because "Muse can emit any one
of these phrases as ordinary assistant text after a completed turn". ⭐ That is
this repository's own forbidden pattern - a check satisfied by the command's own
echo - met independently in somebody else's detection rules.

### ⭐ A third Windows surface this entry never considered

The decision's three forks are all about an interactive client. There is a fourth
thing, and for "the operator watches the agents from Windows" it is the better
one:

```powershell
herdr --machine base agent list
herdr --machine base agent prompt w1:p1 "..."
```

⭐ **`herdr --machine <label-or-id>` routes one API command to a saved SSH machine
with no terminal UI open at all**, over non-interactive SSH. ⭐ **And herdr states
that those payloads are not interpolated into the SSH shell command** - the same
rule this tool reached on 2026-09-09 for every guest payload, reached
independently by somebody else.

⚠ **Two herdr documents disagree about Windows.** Its capability table calls
saved SSH machines supported; its connecting-machines page says multi-machine
connections are **not yet verified or supported on a Windows client**, with
standalone `--remote` still supported. ⛔ Neither was measured. The next session
measures `--remote`, `--machine` and `machine add` on this host and records which
sentence held.

What `--machine` will and will not carry, which shapes any tooling built on it:

- forwarded: `workspace`, `worktree`, `tab`, `pane`, `notification`, `agent`
  (⛔ except `attach`), `api snapshot`, `status server`, and API-backed plugin
  `link`/`unlink`/`enable`/`disable`/`list`/`action`/`log`/`pane`;
- ⛔ **not forwarded: plugin INSTALLATION**, session management, local
  configuration, interactive attach;
- ⛔ the selector is a saved profile id or a **unique, case-sensitive label**, not
  an SSH hostname; combining it with `--session` or `--remote` is an error;
- ⛔ local pane ids are not inherited and `--current` cannot mean a local pane.

### ⛔ Four traps the sweep found that this shape walks straight into

1. ⛔ **tmux is invisible to herdr's agent detection**, so a pane that auto-enters
   it loses its agent's state entirely. ⭐ The 2026-09-13 ruling keeps tmux as a
   generic fallback; this is what that must mean in practice. The mechanism is in
   [`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md)
   section 8, and both example pages now warn about it.
2. ⛔ **A launcher is a wrapper, and a wrapper hides the agent.** `HERDR_AGENT=<agent>`
   must be set **on the wrapper command, on the herdr side**; herdr cannot see it
   if it is set only inside a VM or container. ⚠ `base agent NAME` and the
   `NAME.exe` launcher are exactly such wrappers, and `WSL-78` owns them.
3. ⚠ **WSL may not expose a foreground process group**, which is how herdr
   identifies a pane's agent. herdr offers `HERDR_PROCESS_DETECTION=child-groups`
   for "restricted Linux runtimes": read by the **server**, needs a restart, best
   effort. ⭐ **Whether WSL needs it is a one-command measurement** and it belongs
   in this entry's first driven pass.
4. ⛔ **herdr copies nothing onto an SSH host** - no plugin, configuration,
   executable or secret - and "missing remote commands fail visibly". Everything
   the base needs is installed by the adapter, in the base.

### ⭐ The integration contract, if this entry builds one

Should the closing need lifecycle authority rather than screen detection, the
whole contract is in
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md)
sections 1 to 4. The two rules that are not obvious, both of which cost their
finders a pull request:

- ⛔ **`--seq` is strictly increasing per source, and a stale one is accepted by
  the API and IGNORED by the pane state.** Nothing errors. A `release-agent`
  without its own `seq` is silently dropped and the stale row sits in
  `herdr agent list` for ever.
- ⛔ **`muse resume` reuses the session id and never emits `SessionStart`**, so a
  resumed session is invisible without an adoption path. Adopt only on
  `UserPromptSubmit`, `PreToolUse` or `PermissionRequest`, only into an unowned
  pane, and seed the seq from wall-clock.

⚠ **A Muse hook sees none of herdr's environment.** `HERDR_ENV`, `HERDR_PANE_ID`
and `HERDR_BIN_PATH` are not visible to it, which is why one reference patches the
`muse` launcher and the other walks the process tree. ⛔ **And a Muse hook must
never write to stdout**: hook stdout can influence the agent.

### Still open, revised

1. **Measure herdr on this host before anything else**, and in this order: the
   installed Windows version against `#4176`; `--remote`, then `--machine`, then
   `machine add`; and whether WSL needs `HERDR_PROCESS_DETECTION=child-groups`.
2. Muse through herdr: the prove's two commands. They need the Muse adapter and
   the operator's sign-in on `wsl-toolkit-base`.
3. The attach line reaching the same server, and the close keys in Muse's own
   pane, after 2.
4. ⭐ **Done in this session:** `examples/common/zellij.md` is removed, every link
   to it is gone, `examples/common/herdr.md` carries the operator and agent guide
   with the sweep's traps, and `examples/muse-code/README.md` is rewritten from
   eleven commands to three.
5. The three reviews, and the closing.

## Ruled by the operator, 2026-09-14: Muse and herdr stay together after existing work

⭐ **Finish the ordered safety, correctness and issue 30 work first.** The sealed
base is also complete before this entry resumes. Muse and herdr remain one dedicated
session after that work. The operator on Windows, the operator's agent on Windows
and Muse in the base use the same session, with no manual message relay between
them.

What that session owns, read against the entries:

- this entry's four open items, which need `wsl-toolkit-base` and the operator's `muse
  login`, so the later session builds the base from `wsl-toolkit-base.json` first;
- `WSL-78`'s open items: the prove, the guide, and Muse's own screen started in a herdr
  pane at the project's guest path and attached from Windows;
- ⚠ **`pi` and `omp` as herdr agents driving Muse**, which no entry carries yet:
  `WSL-77` names them as the next adapters and builds neither, so the session authors
  them before it builds them.

`WSL-68`, the sealed base, is a dedicated session of its own before this one.

## Amendment, 2026-09-15: the interactive session measures the four, and the reporter meets a real herdr

⚠ **Conditions:** `wsl-toolkit-base`, built this session from
`examples/muse-code/wsl-toolkit-base.json` saved as the instance's own configuration;
herdr 0.9.0 on Windows from scoop and in the base; Muse Code 1.3.0
(`1.3.0-R3057.1`), which the approved installer digest now serves, driven with its
credential-free `--provider echo` because `muse login` had not happened.

### ⭐ The four measurements the sweep named, in order

| # | question | measured |
| --- | --- | --- |
| 1 | the Windows herdr | `herdr 0.9.0`, exit 0. `#4176` is closed `not_planned` as a duplicate of `#4038`, which herdr closed `completed` on 2026-09-13 as fixed on its development branch. ⛔ The four fix commits `#4038` names are 20 to 51 commits after `v0.9.0` and after the newest preview, `preview-2026-09-08-62431dbd033b`, read with GitHub's compare API: **no published herdr carries them** |
| 2 | `--remote`, `--machine`, `machine add` | `--remote`: owed, below. ⛔ `--machine`: exit 2, `unknown option: --machine` - it is not in 0.9.0. `herdr machine add wsl-toolkit-base --label base`: exit 0 in 2.7 s, the profile saved in `%LOCALAPPDATA%\herdr\client\endpoints.json`, nothing installed in the base, no second server, and a workspace `w2` created on a server that had none |
| 3 | a foreground process group in WSL | ⭐ **exposed.** A pane running `sleep 300`: herdr's `pane process-info` answered `foreground_process_group_id` 3644 and `ps` answered `TPGID` 3644 on `pts/7`, so `HERDR_PROCESS_DETECTION=child-groups` is not needed |
| 4 | Muse's identity through this tool's wrapper | ⭐ **kept.** `muse --provider echo --trust-workspace` in a pane: herdr named the agent `muse` from the process `muse-bin-1.3.0-`, argv `~/.local/bin/muse-bin-1.3.0-R3057.1` of the account, because every hop `exec`s. ⚠ `agent explain`: manifest `2026.08.26.1`, `rule: none`, `fallback_reason: default_known_agent_idle_fallback` - no screen rule matches Muse 1.3.0's input screen |

⛔ **What the sweep got wrong, and why:** it read herdr's `docs/next`, the pages for
the unreleased version. `docs/versions/0.9.0` agrees with itself that multi-machine is
not verified or supported on a Windows client, and has no `--machine`.
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md) carries the
row-by-row correction, including two more: herdr's Linux release binary is static and
carries musl's allocator itself, so the base's libc does not avoid `#4174`; and no
update exists to apply for `#4176`.

### ⛔ The reporter's first real run found four defects, and none was visible to the stub

`base ensure` from nothing **exit 2 in 92.34 s**: herdr healthy, Muse 1.3.0 installed,
then `/tmp/tmp.X3WzEiEIg6: Permission denied` merging the reporter into
`~/.muse/settings.json`.

| defect | measured | fixed |
| --- | --- | --- |
| root's redirect into a temporary file chowned to the account | systemd's `/usr/lib/sysctl.d/50-default.conf` sets `fs.protected_regular = 1`; root's write into a herdr-owned file in `/tmp` refused, into a root-owned one allowed | the temporary file stays root's until `install -o` places the result |
| the wrong file | hooks in `~/.config/muse/settings.json` ran; in `~/.muse/settings.json` and a workspace's `.muse/settings.json` they were ignored, every shape | `MUSE_SETTINGS` is `~/.config/muse/settings.json`, and the probe reads the same file |
| the wrong shape | a command straight under an event never ran; inside `{"matcher":"","hooks":[...]}` it ran for `SessionStart`, `UserPromptSubmit`, `Stop`, `SessionEnd` | the merge writes one matcher group per event and drops only our old commands |
| the `{}` a new file got | Muse exit 1, `malformed settings file ... missing field schema_version` | a new file carries `schema_version: 1`, and one without it stops the ensure untouched |

⭐ **Proved:** `TestTheHerdrReporterIsRegisteredWhereMuseReadsIt`, the first case to
reach the registration branch, passes in `golang:1.25` with `jq` and skips on
Windows; **5 new mutation rows, and `repo mutate --only muse:` 8 of 8 guards proved in
`golang:1.25` in 34.3 s**. Over the fix, `base ensure` exit 0 in 4.5 s, the muse
adapter healthy with `herdr_reporter_events` naming all six events.

### ⭐ The lifecycle, against herdr 0.9.0 and Muse 1.3.0, in a pane of the base

| step | the reporter's log | herdr |
| --- | --- | --- |
| the first prompt, by `herdr agent prompt` | `reported idle seq=1`, `working seq=2`, `idle seq=3`. ⚠ `SessionStart` arrived with the first submit, not at start | `state_change_seq` 1 to 3 |
| a second prompt, with `herdr agent wait --until working` waiting | `reported working seq=4` at 10:57:04Z | the wait returned `working`, exit 0, 0.6 s after the submit, with no screen rule matching |
| `/exit` | `reported idle seq=6`, `released seq=7`, the bindings file empty | `agent list` empty |
| `muse resume ID`, then a prompt | no `SessionStart`; `adopted w1:p1 ... at seq 1789469866`, `working`, `idle` | accepted above the release |

⚠ **Read in herdr's source at `v0.9.0`, `src/detect/mod.rs` and
`src/terminal/state.rs`:** a full lifecycle authority, which turns screen detection
off, is an allowlist of six `herdr:` sources. A `custom:` report is always effective
for the pane's state, and a blocker on the screen still overrides it to `blocked`.
`PreToolUse` and `PermissionRequest` are registered and not driven: the echo provider
calls no tool.

### herdr built from its development branch, by the operator's instruction of 2026-09-15

The operator: "let's build herdr ourself (now locally for windows) and if it works, we
will create a dedicated nightly builder for it on github and publish it on our repo".
At `052779c4159ed851`, with herdr's own release settings:

| target | result |
| --- | --- |
| `x86_64-pc-windows-msvc`, Rust 1.96.1, Zig 0.16.0, VS Build Tools 2022 | exit 0 in 227 s, `herdr.exe` 25,253,888 bytes, SHA-256 `88357283...736e3507`, staged with the ConPTY bundle whose three files match the development branch's pins |
| `x86_64-unknown-linux-musl` in `rust:1.96.1-bookworm`, Zig's tarball checked against its published digest | exit 0 in 223.4 s, 26,059,064 bytes, SHA-256 `978fde51...9827d75` |
| both, `--version` | ⚠ `herdr 0.9.0`: the development branch has not moved its version, so a build cannot be told from the release by asking it |
| the Windows build, `--machine base agent list`, against the 0.9.0 server | exit 1 in 0.87 s, `remote Herdr does not support machine API forwarding` |

⛔ **Publishing a herdr build from this repository contradicts `docs/AGENTS.md`
section 1**, which says one thing is published from here. That is the operator's to
rule on, in an entry of its own.

### Still open, revised

1. `--remote` measured by the operator in Windows Terminal, with the 0.9.0 client and
   then the development build, against `#4176`'s signals: repaint, typed text, a
   prefix command, resize, detach.
2. `--machine` end to end, which needs the development build in the base too.
3. Muse through herdr: the prove's two commands, after `muse login`.
4. The attach line reaching the same server, and the close keys in Muse's own pane.
5. `PreToolUse` and `PermissionRequest` reported from a real turn.
6. The three reviews, and the closing.

---

## WSL-77. A provider base rebuilt from a clone in one command, with herdr and Muse as its first adapters

**Source** [issue 32](https://github.com/Azathothas/ToolKit/issues/32) item 4,
filed by the operator on 2026-09-13. Their correction the same day puts herdr where
the item says Zellij.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** done

---

## Problem

The Muse base was built by hand, in the order `WSL-69` records: a profile, `base
ensure`, the multiplexer from pacman, the bootstrap with a `--without` list, the
Muse installer run by the operator, and `muse login`. Nothing rebuilds it. Another
agent CLI, `pi` or `omp` for example, or several agents in one base, means writing
that order again. The operator wants a clone of this repository and one command to
rebuild the base, the login excepted, with the multiplexer and each agent CLI as a
pluggable adapter.

## Premise

- ⭐ **Every step, its order and the two operator steps are measured**, in
  `WSL-69`.
- ⭐ **`base ensure` is already the reconciler and `base status --probe` the
  proof.** `WSL-67` extended `cmd_base_preset.go` rather than fork a second
  provisioning path.
- ⛔ **The Muse installer is mutable**, and the launcher it fetches is checked
  only when the server sends a digest. [`../docs/security/remote-ops.md`](../docs/security/remote-ops.md)
  refuses running a fetched script unread, so an unattended rebuild cannot simply
  run it.
- ⚠ **Several agents in one base agrees with the ruling that agents with
  different authority get separate instances**, when they share one authority:
  one account and one herdr server. Agents that need different authority still
  need instances of their own.

## Approach

1. **An adapter is a directory with a fixed contract:** what it installs and as
   which account, what it pins and how it verifies it, what it depends on, the
   command that proves it, and the steps only the operator can do.
2. **Adapters are declared in the instance's configuration, applied by `base
   ensure` after provisioning, and each one reports in `base status --probe`.**
3. **herdr, from `WSL-76`, and Muse are the first two.** ⛔ Muse's adapter saves
   the installer, prints its length and SHA-256, and runs it only when that digest
   equals one the operator passed after reading the file, as the bootstrap's
   `--expect-sha256` does. Otherwise it stops and prints the file's path and the
   command that continues. `muse login` stays the operator's, printed at the end.
4. **The one command:** an example profile under `examples/muse-code/` that names
   the adapters, run with `base ensure` from a build of the clone.
5. `pi` and `omp` are named as the next adapters and not built.

⛔ **No adapter registry and no adapter fetched from outside the tree.**

## Decision

⭐ **Ruled by the operator on 2026-09-14: A**, the definitions in the tree and an
embedded generated copy a gate rule compares byte for byte, with the pinning as
recommended.

⭐ **The operator approved one digest for Muse's installer the same day.** The agent
fetched `https://dev.meta.ai/install.sh`, read it and did not run it: 314 lines,
9,314 bytes, SHA-256 `5196d820…632a0ca`, the file run on 2026-09-13. It fetches
`https://api.meta.ai/muse-launcher.sh`, checks that file's SHA-256 only when the
server sends one, installs `~/.local/bin/muse`, runs it to download the binary, and
appends `PATH` lines to shell profiles, with no root. The adapter runs the installer
only while its digest is that value; any other stops and asks.

Where adapters live and what applies them:

- **A. Declared in the configuration and applied by `base ensure`. Recommended.**
  The definitions live in the tree, and the executable embeds a generated copy
  that a gate rule compares byte for byte, the precedent `RULES.md` section 4 sets
  for the package table. One reconcile path and one proof path.
- **B. A script beside the tool that runs `base exec --script` per adapter.** Less
  code, and a second provisioning path the probe cannot see.

A second fork: how an adapter pins. Recommended: a digest in the tree where
upstream publishes a release digest, as herdr does; a resolve-and-print where it
does not, which proves transport only and says so; and never Muse's installer
without the operator's digest.

## Consumers

None until an adapter reaches a fetched file. A new configuration key is
additive.

## Prove

From a fresh clone, on a throwaway instance whose profile names that instance's
distribution, never `wsl-toolkit-muse`:

```powershell
wsl-toolkit --instance NAME --config PROFILE base ensure
wsl-toolkit --instance NAME --config PROFILE base status --probe --json
```

Passing is:

- the first run stops at the Muse installer with exit 2, printing its digest;
- run again with that digest, `base ensure` exits 0, and the probe reports each
  adapter healthy with its version;
- `herdr --version` and `muse --version` answer through `base exec`;
- `base remove --yes` removes the instance afterwards.

## Amendment, 2026-09-14: the Muse adapter and the one base's profile, built

⭐ **Built, under the ruling:**

- `adapters/muse/`. `install.sh` saves the installer Meta serves, prints its length
  and SHA-256, and runs the saved file as the account, with `MUSE_NO_MODIFY_PATH`,
  only while the digest is the approved one, pinned as
  `MUSE_INSTALLER_PINNED_SHA256`, or the `installer_sha256` the configuration's
  `muse` entry carries. Any other stops with exit 2 and keeps the file at
  `/var/lib/wsl-toolkit/muse/install.sh`, with the command to read it. A Muse that
  answers a version is not installed again. A wrapper at `/usr/local/bin/muse` makes
  the name resolve in `base exec`, and turns away any account but the base's own.
  `probe.sh` reports the version, asked with `MUSE_NO_AUTO_UPDATE`, the wrapper, where
  `muse` resolves, and whether a credential file is present.
- `base.adapters[].installer_sha256`, the approval approach point 3 names: refused on
  an adapter that runs no installer and when it is not 64 lowercase hex characters,
  and passed to `install.sh` as `TK_INSTALLER_SHA256`. `config` prints it.
- [`../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json`](../tools/windows/wsl-toolkit/examples/muse-code/wsl-toolkit-base.json),
  the one base of the operator's ruling with the `herdr` and `muse` adapters, and the
  install section of the Muse guide rewritten to use it.
- `pi` and `omp` named in the adapter contract as the next adapters, not built.

⭐ **Read before it was built, on 2026-09-14, and not run by hand:**

- the installer Meta serves still has the approved digest, and is 314 lines and 9,314
  bytes. It needs bash, curl and mktemp, installs the launcher at `~/.local/bin/muse`,
  checks the launcher's SHA-256 only when the server sends `x-content-sha256`, runs
  it once to fetch the binary, and appends `PATH` lines to profiles unless
  `MUSE_NO_MODIFY_PATH` is set;
- the launcher, 1,138 lines and 33,118 bytes, sent with a matching
  `x-content-sha256`. It looks for an update at most hourly unless
  `MUSE_NO_AUTO_UPDATE=1`, and a download it is refused starts a device sign-in only
  when stderr is a terminal or `MUSE_LOGIN=1`.

⭐ **Measured without credentials:** the channel manifest and the release manifest
both answered 200, the channel `"state":"public"` at version `1.2.1-R2847.1`, and the
Linux x86 artifact answered a one-byte range with 206. So an unattended install needs
no Meta account, and the operator's sign-in is needed only to use Muse.

⚠ **The prove's first condition was written for a digest passed at run time.** Under
the ruling the approved digest is pinned, so a fresh clone installs Muse without
stopping while Meta serves that file. The stop is held by the shell case below, and on
a real base by a build whose pin is planted to differ.

| case | Windows, `TEMP` at the 8.3 path | `golang:1.25` |
| --- | --- | --- |
| `TestAnInstallerDigestIsTakenOnlyByAnAdapterThatRunsAnInstaller` | pass | pass |
| `TestAnApprovedInstallerDigestReachesInstallSh` | pass | pass |
| `TestTheMuseInstallerRunsOnlyWhileItsDigestIsApproved`, `install.sh` against stand-ins under a POSIX shell | skip | pass |

⭐ **Six mutation rows went red**, each after its case passed unmutated: the refusal on
an adapter with no installer, the refusal of a malformed digest and the variable that
carries it, on Windows; the unapproved installer kept and not run, the wrapper's
account check and the installed Muse not fetched again, in `golang:1.25`.

---

## Closing

**Closed 2026-09-14T10:31:57Z.** A clone of this repository and one `base ensure`
rebuild the provider base with herdr and Muse, the sign-in excepted. The prove, from
a fresh clone of `main` at `d8c8328`, on the throwaway instance `m77`, whose profile
is `wsl-toolkit-base.json` with its name changed, and with `WSL_TOOLKIT_SSH_DIR` under
this repository's `.tmp`. The digests are shortened, because the tree refuses a long
hex identifier:

```text
wsl-toolkit --instance m77 --config profile-m77.json base ensure
exit 0 in 77.6 s
  adapter herdr: healthy, version 0.9.0
  adapter muse: installing
    * saved Meta's installer at /var/lib/wsl-toolkit/muse/install.sh: 9314 bytes, SHA-256 5196d820…632a0ca
    * running it as herdr, approved by the digest this adapter pins
    * installed Muse Code 1.2.1 (1.2.1-R2847.1) for herdr
    * wrote /usr/local/bin/muse, which runs Muse as herdr
    * signing in is the operator's: wsl-toolkit --instance m77 base shell, then muse login
  adapter muse: healthy, version 1.2.1 (1.2.1-R2847.1)

a build of the clone with its pin changed, after the launcher was removed
exit 2 in 4.4 s
  muse adapter: the installer Meta serves now is not one the operator approved, so it was not run.
    saved at    /var/lib/wsl-toolkit/muse/install.sh, 9314 bytes, SHA-256 5196d820…632a0ca
    read it     wsl-toolkit --instance m77 base exec --root -c 'cat /var/lib/wsl-toolkit/muse/install.sh'
    approve it  add "installer_sha256": "5196d820…632a0ca" to the muse entry in base.adapters, then run base ensure again

the same build, with that digest as installer_sha256
exit 0 in 5.6 s
    * running it as herdr, approved by the installer_sha256 in this base's configuration

base status --probe --json               exit 0, healthy, no problem; herdr 0.9.0 and muse 1.2.1 (1.2.1-R2847.1) healthy
base exec -c 'herdr --version'           herdr 0.9.0, exit 0
base exec -c 'muse --version'            Muse Code 1.2.1 (1.2.1-R2847.1), exit 0
base exec --root -c 'muse --version'     muse is installed for herdr, and runs only as herdr, exit 126
base remove --yes                        exit 0 in 10.5 s
```

| condition | measured |
| --- | --- |
| the first run stops at the installer with exit 2, printing its digest | on the build whose pin differs: exit 2 with the digest, the file kept and not run, and the launcher still absent afterwards. ⚠ The fresh clone's own first run installed, because its pin is the digest Meta serves; the amendment above says why |
| run again with that digest, exit 0, each adapter healthy with its version | exit 0, approved by the configuration; the probe read both adapters healthy with their versions |
| `herdr --version` and `muse --version` through `base exec` | above |
| `base remove --yes` removes the instance | exit 0, and `wsl -l` read the four distributions of the session's start. The instance directory, which held nothing, and everything under `.tmp` were deleted by hand |

A fourth `base ensure` over the installed Muse exited 0 in 3.8 s without fetching the
installer. The operator's `%USERPROFILE%\.ssh\config` read SHA-256 `18FC11BE…E93F4`
before and after, and the door's own block went into, and out of, the file under
`.tmp`.

### The reviews

⭐ **The door sweep** followed the new key and the new file. `installer_sha256` is
read by `LoadConfig`, `Validate`, `adapterInstallEnv` and `config`, and `config
--write` writes it back; the helper route, `ready --ensure` and `base recreate` reach
the installer through the one `applyAdapters`, and the probe needs no key. The saved
installer is root's, in a directory root owns, so the account cannot change it
between the digest check and the run. ⚠ The profile's passwordless sudo gives the
account root anyway, which the access profiles already call authority and not
containment. ⛔ **It found one gap:** `LoadConfig` copies `base.adapters` whole, and
no case held a field inside an adapter surviving a load, which is the shape of the
defect `base.adapters`' first drive found. The case now carries one, and a row that
copies the names alone went red.

⭐ **The guard mutation proved 7 rows**, each after its case passed unmutated: the
three configuration rows and the loading row on Windows, and the three `install.sh`
rows in `golang:1.25`. The planted build proved the stop on a real base.

⭐ **The claim audit** corrected three sentences. The manual and the adapter contract
said the operator approved the digest after reading the file; the record says the
agent read it and the operator approved it. The manual said Muse's launcher updates
it; that is read in the launcher and not measured, and it now says so. And Meta's
installer tells a reader that `~/.local/bin` is not on `PATH`, which a first-time
reader would act on; the guide says why nothing needs adding.

---

## WSL-78. Muse from any Windows project, and a guide for someone who has never used a coding agent

**Source** [issue 32](https://github.com/Azathothas/ToolKit/issues/32) item 5,
filed by the operator on 2026-09-13.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** open

---

## Problem

Muse runs only inside its base, through `base exec`, against its one granted
checkout. An agent on Windows working in another project cannot call it. And the
operator has never used a terminal coding agent: they want every Muse option that
matters, reasoning effort included, run and written down in ASD-STE100 Simplified
Technical English.

## Premise

⭐ **Measured on 2026-09-13 from Muse Code 1.1.1's own help**, read-only through
`base exec`:

- `--reasoning-effort none|minimal|low|medium|high|xhigh|max|ultra`, default
  `high`, on both `muse` and `muse exec`;
- `--model`, `--preset native-basic|miniswe`, `--approval-mode
  untrusted|on-request|never` with `on-request` the default, `--approval-judge`,
  `--permission-profile` and `--worktree`;
- the safety switches `--yolo`, `--trust-workspace`, `--disable-approval`,
  `--disable-sandbox`, `--sandbox-network restricted|enabled|proxy-only`,
  `--disable-write` and `--disable-shell`;
- `muse exec` adds `--json`, `--prompt-file`, `--max-model-steps`, the
  `--context-compaction-*` settings, `--session-id` and `--disable-web-tools`;
- the subcommands `resume`, `exec`, `config`, `export`, `trace`, `skills`,
  `sandbox`, `schema`, `serve`, `session-message`, `mcp`, `auth`, `login`,
  `logout` and `init`.

⛔ **Muse is not Linux-only, so issue 30's premise does not hold for it.** Meta
serves `https://dev.meta.ai/install.ps1`, 8,367 bytes, read and not run. It puts
`muse.cmd` and a launcher in `%LOCALAPPDATA%\Programs\muse`, or `MUSE_INSTALL_DIR`,
adds that directory to the account's `PATH` unless `MUSE_NO_MODIFY_PATH` is set,
and checks the launcher's SHA-256 only when the server sends one. Muse 1.1.1 in the
base also carries `muse sandbox windows check|setup`. ⚠ Whether the Windows build
works was not measured.

⚠ **A native Windows Muse runs as the Windows account.** Issue 30's rule that it
reach no directory it was not given would then rest on Muse's own sandbox, which
nobody here has measured.

## Approach

1. The decision below first.
2. **Under A, a `muse` entry point on Windows that runs Muse in the base for the
   caller's project.** It maps the working directory to its grant from `WSL-75`,
   refuses an ungranted directory and prints the command that grants it, passes
   arguments through unchanged, and forwards the exit code. Headless `muse exec`
   goes through `base exec`; the interactive screen goes through herdr, `WSL-76`.
   ⛔ No second path into the guest.
3. **The guide**, beside the Muse example: short sentences, one instruction each,
   in the imperative, written to ASD-STE100's rules without copying its
   specification. Every command in it is run and its output read before it is
   written. It covers checking the install, signing in, the first session, the
   workspace trust question, reasoning effort, the model, the approval modes, what
   each safety switch allows, `exec --json`, `resume` and `export`, skills, the
   herdr pane, and stopping without losing work.
4. Each option names the Muse version it was measured on, because Meta changes
   them.

## Decision

⭐ **Ruled by the operator on 2026-09-14: A**, and wider than Muse: "muse and most
of these other agents, even if we install them on windows, they will again have to
use wsl-toolkit anyway because they need Linux because that's where they work best.
So these agents should run inside wsl-toolkit base to begin with". The agent may
write the launchers into `%USERPROFILE%\bin`, which is already on `PATH` beside
`wsl-toolkit.exe` on this host.

- **A. Muse stays in the base, and Windows gets an entry point. Recommended.**
  Issue 30's boundary holds, and one install serves the operator and every agent
  on Windows.
- **B. Meta's Windows installer, run natively.** Simpler, and Muse reaches
  whatever the Windows account can. herdr runs locally on Windows, and a Windows
  host cannot be a `herdr --remote` target.

## Consumers

None: a Windows entry point and a guide. If the entry point ships inside the
executable, it is additive.

## Prove

From a granted project directory on Windows:

```powershell
muse exec --json --reasoning-effort low --approval-mode never "Answer with only the number of files git tracks here."
```

Passing is:

- exit 0, JSON events on stdout, and the number equal to what `git ls-files`
  counts on Windows;
- from an ungranted directory, exit 2 and the grant command printed;
- a transcript of every command in the guide, run in the order the guide gives.

## Amendment, 2026-09-14: the entry point and its launcher, built and driven without a sign-in

⭐ **Built, under the ruling:**

- `wsl-toolkit base agent NAME [--] ARGS`, for an adapter that names an `Agent`
  command. It maps the working directory to the grant that covers it, a grant
  covering its directory and what is beneath it and no sibling whose name starts the
  same way; refuses an ungranted directory with exit 2 and the `base grant` line;
  runs the command as the account at that guest path through `base exec`'s framed
  channel, with every argument single-quoted; and forwards the exit code. `muse` with
  no argument, and `muse resume`, answer exit 2 naming the herdr route.
- **The launcher is this executable under the agent's name**: `muse.exe` in
  `%USERPROFILE%\bin` reaches the instance `base`, and `muse-NAME.exe` the instance
  `NAME`. It is the `muse` adapter's half on this machine, so `base ensure` writes
  it, `base status --probe` names one that is another build, and `base remove` takes
  it away. It never writes over or removes a file that is not a build of this tool,
  read from Go's build information without running it.

⛔ **Why an executable and not a script:** `forbidden-patterns.md` records that
`cmd.exe` does not parse the argument list its callers produce, so a `.cmd` launcher
would read an agent's prompt again, and an ampersand in one would start a second
command. A `.ps1` reaches PowerShell callers alone. A copy of the executable takes its
arguments as any program does, and the price, a copy an update leaves behind, is
named by the probe.

⭐ **Driven on the throwaway `wsl-toolkit-m78`**, its profile in the instance's own
`config.json`, with `WSL_TOOLKIT_BIN_DIR` under this repository's `.tmp` and the
grant a throwaway git project there:

| measured | result |
| --- | --- |
| `base ensure` from nothing, with the muse adapter | exit 0 in 76.6 s, `muse-m78.exe` written, 14,200,320 bytes |
| `base grant --source .tmp\wsl78\proj --mode rw` | exit 0, mounted live |
| `muse-m78.exe --version` in the project, then in a directory beneath it | `Muse Code 1.2.1 (1.2.1-R2847.1)`, exit 0, both |
| `muse-m78.exe --version` in a directory no grant covers | exit 2, and the `base grant --source` line for that directory |
| `muse-m78.exe` with no argument | exit 2, naming `base attach` and the guest path |
| `muse-m78.exe --definitely-not-a-flag`, read unpiped | exit 2, the code `base exec` read from Muse for the same argument |
| `muse-m78.exe exec --help` | exit 0, Muse's 91-line help |
| a launcher from an earlier build of the tree | the probe exit 1 naming it; `base ensure` rewrote it, byte for byte this build, and the probe exit 0 |
| `base revoke`, then `base remove --yes` | exit 0 both, and the launcher removed with the distribution |

⚠ **Driving it found two messages worth rewriting, and they are:** the refusal
repeated itself, `not granted to the base: ... is not granted to`, and the herdr
route said "the lines this prints" beside a command that prints them.

⚠ **Not driven:** the launcher in the account's own `%USERPROFILE%\bin`, which the
drive replaced with `WSL_TOOLKIT_BIN_DIR` so the operator's directory stayed as it
was, and an argument carrying quotes or a dollar sign from a Windows command line to
the guest. The quoting is held by the Linux case, and Windows parses a program's
arguments by the rules every executable uses.

### The reviews so far

⭐ **The door sweep** found two ways to the agent run, `base agent` and the launcher's
name in `main`, both through `cmdBaseAgent`, and the helper route refusing it. A
launcher names its instance, so the environment and a project's pointer file cannot
send it elsewhere, and `WSL-74`'s refusal still holds its configuration. The
launcher's removal runs on every ensure of an instance that names no agent, and it
reaches only that instance's own launcher name. What would have made it fire: a
second path that runs an agent without the grant mapping.

⭐ **The claim audit** corrected the manual's "every argument reaches Muse as
written", which the drive does not reach end to end; it now says each argument is
quoted for the guest's shell. The third review is the guard mutation above.

| case | Windows, `TEMP` at the 8.3 path | `golang:1.25` |
| --- | --- | --- |
| `TestAGrantCoversItsDirectoryAndWhatIsBeneathItAlone` | pass | pass |
| `TestAnAgentRunsOnlyForAnAdapterThatIsOne` | pass | pass |
| `TestAnAgentsOwnScreenIsNotStartedWithoutATerminal` | pass | pass |
| `TestALauncherNameCarriesItsInstance` | pass | pass |
| `TestALauncherIsWrittenOnlyOverOneOfThisToolsBuilds` | pass | pass |
| `TestAnAgentsArgumentsReachItAsWritten`, quotes, a dollar sign, a backtick, a glob and an empty argument | skip | pass |

⭐ **Seven mutation rows went red**, each after its case passed unmutated: the path
boundary, an adapter that is not an agent, the screen rule, the launcher's instance
suffix, the refusal to write over and to remove a file that is not this tool's, on
Windows; and the quoting, in `golang:1.25`.

### Still open

1. The prove, and the guide, whose every command is run before it is written. Both
   need Muse signed in, so both wait for `wsl-toolkit-base` and the operator's
   `muse login`.
2. The interactive screen through herdr: starting Muse in a pane at the project's
   guest path and attaching from Windows, with `WSL-76`'s closing drive.
3. The closing, with the three reviews run again over what that drive adds.

## Amendment, 2026-09-15: what the reference sweep adds, and one trap it closes

The sweep behind this is
[`../docs/reference-sweeps/findings.md`](../docs/reference-sweeps/findings.md),
commits and all; the contract half is
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md). ⛔
**Nothing in it was run**, and every claim carries the version it was read
against.

### ⛔ The trap this entry's own launcher walks into

⛔ **A launcher is a wrapper, and a wrapper hides the agent from herdr.** herdr
identifies a pane's agent from its foreground process, and its agents page says
plainly: a host-visible wrapper can hide the real agent. The fix herdr documents
is `HERDR_AGENT=<agent>` **on the wrapper command**, and it is explicit about the
half that gets it wrong - "Herdr cannot see it if you set it only inside a VM or
container."

⚠ **`base agent muse` and the `muse.exe` launcher are both exactly such a
wrapper**, and the `wsl.exe` hop is the container boundary that sentence is about.
So the setting has to be made **on the herdr side**, in the pane herdr owns, not
inside the base by the launcher. ⛔ **This was not measured**, and it is the first
thing the interactive half of item 2 should measure, because a Muse screen that
herdr reports as `unknown` for ever is the failure this predicts.

### ⭐ herdr can start Muse itself, which shrinks item 2

⭐ **`muse` is a supported `--kind`.** herdr's agent-automation page lists 24 of
them and Muse is one:

```bash
herdr agent start muse --kind muse --pane "$pane" -- --reasoning-effort high
```

| ⛔ | |
| --- | --- |
| `agent start` needs an **available shell pane at its prompt** | it never creates, splits or moves layout |
| the name must match `[a-z][a-z0-9_-]{0,31}` | and be unique among live agents |
| 30 s default, `--timeout` 3000-300000 ms | `agent_not_ready` if detection reports blocked during startup |
| capture ids from the JSON | `pane split` returns `.result.pane`, `workspace create` returns `.result.root_pane` |

⭐ **So "Muse's own screen started in a herdr pane at the project's guest path"
is one herdr command against a pane created at that path**, not a terminal to
script. ⚠ Whether it survives the `wsl.exe` wrapper is the measurement above.

### ⭐ Muse's real interop surface, corroborated three ways

⛔ **This entry's premise reads Muse's `--help` and stops at flags.** Three
independent references name a protocol:

| reference | what it says |
| --- | --- |
| `ibchouti9/openmuse` | talks to the real `muse` binary over its **MSP session protocol**, `muse serve` over **stdio**, speaking **JSON-RPC** |
| `LimpingNinja/omp-muse-bridge` | a **persistent `muse serve` host** keeps backend context between turns |
| `danny-hines/muse-code-bridge` | requires **Muse Code 1.0.3+ and Muse Session Protocol v1** |

⭐ **That is a far better surface than sending keys and reading a screen**, and it
is the one item 1's guide should prefer wherever a command has to be scripted
rather than typed.

⛔ **And this entry already recorded the obstacle**: `base exec` gives a command
`/dev/null` as stdin, `cmd_base_exec.go:18`, so a stdio protocol cannot pass
through it. ⚠ **That is now a design question with a name**, not a note: either a
`base` surface that keeps stdin open for a stdio protocol, or `muse serve` reached
from inside the base by something that is already there. Neither is built and
neither is ruled.

### ⭐ Muse's lifecycle hooks, and what they are worth to a guide

Muse emits `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PermissionRequest`,
`Stop` and `SessionEnd`, each a **command hook reading a JSON payload on stdin**,
carrying `session_id`, `turn_id`, `cwd`, `transcript_path`, `model` and
`permission_mode`.
[`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md)
section 3 carries the payloads.

⛔ **A Muse hook must never write to stdout** - hook stdout can influence the
agent - and every failure path exits 0. ⚠ **The `PermissionRequest` payload is the
one shape nobody has confirmed**; its own publisher marks the fixture
`INFERRED SHAPE - not yet observed live`.

⚠ **`muse skills install` plus a `settings.json` merge is how a third party
installs into Muse**, because `siddicky/oh-my-musecode` measured that **Muse 1.1.1
reports plugins are unavailable in that build**. herdr's own unmerged plumbing did
the same `settings.json` merge. A guide that tells a first-time operator to
install a Muse plugin would be telling them to do something their build refuses.

### ⚠ Every Muse version in evidence disagrees

| source | version |
| --- | --- |
| herdr's shipped detection manifest | measured against **0.2.1** |
| `danny-hines/muse-code-bridge` | requires **1.0.3+** |
| `siddicky/oh-my-musecode` | measured **1.1.1** |
| this entry's own premise | **1.1.1** through `base exec` |
| this repository's record, public channel 2026-09-14 | **1.2.1-R2847.1** |

⛔ **So no behaviour quoted from a reference is evidence for the build this
session would install.** The guide's every command is run on the installed build
before it is written, which this entry already required; this table is why.

### Still open, revised

1. The prove, and the guide, whose every command is run before it is written. Both
   need Muse signed in, so both wait for `wsl-toolkit-base` and the operator's
   `muse login`.
2. The interactive screen through herdr, now with a named first measurement:
   **does a Muse started through `base agent` or `muse.exe` keep its identity to
   herdr, and does `HERDR_AGENT` on the herdr side restore it when it does not?**
   Then `herdr agent start muse --kind muse` in a pane at the project's guest
   path, and attach from Windows with `WSL-76`'s closing drive.
3. ⚠ **A decision nobody has made:** whether `muse serve`'s stdio protocol gets a
   route through this tool, given that `base exec` closes stdin. It is named here
   so the later session rules on it rather than discovering it.
4. The closing, with the three reviews run again over what that drive adds.

## Amendment, 2026-09-15: the launcher on the operator's own base, before the sign-in

⚠ **Conditions:** `wsl-toolkit-base` as `WSL-76`'s amendment of the same date gives
them, the launcher written by `base ensure` into the account's own
`%USERPROFILE%\bin`, and a throwaway git project of three tracked files at
`.tmp\wsl78\proj`.

| measured | result |
| --- | --- |
| `base ensure` | wrote `%USERPROFILE%\bin\muse.exe`, 14,413,312 bytes; `Get-Command muse -All` names it alone |
| `muse --version` from this repository's root, which no grant covers | ⭐ exit 2, and the `base grant --source` line for that directory: the prove's second condition |
| `base grant --source . --mode rw` in the project | exit 0 in 0.33 s, `/workspaces/proj` mounted and written to the instance's own configuration |
| `muse exec --json --provider echo --approval-mode never "..."` in the project | exit 0 in 0.98 s, 29 JSON records on stdout, Muse's `workspace root: /workspaces/proj` on stderr. ⚠ The echo provider answers with the prompt, so the number is still owed |
| `base agent muse` with no argument, from an ungranted directory | exit 2 on the grant, before the screen rule |

⛔ **The example page had two commands that fail as written**, both found by running
them: `base grant . --mode rw`, exit 2 because `base grant` takes `--source` and no
positional path; and `base agent muse`, exit 2 whether or not the directory is
granted, while the page said it opens Muse's screen in a herdr pane. The page is
rewritten to commands that were run, and to the instance's own configuration: a
`--config` pointing at the example would let `base grant` edit the tracked file.

⚠ **Muse's version moved under the approved installer**: the same digest,
`5196d820...632a0ca`, now installs 1.3.0 where it installed 1.2.1 on 2026-09-14,
because the installer fetches a launcher that fetches the current Muse.

### Still open, revised

1. The prove's first condition, and the guide, after `muse login`.
2. The interactive screen through herdr: Muse started in a pane at the project's guest
   path, attached from Windows, with `WSL-76`'s closing drive. ⭐ Its identity question
   is answered in `WSL-76`: kept.
3. ⚠ **The decision nobody has made** still stands: whether `muse serve`'s stdio
   protocol gets a route through this tool.
4. The closing, with the three reviews.

---

## WSL-79. A BSD run pays two minutes before its first command, and a comment line ends its script early

**Source** [issue 33](https://github.com/Azathothas/ToolKit/issues/33), filed by
the operator on 2026-09-13, and the comment the operator had posted on it the same
day about the line join. By the operator's ruling the join belongs to that issue
and is not an entry of its own, so it is a task here.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

1. **Issue 33:** every `bsd run` boots the FreeBSD guest from power-on, waits
   about two minutes for a login prompt before its payload runs, and powers it off
   afterwards, so the next run pays again. The operator asks why and whether it is
   fixable. If it is not, the docs say so and tell people and agents to use the
   guest only when nothing else will do.
2. ⛔ **Its comment:** a payload's lines are joined with `; `, so a `#` comment
   line swallows every command after it and the run still exits 0. A blank line or
   a compound command split across lines is a syntax error.

## Premise

- ⭐ **Measured before this entry**, in `BSD-03` in [`bsd.md`](bsd.md): `login:`
  at 113.6 s, 117.4 s and 117.7 s over three boots, 108 s of it between the kernel
  banner and mounting root. `WSL-72`'s five sessions took 128.6 s to 287.2 s
  each.
- ⭐ **Read from the code:** each run starts `qemu-system-x86_64 -accel whpx -M
  q35 -cpu Icelake-Server-v7 -smp 2 -m 2048`, the image on `virtio-blk-pci`,
  `-serial stdio`, and no network device without `--network`, at
  `internal/toolkit/bsd.go:334`. It waits for `login:` at `bsd.go:484`, grows the
  root, runs the payload and powers off. Nothing survives between runs.
- ⚠ **Not measured: which probe holds the 108 s.** The console is mirrored to
  stderr without timestamps, `cmd_bsd.go:306`, so one timestamped boot answers it.
- ⭐ **The join is `bsd.go:590` and `bsd.go:591`**, and the comment measured it:
  `echo joined-a; # a comment; echo joined-b` printed only `joined-a` and exited
  0 in the FreeBSD 15.1 guest.

## Approach

1. **Time a boot**, with each console line timestamped on the host, and name the
   probe that stalls.
2. **Change one thing per boot, three boots each, and keep only what measures
   faster:** what the timeline implicates, among the loader's countdown, probes for
   devices this guest does not have, the devices QEMU presents, and the CPU and
   machine models.
3. **The payload reaches the guest as bytes, not as a typed line.** The candidate
   to measure first is a second raw disk holding a tar of the script, read in the
   guest with `tar -xf /dev/vtbd1`, which also removes the console's line-length
   limit. Typing it as bounded base64 lines is the fallback. ⛔ Never strip
   comments or parse shell on the host.
4. **The manual's BSD section** carries the boot measured after step 2, and says
   to use the guest only when a BSD kernel is required if a run still pays more
   than 30 seconds before its payload.
5. The comment's three shapes become regression cases, each asserting that the
   later command ran and that the exit code is the script's.

⛔ **The guest image is shared, and it is a pkgbase system whose kernel is a
package.** Take a package baseline first, remove only what was added, and never
remove a `FreeBSD-*` package.

## Decision

⭐ **Ruled by the operator on 2026-09-14, ahead of step 2 and conditional on it: as
recommended.** The measured cost is documented in every outcome, and a guest that
stays running between runs is built only if tuning leaves more than 30 seconds
before the payload.

Asked after step 2, not before: keep tuning, keep a guest running between runs, or
document the cost. Recommendation: document the measured cost in every outcome, and
build a guest that stays running only if tuning leaves more than 30 seconds before
the payload.

## Consumers

`wsl-toolkit bsd run` ships in the executable. A payload whose comment line
swallowed later commands now runs them, which is the fix of a silent failure, and
the changelog row says so. A faster boot changes nothing a payload sees.

## Prove

A script file holding, one per line: `echo a`, `# note`, an empty line, `if true;
then`, `echo b`, `fi`, `exit 3`.

```powershell
wsl-toolkit bsd run --script bsd-join.sh
```

Passing is:

- stdout `a` then `b`, and exit 3, read unpiped;
- `wsl-toolkit bsd run -c true` three times, with the `login at` figure recorded,
  and the manual carrying it;
- the regression cases green in the Go suites;
- the image left as found: the package list compared with its baseline, no
  `FreeBSD-*` package removed.

---

## Closing

**Closed 2026-09-14T04:11:39Z.** A run of `bsd run -c true` takes about 23 seconds
where it took about 128, and a script reaches the guest as a file on a disk of its
own, so the join is gone rather than worked around.

### ⛔ The premise called it device probing, and it was a driver waiting for a host

The premise carried `BSD-03`'s reading that 108 s of the boot is device probing, and
said which probe was not measured. Measured, with every console line stamped on
arrival: the console fell silent for 105.3 s after `usb_needs_explore_all: no
devclass` at 8.03 s, and the next line was the DVD drive QEMU adds by default, at
113.33 s. That line was a coincidence of order. With the default devices gone, the
silence was 105.6 s and ended at `Trying to mount root`.

⭐ **The wait is FreeBSD's Hyper-V VMBus driver.** WHPX shows the guest the host's own
signature, the console prints `Hypervisor: Origin = "Microsoft Hv"`, and `vmbus0:
<Hyper-V Vmbus> on pcib0` attaches at 4.75 s and holds root mount waiting for a VMBus
QEMU does not provide. Named by elimination on a copy of the image, so the shared one
was not touched: `hint.vmbus.0.disabled="1"` alone took the login to 7.5 s.

### The boots, three each, one change at a time

| series | what changed | login, s | session, s |
| --- | --- | --- | --- |
| as the image boots | nothing | 115.0, 114.8, 115.0 | 121.6, 121.0, 121.3 |
| 2 | `-nodefaults`, which removes the DVD drive and the VGA card | 113.3, 113.0, 113.3 | 119.5, 119.3, 119.5 |
| 3 | 2, and the hypervisor bit hidden | 9.3, 8.3, 9.5 | 15.5, 14.6, 15.8 |
| 4 | 3, and CLFLUSH and CLFLUSHOPT removed | 8.8, 8.5, 9.0 | 15.1, 14.8, 15.3 |
| on the copy | 2, and VMBus disabled with the signature still shown | 7.5, 7.8, 7.8 | 13.8, 14.0, 14.1 |
| the build that closes this | 4, and the payload disk | 9.5, 8.0, 8.0 | 17.4, 15.9, 15.9 |

⚠ **Series 4 is kept although the copy measured about a second faster.** Disabling
the driver means a line in the shared image's `/boot/loader.conf.local`, and a freshly
fetched image would pay the two minutes on its first run; hiding the bit is a QEMU
argument and costs nothing on any image. Series 3 printed `Unimplemented handler
(ffffffff8107e4a0) for FST - 150 (f ae)` 256 times a boot, QEMU's WHPX emulator
meeting a CLFLUSH the guest now believed it had; series 4 printed none.

⭐ **By the ruling, no guest is kept running.** The tuned run reaches its payload at
about 15 seconds, under the 30 that would have built one, and the manual carries the
cost: a login at 9.5 s, 8.0 s and 8.0 s, the command done at 17.4 s, 15.9 s and 15.9
s, and the process gone at 24.3 s, 22.8 s and 22.8 s.

### The acceptance command, on the build that closes this

The script file the Prove names, `echo a`, `# note`, an empty line, `if true; then`,
`echo b`, `fi`, `exit 3`, one per line:

```text
wsl-toolkit bsd run --script .tmp\wsl79\bsd-join.sh
exit=3, read unpiped
stdout: a
        b
  login at 9s, session 17s, disk 10.0 GiB, root filesystem 8.7 GiB, exit 3
```

All four passing conditions hold:

| condition | measured |
| --- | --- |
| stdout `a` then `b`, and exit 3 | above |
| three runs of `-c true` with the login recorded, and the manual carrying it | the last row of the table, and the manual's BSD section |
| the regression cases green in the Go suites | Windows with `TEMP` at the 8.3 path: 259 top-level cases, 255 passed, 4 skipped; `golang:1.25`: `check-go` ok, and the shell case, which skips on Windows, passed |
| the image left as found | 500 packages before and after, none added, none missing, all 499 `FreeBSD-*` present |

A script that exited 5 answered 5. Afterwards the guest's `/tmp` held one copy, and it
was the checking script's own `$0`; no payload disk was left beside the image.

### The reviews

⛔ **The door sweep found the guest keeping copies, and it is fixed.** Every line typed
at the console was listed: the login, the grow script, the extract, the run and the
poweroff. The first build removed the script's copy in `/tmp` only after a clean run,
so a script that failed or a run that timed out left it in the shared image, which
FreeBSD does not clear at boot. The run line now removes it whatever the exit and still
answers the script's code, and the extract sweeps copies by their exact name shape. A
payload disk the host cannot remove is swept by a later run once it is an hour old.
Any line with a newline in it is refused, so no later caller can reintroduce the join.

⭐ **The guard mutation proved 8 rows**: the refused newline, the archive padding, the
hidden hypervisor bit, `-nodefaults` and the payload disk's position on this host; and
in `golang:1.25`, where the shell case runs, all 8, the script run as a file, the copy
removed on a failed exit and the sweep among them.

⭐ **The claim audit** re-measured the manual's figures on the final build after the
run line changed, because the first figures came from a build with one more typed line;
removed "about two minutes" from `bsd run`'s progress line and from the sweep's reason
for not calling it; and corrected the premise above. What would have made it fire and
did not: a figure in the manual from a build other than the one that closes this.

---

## WSL-80. A throwaway command that has finished can leave `distro run` waiting forever

**Source** found on 2026-09-14 by the acceptance runner's baseline run, while
closing `WSL-74`.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

`wsl-toolkit distro run` relayed a command's last line and never returned. Its
heartbeat went on reporting the silence, and from 22 seconds reported the
distribution as stopped, for ten minutes, until the process was ended. A caller
with no `--timeout` waits forever over a command that exited at six seconds, and a
caller with one is answered 124 over a command that succeeded.

## Premise

⭐ **Measured once, on a build of `e73d7d5`**, in the acceptance case `an
unterminated line is shown early and a silence is reported with the distribution
state`: `distro run --log-profile ci --tick 1s --tick-escalate 3s` over `printf
'Password: '; sleep 6; echo; echo done`. The heartbeat's own lines: the output
resumed at 6.095 s, `distro running` until 21.1 s, `distro stopped` from 22.4 s,
and ticks until 10m31s. The process held no `wsl.exe` for the command, and it kept
starting the heartbeat's own `wsl.exe` children.

⚠ **Read, not measured.** `Wsl.Exec` reaches `runCommand` in
`internal/toolkit/process.go`, which calls `exec.Cmd.Run` with writers that are not
files, so `Wait` waits for the goroutines copying the child's pipes. `newCommand`
sets `WaitDelay` to one second, which closes those pipes once the child has exited.
⚠ On Windows a synchronous read of an anonymous pipe is not released by closing
it, so a write end held open past the child's exit would keep `Wait` from ever
returning. What held it here was not identified.

⚠ **Not measured: how often.** The same case passed in the 90-case run that
closed `WSL-73`.

## Approach

1. **Reproduce before changing anything:** the case's command in a loop on a
   throwaway distribution, each run bounded from outside, counting the runs that
   do not return. Then a diagnostic build kept outside the tree that writes every
   goroutine's stack after a bound, to name the blocked call.
2. **Fix it at the seam every command shares**, `runCommand`: once the child has
   exited, its output copies get a bounded time to drain, and then the child's own
   exit code is the answer, with the undrained stream named rather than waited on.
3. **A regression case that needs no WSL:** a child that exits while a grandchild
   still holds its stdout, asserting `runCommand` returns within the bound with the
   child's code.

⛔ **No deadline on the caller's command hides it.** The defect is waiting after
the command has ended, and a timeout would turn it into a wrong 124.

## Consumers

None by [`../docs/consumers.md`](../docs/consumers.md)'s definition: a run that hung
now returns its command's exit code.

## Prove

The case's command, run 30 times on a throwaway distribution, each bounded at 60
seconds from outside the tool:

```powershell
wsl-toolkit distro run --name NAME --log-profile ci --tick 1s --tick-escalate 3s --command-base64 cHJpbnRmICdQYXNzd29yZDogJwpzbGVlcCA2CmVjaG8KZWNobyBkb25lCg==
```

Passing is:

- 30 of 30 runs exit 0, each inside the bound, read unpiped;
- the regression case green in the Go suites on Windows and on Linux, and a
  mutation row that removes the bound goes red;
- the acceptance case above green in a full run.

---

## Closing

**Closed 2026-09-14T04:11:39Z.** The heartbeat's loop takes its stop and done
channels as arguments, `Finish` takes both under the lock, and a case holds the
ordering that hung. All three passing conditions hold:

| condition | measured |
| --- | --- |
| 30 of 30 runs exit 0 inside the bound | the tree's build, the case's command on a throwaway Alpine distribution: 30 of 30 exit 0 with `done` on stdout, in 6.2 s to 8.9 s, no hang |
| the regression case green in the Go suites on Windows and Linux, and a mutation row red | Windows with `TEMP` at the 8.3 path: 259 top-level cases, 255 passed, 4 skipped; `golang:1.25`: `check-go` ok and the case passed; two rows red on both hosts |
| the acceptance case green in a full run | the tree's build: `acceptance: 91 case(s) passed against a real machine.` |

⚠ **The first full run after the fix failed that case with `False`, and it was the
case.** Run as written ten times on a quiet host, `after 3s of silence` was missing
in 4: a tick lands a 250 ms poll plus WSL's answer after the last, and in six seconds
of silence one run's ticks fell at 3.615 s and 4.863 s, with output resuming at
6.099 s before a third. The case now sleeps ten seconds, which reached the
threshold in 10 of 10 runs, and the full run above is on that case.

### ⛔ The premise blamed the wrong thing, and the approach named the wrong seam

The premise said a pipe held open past `wsl.exe`'s exit kept `Wait` from returning,
and read that from `os/exec`. ⛔ **Measured, it was not a pipe.** Go 1.27 cancels a
pending pipe read with `CancelIoEx` when it closes the pipe, and the hung process was
not in `Exec` at all. A diagnostic build kept outside the tree wrote every
goroutine's stack 40 seconds into a run, and each of six hangs showed the same two:

```text
goroutine 1 [chan receive]:
toolkit.(*RunLog).Finish(...)        internal/toolkit/runlog.go:688
toolkit.(*Throwaways).runIn(...)     internal/toolkit/throwaway.go:1177
goroutine 18 [select]:
toolkit.(*RunLog).poll(...)          internal/toolkit/runlog.go:523
```

⭐ **The command had ended and `Exec` had returned.** `Finish` was waiting for the
heartbeat's loop to stop, and the loop never would: it selected on `r.stop` read from
the struct on every pass, while `Finish` set that field to nil under the lock and then
closed the channel it had taken. A pass that was inside `check`, asking `wsl.exe`
about the distribution when `Finish` ran, came back to a select on a nil channel and
never saw the close. The ticks that went on for ten minutes were that loop.

So the seam is the relay, not `runCommand`: `poll` takes its stop and done channels as
arguments, and `Finish` takes both under the lock. The regression case needs no WSL
and no grandchild process either. It holds the heartbeat inside its question about
the distribution, lets `Finish` take the channels, then answers.

### What was measured

On this host, a throwaway Alpine distribution under an isolated state directory, the
case's command, each run bounded at 60 seconds from outside the tool:

| build | runs | hung | each run that returned |
| --- | --- | --- | --- |
| the tree before the fix, with the stack hook | 17, stopped there | 6: runs 8, 11, 14, 15, 16 and 17 | exit 0 in 6.2 s to 6.6 s |
| the fix, with the stack hook | 30 | 0 | exit 0 in 6.2 s to 6.5 s |

⭐ **The regression case went red first.** Against the unfixed relay it failed after
10.25 s with `Finish did not return after the heartbeat's question was answered`; with
the fix it passed five times in five at 0.25 s each, and under `go test -race` three
times in three with the relay's closed-stream case beside it.

### The reviews

⭐ **The door sweep** looked for every loop that selects on a channel read from a
struct: the relay's `poll`, the job ticker in `tick.go` and the helper's shutdown in
`helper.go`. The other two never clear the field they select on, and each closes its
channel once through `sync.Once`, so the relay was the only one. What would have made
it fire: a second loop re-reading a field some other path reassigns.

⭐ **The guard mutation** proved two rows: the loop selecting on `r.stop` again, and
`Finish` waiting on `r.done` after clearing it. The second first failed to compile,
because the local it replaced became unused, and was rewritten to keep it; it then went
red. The acceptance case that found the hang is the third check, in the full run below.

⭐ **The claim audit** is the correction above: the premise's mechanism was read from
the standard library and a stack showed it false, and the approach's seam and
regression shape moved with it.

---

## WSL-81. A FreeBSD guest that panics mid-run leaves `bsd run` waiting out its budget

**Source** found on 2026-09-14 by `WSL-72`'s prove, run against `main` at `469e52a`.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

`wsl-toolkit bsd run --network --timeout 25m -c '...'` exited 2 after 1500.2 s with
`the guest did not finish the command within the budget`. The guest had panicked 65
seconds after boot, and QEMU had exited soon after. The run spent the rest of its 25
minutes waiting for console text from a process that no longer existed, and its
message named a timeout, not the panic.

The panic came in the middle of `pkg install`. The next boot stopped at `UNEXPECTED
SOFT UPDATE INCONSISTENCY; RUN fsck MANUALLY` and asked for a single-user shell, so
the shared image could not boot until `bsd fetch --force` restored it.

## Premise

⭐ **Measured on 2026-09-14**, the prove's console:

```text
bootstrap:   installing 6
Fatal trap 12: page fault while in kernel mode
current process		= 7 (dom0)
panic: page fault
cpuid = 1
#5 0xffffffff81088f06 at pmap_ts_referenced+0x5a6
#6 0xffffffff80f46778 at vm_pageout_worker+0xb18
Uptime: 1m5s
Dumping 277 out of 2009 MB:..6%..12%..24%..35%..41%..52%..64%..75%..81%..93%
Automatic reboot in 15 seconds - press a key on the console to abort
```

- ⭐ **Read in `bsd.go`:** the goroutine `startGuest` starts to read the console
  returns when QEMU closes it, and tells nobody. `waitFrom` and `waitBoot` poll the
  text until a pattern matches or the context ends, so a guest that is gone costs
  whatever is left of `--timeout`.
- ⭐ **Read in `bsd.go`:** `bsdBootFailure` recognises a panic only while a run
  waits for `login:`. After the login, nothing looks for one.
- ⚠ **The panic is in the page daemon**, while `pkg` installed six packages into a
  2048 MiB guest. The same payload exited 0 twice before it: the tree's
  `bootstrap.sh` by `--script` on 2026-09-14 in 3m36s, and run B of `WSL-72`'s
  amendment on 2026-09-13, before `WSL-79` changed the CPU model.
- ⚠ **Not measured: whether `WSL-79`'s CPU model makes a panic likelier.** It hides
  the hypervisor bit, CLFLUSH and CLFLUSHOPT from the guest. `WSL-72`'s amendment
  records a kernel page fault at poweroff under the model before those flags, so
  this image has panicked under WHPX without them.

## Approach

1. **A guest that has gone ends every wait at once.** The console reader closes a
   channel when QEMU closes its console, and every wait selects on it.
2. **A kernel panic ends a run's wait at once**, recognised by FreeBSD's own shape:
   a line starting `panic: ` and, on the next line, `cpuid = `. ⛔ Not `panic: `
   alone. A program's output can start a line that way, and Go's runtime does.
3. **The error names what happened:** the panic line and the `Fatal trap` line
   before it, or the console's last line when it closed. It also says that a panic
   during writes can leave the image needing `bsd fetch --force`.
4. **The panic rate is measured**, with the payload that panicked, on throwaway
   copies of the published image: the CPU model `WSL-79` chose against the same
   model with the hypervisor bit shown. If hiding the bit measures likelier to
   panic, `WSL-79`'s choice is reopened here.

## Consumers

None by [`../docs/consumers.md`](../docs/consumers.md)'s definition. A run that
waited out its budget and exited 2 naming a timeout exits 2 at once, naming the
panic.

## Prove

Cases with no guest, through `startGuest`: a child that exits, and a child that
prints the panic above and keeps running. Each must answer within seconds under a
budget of minutes. Then the runs from step 4.

Passing is:

- both cases green on Windows and Linux, with a mutation row per rule red;
- the runs recorded, each with its exit, wall time and panic, and a decision about
  the CPU model written from them;
- the shared image boots, and reads back its published packages.

## Amendment, 2026-09-14: the waits end when the guest does

⭐ **Built.** The console reader closes a channel when QEMU closes the console, and
every wait selects on it. A command's wait also ends on a kernel panic, told by
the `cpuid = ` line after the panic line, and the error names the panic, the trap
before it and `bsd fetch --force`. A boot whose console closes answers with the last
line it printed. A step that could not finish, such as the grow or the extract,
names why instead of its first line of output.

| case | Windows, `TEMP` at the 8.3 path | `golang:1.25` |
| --- | --- | --- |
| `TestAKernelPanicIsToldFromAProgramThatSaysPanic` | pass | pass |
| `TestAGuestWhoseKernelPanicsEndsTheCommandAtOnce` | pass, 0.66 s to 0.97 s in three runs | pass, 0.56 s |
| `TestAGuestWhoseConsoleClosesIsNotWaitedOn` | pass, 0.03 s to 0.05 s in three runs | pass |

⚠ **The panic case first held by timing.** Its child printed the panic as it
started, which could land before the position the command's wait reads from. The
child now prints it after the typed line arrives, as a panic during a command does.

⭐ **Five mutation rows went red on Windows**: the reader closing the channel, a
command's wait and the boot's wait selecting on it, the panic check in a command's
wait, and the `cpuid = ` line in the panic shape.

⭐ **The shared image is restored.** `bsd fetch --force` found its archive whole,
checked the pinned digest and expanded it again in 84 s: the published 6,476,638,208
bytes. `WSL-72`'s read-back eleven minutes before the panic found only the published
packages and files on it, with its own additions removed, so the restore lost
nothing another session had put there.

⭐ **A second shape, found by the panic-rate runs:** a run whose payload exited 0
and whose kernel then panicked while powering off, in `VOP_RECLAIM_APV` after `All
buffers synced`. Its exit stands; the result now carries the panic as
`shutdown_panic` and the run warns that the next boot saves a core dump into the
shared image. A case holds it with a child that panics on `poweroff`, and its row
went red.

### The panic-rate runs so far, stopped at the session's checkpoint

The prove's payload on fresh copies of the published image, each run from its own
cache directory under `.tmp`. ⚠ **The host was not quiet:** Go builds, mutation runs
and container jobs ran beside most of them, which is a condition of these numbers
and may be a cause.

| model | runs | panic mid-run | panic at poweroff | clean |
| --- | --- | --- | --- | --- |
| `WSL-79`'s, the hypervisor bit, CLFLUSH and CLFLUSHOPT hidden, 2048 MiB | 6, and the prove | 1, and the prove | 1 | 4 |
| the same with 4096 MiB | 2 | 0 | 1 | 1 |
| the hypervisor bit hidden, CLFLUSH shown | 2 | 1 | 1 | 0 |
| the hypervisor bit shown, which waits 105 s for VMBus | 4 | 1 | 1 | 2 |

Eight panics in 14 runs and the prove, in eight different kernel functions:
`pmap_ts_referenced`, `vmspace_exit`, `ufs_direnter`, `VOP_RECLAIM_APV`,
`cache_purge_impl`, `uipc_close`, `sorele_locked`, and one whose backtrace the run
cut short. ⚠ The four panics counted at poweroff on builds from before `shutdown_panic`
are inferred: those builds end a run on a panic, so a run that exited 0 with a panic
on its console panicked after the payload answered.

⭐ **The rule the fix added worked on a real guest:** the second CLFLUSH-shown run
ended `the guest's kernel panicked: panic: page fault, after Fatal trap 12` at 250.8
s, where the first matrix's mid-run panics waited 540 s and 660 s for their budgets.

⭐ **Decision about the CPU model, from these runs: it stays.** Every model tried
panicked, including the one that shows the hypervisor, so hiding the bit is not the
cause, and memory is not either.

### One-vCPU proof and checkpoint, 2026-09-14

⭐ **The hypothesis held for five fresh images.** Each run copied and verified the
published archive into its own cache under `.tmp`, expanded it, and ran the exact
`WSL-72` language-toolset payload with `--cpus 1`, 2048 MiB and a 25-minute budget.
The host ran no other toolkit guest or mutation job beside them.

| run | wall | guest | boot | exit | panic |
| --- | ---: | ---: | ---: | ---: | --- |
| 1 | 320.3 s | 312.4 s | 11.8 s | 0 | none |
| 2 | 212.0 s | 204.2 s | 13.8 s | 0 | none |
| 3 | 234.1 s | 225.3 s | 11.8 s | 0 | none |
| 4 | 316.3 s | 307.8 s | 15.0 s | 0 | none |
| 5 | 264.3 s | 255.6 s | 12.5 s | 0 | none |

All five reported the six expected tools present, `nim 2.2.10`, `rustc 1.96.1`,
no failed package, no mid-run panic and no `shutdown_panic`: **5 clean of 5**.
Each expanded copy was 12,884,901,888 bytes and was deleted immediately after its
result was recorded; no image copy, QEMU process or toolkit process remained.

⭐ **The shared image also passed the non-mutating part of the prove.** With one
vCPU it booted, grew from the published 6,476,638,208 bytes to 12,884,901,888
bytes, and read back 500 packages, 499 named `FreeBSD-*`. The sorted package
baseline's SHA-256 was
`f447f1da…0d9aa2`.
`df -k /` read 11,138,540 1024-blocks, 2,608,628 used and 7,638,832 available.
The run exited 0 in 35.7 s with no panic.

⭐ **Built from that result:** `bsd run` now defaults to one processor in both the
command and the library fallback; `--cpus` remains an override. The manual names
the default and the five-run basis. The unmutated command-line case passed, and a
mutation that restores the old two-processor default went red.

⭐ **Door sweep complete.** The public `bsd run` path has one library entry point.
Command waits converge on `waitFrom`, boot has its own closed-console branch, and
poweroff converges on `stopAndReadPanic`; no sibling wait bypasses the panic or
closed-console guards.

---

## Closing

**Closed 2026-09-14T09:36:07Z.** A guest that has gone ends every wait at once, a
kernel panic ends a command's wait when it prints, a panic at poweroff is carried on
the result, and a run defaults to the one processor that ran the payload five times
without a panic. All three passing conditions hold on the tree's build:

| condition | measured |
| --- | --- |
| both cases green on Windows and Linux, a mutation row per rule red | Windows with `TEMP` at the 8.3 path: 280 top-level `wsl-toolkit` cases, 276 passed, 4 skipped. `golang:1.25`: 279, 278 passed, 1 skipped, every `WSL-81` case among the passes. 8 rows red on Windows, below |
| the runs recorded, and a decision about the CPU model | the two tables above: the model stays, and the processor count is one |
| the shared image boots, and reads back its published packages | the one-processor run on the shared image above: 500 packages, 499 `FreeBSD-*`, sorted digest `f447f1da…0d9aa2`. Three runs of `-c true` below booted it again, with no panic |

### The per-run cost, measured again on this build

The manual's figures came from the two-processor build. Three runs of `bsd run -c
true --json` on the shared image with the one-processor default:

| run | login, s | command done, s | process gone, s | panic |
| --- | ---: | ---: | ---: | --- |
| 1 | 8.5 | 16.4 | 23.3 | none |
| 2 | 8.3 | 16.2 | 23.0 | none |
| 3 | 8.5 | 16.4 | 23.3 | none |

One processor costs the boot nothing this series can see: the two-processor build read
9.5, 8.0 and 8.0 s to a login, and 24.3, 22.8 and 22.8 s to the process gone. Each
run read a 12,884,901,888-byte disk and an 11,405,864,960-byte root.

### The reviews

⭐ **The door sweep** listed every wait on the console: the boot's `waitBoot`, the root
shell's `wait`, each command's `waitFrom` in `run`, and the poweroff's `waitFrom` in
`stop`, which `stopAndReadPanic` reaches. `startGuest` has one caller outside the
tests, `BsdRun`, and `BsdRun` has one, `cmdBsdRun`. It found no wait that bypasses
the closed-console or panic checks. What would have made it fire: a console read
outside `waitFrom` and `waitBoot`, or a second path that boots a guest.

⭐ **The guard mutation proved 8 rows on Windows**, each run alone with `repo mutate
--only` after its cases passed unmutated: the one-processor default, the reader
closing the channel, the command's and the boot's wait selecting on it, the panic
check in a command's wait, the `cpuid = ` line in the panic shape, the panic at
poweroff carried on the result, and the step error below. The first seven took 7.2 s
to 40.5 s each and 2m56s together. The checkpoint's broad `bsd:` run, which left its
runner idle, did not recur in these narrow runs.

⛔ **The claim audit found three things:**

| found | what was done |
| --- | --- |
| a grow or an extract that a panic ended printed `(exit 0)` beside the panic, a code the guest never sent | `stepError` gives a step that never finished no exit code. `TestAStepTheGuestNeverFinishedCarriesNoExitCode` holds it through the fake guest, and its row went red |
| the manual's per-run cost, about 25 seconds, came from the two-processor build | measured again above, and the manual and its known-limits row say about 23 seconds |
| a payload that prints FreeBSD's two panic lines has its run ended as a panic, because its output shares the console, and nothing said so | measured through the fake guest: the two lines, then the command's closing marker a second later, ended the command at 641 ms as `the guest's kernel panicked`. With no gap, it answered exit 0. The manual says so, and `WSL-82` carries it, because telling the two apart is a decision |

⚠ The Prove asked for an answer "within seconds under a budget of minutes". The cases
hold a 30-second budget with a 15-second ceiling, and answer in under a second. A
mutated case fails at the 30-second budget, which is why those rows took about 40 s.

### ⛔ Corrected the same day: one processor panicked at poweroff, before the buffers synced

Six minutes after this closing, two read-only runs in a row on the shared image
panicked while powering off, with one processor, and the first came before `Syncing
disks`. The amendment's panic at poweroff came after `All buffers synced`, and the
manual and two comments in `bsd.go` had made that true of every such panic; all three
are rewritten. `shutdown_panic` caught both panics on a real guest. `WSL-83` carries
what they did to the image.

---

## WSL-82. A payload that prints a FreeBSD panic's two lines has its `bsd run` ended as a kernel panic

**Source** found on 2026-09-14 by `WSL-81`'s claim audit.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** S, **Status** done

---

## Problem

`bsd run` ends a command's wait as a kernel panic when the console shows a line
starting `panic: ` with `cpuid = ` on the next line, and the payload's own output is
on that console. A payload that prints those two lines, as a copy of an earlier panic
would, has its run ended with exit 2 and `the guest's kernel panicked` while the guest
is still running. The guest is then killed rather than powered off.

## Premise

- ⭐ **Measured on 2026-09-14 through the suite's fake guest**, a child that answers
  the typed line: the two lines, then the command's closing marker one second later,
  ended the command at 641 ms with `the guest's kernel panicked: panic: page fault`.
  With no gap between them, the command answered exit 0, because `waitFrom` in
  `bsd.go` looks for the marker before it looks for a panic.
- ⭐ **Read in `bsd.go`:** `waitFrom` checks the console after the typed line, the
  payload's output included, and a `*guestGoneError` leaves `graceful` false, so `stop`
  kills QEMU rather than typing `poweroff`.
- ⚠ **Not measured on a real guest.** Which payloads print both lines at the start of
  a line was not surveyed.
- ⚠ **Read from the console of `WSL-72`'s prove, not measured:** after a panic,
  FreeBSD dumps its memory and prints `Automatic reboot in 15 seconds`, and QEMU's
  `-no-reboot` turns that reboot into an exit.

## Approach

1. ⛔ **A real panic is still named with its line, and a guest that hangs after one
   still ends before its budget.** `WSL-81`'s cases hold both.
2. The decision below sets what ends the wait once the two lines show.
3. A case through the fake guest for the payload's copy, in the shape the premise
   measured, and a mutation row per rule.

## Decision

⭐ **Ruled by the operator on 2026-09-14: B.** After the console shows the two
lines, QEMU's exit, the command's closing marker or 60 seconds ends the wait,
whichever comes first. A real panic is named after its dump and reboot. A guest that
hangs after a panic ends at 60 seconds. A payload's copy that finishes inside the
bound answers normally. Waiting at once was rejected because it kills a running
guest over its payload. Waiting for QEMU alone was rejected because a hung guest
would wait for the full run budget.

## Consumers

None by [`../docs/consumers.md`](../docs/consumers.md)'s definition. Under B, a run
that exited 2 over a payload's copy exits with the payload's own code, and a real
panic keeps exit 2.

## Prove

```powershell
go test ./internal/toolkit -run 'TestAGuestWhoseKernelPanics|TestAPayloadsCopyOfAPanic' -count=1 -v
```

Passing is, under B, green on Windows and in `golang:1.25`:

- a fake guest that prints the two lines and its closing marker a second later
  answers exit 0;
- one that prints them and exits, as a rebooting guest does, answers `the guest's
  kernel panicked` within seconds;
- one that prints them and hangs answers the panic at the bound, not at the budget;
- a mutation row per rule red.

---

## Closing

**Closed 2026-09-14T15:45:01Z.** After the console shows a panic's two lines, a
command's wait ends at QEMU's exit, at the command's closing marker, or 60 seconds
later, whichever comes first, and a budget that ends inside those 60 seconds names
the panic. All four passing conditions hold on the tree's build:

| condition | Windows, `TEMP` at the 8.3 path | `golang:1.25` |
| --- | --- | --- |
| the two lines, then the closing marker a second later, answer exit 0 | `TestAPayloadsCopyOfAPanicThatFinishesAnswersNormally` pass, 1.66 s, the lines in the output | pass, 1.55 s |
| the two lines, then an exit, answer `the guest's kernel panicked` within seconds | `TestAGuestWhoseKernelPanicsAndExitsEndsWhenQEMUDoes` pass, 0.56 s | pass, 0.55 s |
| the two lines, then a hang, answer the panic at the bound and not the budget | `TestAGuestWhoseKernelPanicsAndHangsEndsAtTheBound` pass, 2.98 s with the bound at 2 s and a 30 s budget | pass, 2.65 s |
| a mutation row per rule red | 4 new rows and 2 changed, below | the same 6 red |

The prove command exit 0 on both hosts. The whole `wsl-toolkit` suite: 303 top-level
cases on Windows, 296 passed and 7 skipped; 302 in `golang:1.25`, 301 passed and 1
skipped; `check-go` exit 0 on both; ShellCheck 0.9.0 clean over 34 tracked scripts.

⭐ **Driven on a real guest, which the premise had not done.** On the shared image, a
script printing `panic: page fault` and `cpuid = 0`, sleeping 2 s, printing a line
and exiting 7: the build before this entry answered exit 2 in 20.2 s, `the guest's
kernel panicked: panic: page fault, before the command finished`, with the output cut
after the two lines and the guest killed. This build answered exit 7 in 30.2 s, the
session 22.9 s, with all three lines. The image's SHA-256 was the same after both
runs, and no QEMU process was left.

### The reviews

⭐ **The door sweep** listed every caller of `waitFrom`: `wait`, which `BsdRun` uses for
the root prompt after the login; `run`, for the grow, the extract and the payload;
and `stop`, for the poweroff lines. All three now take the bound. `waitBoot` reads the
console before any payload, and still ends a boot at a panic's first line, which no
payload can print. `stopAndReadPanic` reads only what the console printed after the
poweroff was typed, so a payload's copy is not read as a panic at poweroff. What
would have made it fire: a fourth wait over the payload's output that does its own
panic check, and a grep for `bsdKernelPanic(` finds `waitFrom` and `stopAndReadPanic`
only.

⭐ **The guard mutation proved 6 rows on Windows**, each seen green unmutated first:
a copy that finishes answers normally, a hang ends at the bound, a budget names the
panic, a real panic ends when QEMU exits, and the changed rows for the panic check
and for a step a panic ended. The same 6 went red in `golang:1.25`, in 2m7s with the
suite.

⛔ **The claim audit corrected the manual**, which said a run ends "as soon as" the
console shows a panic, and that a payload printing the two lines "can end its run the
same way". It now gives the three ways out and the 60 seconds, with the drive above.

---

## WSL-83. A FreeBSD kernel panic at poweroff leaves the shared guest image unchecked, and the next run panics on it

**Source** found on 2026-09-14 while reading the shared image's package baseline for
`WSL-72`'s prove.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

Two `bsd run` calls in a row, each with a read-only payload that exited 0, panicked
the guest's kernel while it powered off. The first panic came before the kernel
synced its buffers. The second run booted on a root filesystem that was not properly
dismounted and had not been checked, saved a core dump into the shared image, and
panicked inside the filesystem as it powered off. Each run exited 0 with a
`shutdown_panic` warning, and nothing told the second run's caller, before its payload
ran, that the guest's filesystem was unchecked.

## Premise

⭐ **Measured on 2026-09-14** on the shared image, grown to 12 GiB, with one processor
and 2048 MiB, on the tree's build at `e32791a`:

| payload ran | payload | exit | panic while powering off |
| --- | --- | --- | --- |
| 09:41:56Z | `pkg query`, `df`, `ls`, `sha256` | 0 | `bad pte va 389278400000 pte 0`, in `pmap_remove_pages` from `exit1`, while `rc.shutdown` stopped processes and before `Syncing disks`; a 165 MB dump |
| 09:44:52Z | `mount -p`, `tunefs -p /`, `dumpfs`, `ls /var/crash` | 0 | `initiate_write_filepage: dir inum 0 != new 160513` |

- ⭐ The second boot printed `WARNING: / was not properly dismounted`; then `savecore`
  wrote `/var/crash/vmcore.0`, 173,228,032 bytes, and the boot printed `Starting
  background file system checks in 60 seconds.` Its payload ran 11 s after the
  kernel's boot time and the poweroff followed, so that check never started.
- ⭐ `tunefs -p /` reads soft updates enabled and soft update journaling disabled.
- ⭐ The second run still read 500 packages, with the sorted digest `f447f1da…0d9aa2`
  the first read.
- ⭐ Three `-c true` runs on the same image with one processor, from 09:28Z to 09:29Z,
  powered off with no panic.
- ⭐ `bsd fetch --force` restored the published image in 13.1 s.
- ⚠ **Not measured: what makes the kernel panic.** Every function named in `WSL-81`'s
  panics and in these two is in memory management or the filesystem, under WHPX.

## Approach

1. The decision below first.
2. Whatever is chosen, a boot whose console shows `was not properly dismounted` is
   named on the run's result, because nothing reads that line today.
3. The manual's BSD section keeps what a panic at poweroff leaves, with the
   measurement.

## Decision

⭐ **Ruled by the operator on 2026-09-14: A.** QEMU opens a throwaway overlay for
each run. A run's writes land in that overlay, and the overlay is discarded after
QEMU exits. A panic cannot change the shared image. Nothing a payload installs
survives its run. The accepted cost is that grow and first-boot work can repeat for
each run. Measure that cost before the implementation is accepted. A writable image
with a guard was rejected because a panic can still damage it. A warning alone was
rejected because it runs the next payload on an unchecked filesystem.

## Consumers

⚠ By [`../docs/consumers.md`](../docs/consumers.md)'s definition, the overlay breaks
a caller that installs something in one run and uses it in the next. The changelog
says so when this entry is implemented.

## Prove

```powershell
wsl-toolkit bsd run -c 'touch /root/tk-overlay-probe'
wsl-toolkit bsd run -c 'test ! -e /root/tk-overlay-probe'
```

Passing is:

- both exit 0, and the image file's SHA-256 is the same before the first and after the
  second;
- three runs of `bsd run -c true` with the login and the session recorded, and the
  manual carrying them;
- a mutation row per rule red.

---

## Closing

**Closed 2026-09-14T15:31:34Z.** A run boots the guest from a qcow2 overlay that
`qemu-img` makes beside the image, with the image as its read-only backing file and
the guest disk's size, and the run removes the overlay after QEMU exits. The image is
never grown or written. A boot that finds the image's root not properly dismounted is
named on the result as `root_not_dismounted`, with a warning. All three passing
conditions hold on the tree's build, on the shared image:

| condition | measured |
| --- | --- |
| both exit 0, and the image's SHA-256 the same before the first and after the second | `touch /root/tk-overlay-probe` exit 0 in 23.4 s; `test ! -e /root/tk-overlay-probe` exit 0 in 23.6 s; SHA-256 `12807CE7…921663BF` and 6,476,638,208 bytes before, after both, after three more runs, and after a last run of the final build; no `tk-overlay-` or `tk-payload-` file left |
| three runs of `bsd run -c true`, the login and the session recorded, and the manual carrying them | below, and in the manual's BSD section |
| a mutation row per rule red | 6 new rows and 4 changed, below |

| run | login, s | command done, s | process gone, s | first-boot work on the console |
| --- | ---: | ---: | ---: | --- |
| 1 | 8.6 | 16.5 | 23.4 | `Growing root partition`, host keys generated |
| 2 | 8.6 | 16.5 | 23.4 | the same |
| 3 | 8.8 | 16.8 | 23.6 | the same |

⭐ **The cost the ruling accepted measured at nothing this series can see.** Every run
is the image's first boot and does its first-boot work, and the three runs read
within 0.6 s of the one-processor series `WSL-81` measured on a grown image, 23.0 s to
23.3 s. The guest disk read 12,884,901,888 bytes and the root 11,405,864,960, the
11,138,540 KiB the manual gives for 12 GiB. ⚠ The 29 s first boot the manual gave
after a fetch was not seen again; its cause was not read.

⭐ **The new result field was driven on a throwaway copy of the image.** The build
before this change ran `sync; sleep 600` with `--timeout 45s`, exit 2 at 45.2 s, which
killed its guest and left the copy's root not properly dismounted. A run of this build
then booted the copy: exit 0 in 23.6 s, `root_not_dismounted` `WARNING: / was not
properly dismounted`, the warning printed, and the copy's SHA-256 the same after the
run. The copy was deleted, and no QEMU process was left.

| suite | Windows, `TEMP` at the 8.3 path | `golang:1.25` |
| --- | --- | --- |
| `check-go` | exit 0 | exit 0 |
| `wsl-toolkit` top-level cases | 300: 293 passed, 7 skipped, 0 failed | 299: 298 passed, 1 skipped, 0 failed |
| ShellCheck 0.9.0 in `ubuntu:24.04` | - | exit 0 over 34 tracked scripts |

`Found, and not filed` item 9 in the record, the guest's shell history growing in
the shared image with every run, ends with this entry: the digest above held across
six runs.

### The reviews

⭐ **The door sweep** listed every path that reaches the image file. `bsd fetch`
writes it, and is the restore path. `BsdProbe` reads its size, and `BsdRun` reads its
size and hands its name to `qemu-img` as a backing file; `startGuest` has one caller
outside the tests, `BsdRun`, and `BsdRun` has one, `cmdBsdRun`. `growBsdImage`, the one
writer a run had, is gone, and a grep for `os.Truncate`, `OpenFile` and `WriteFile`
in `bsd.go` finds only the payload disk. The acceptance runner does not boot a guest,
by its own sweep list. What would have made it fire: a second path that boots a
guest, or a QEMU drive naming the image itself, which
`TestARunWritesToAnOverlayAndNeverTheImage` refuses.

⭐ **The guard mutation proved 10 rows on Windows**, each seen green unmutated
first: the root disk is the overlay, the image is its backing file, a left overlay is
swept, a younger one is kept, a root not properly dismounted is named, and `qemu-img`
beside the emulator; and the changed rows for the disk refusal, the payload disk's
order, a panic ending a command's wait, and a step a panic ended. In `golang:1.25`
every BSD row went red, 23 of 23, in 3m2s: the 6 new, the disk refusal, and the 16
`bsd:` rows, which read the changed code and messages.

⛔ **The claim audit corrected three sentences.** The panic error told a caller that
a panic "can leave its filesystem needing a check" and to run `bsd fetch --force` if
the next run stopped, which the overlay makes false; it now says the guest's writes
go with the overlay. The warning for a root not properly dismounted first said "no
run checks it", and a run longer than 60 s does run the background check, in an
overlay it then discards; it now says so. And `bsd status` said "the next run grows
it to 12.0 GiB", which no run does now.

---

## WSL-84. A base reconfigured to read-only drives keeps writable ones, and reports read-only

**Source** found on 2026-09-14 while measuring `WSL-71`'s premise.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

A changed `base.automount` is not applied by `base ensure`, and both `base ensure`
and `base status` report the new value over a guest that still has the old one.
Changed from `rw` to `ro`, the base kept `/mnt/c` writable while it reported
`automount ro` and `usable true`, so a job in it can still write the Windows checkout
that `WSL-63`'s read-only drives exist to protect.

## Premise

⭐ **Measured on 2026-09-14 on the throwaway `wsl-toolkit-m71`**, an arch base with
interop off:

| the configuration went | `base ensure` answered | what the guest had |
| --- | --- | --- |
| from `off` to `ro` | exit 0 in 2.3 s, `automount ro`, `usable true`, nothing provisioned | `/etc/wsl.conf` still `[automount] enabled=false`, and no `/mnt/c` |
| from `rw`, after `base recreate`, to `ro` | exit 0 in 2.5 s, `automount ro`, `usable true` | `/mnt/c` mounted `9p rw`, and `test -w` on a directory in this repository's checkout exit 0 as the account |

- ⭐ **Read in `internal/toolkit/verify.sh`:** `off` is checked, refusing a mounted
  drive and an empty mount point alike; `ro|rw) ;;` checks nothing, and the script
  then prints `automount` with the value it was handed, which is what the status
  shows.
- ⭐ **Read in `base.go`:** `EnsureWith` on a registered base whose verification
  passes goes straight to the adapters, so a change verification does not see is
  never provisioned.
- ⭐ **Measured the same day, read-only:** the default `wsl-toolkit` base, built `ro`,
  has `/mnt/c` mounted `9p ro`. The defect is a change after the build, not the build.
- ⚠ Not measured: `ro` to `rw`, and `ro` or `rw` to `off`.

## Approach

1. **Verification reads the drives it promises**: with `ro`, every `/mnt/<drive>` in
   `/proc/mounts` carries `ro`; with `rw`, the drives are mounted; and the printed
   `automount` line is what `/proc/mounts` shows, not what was asked for.
2. **A registered base whose verification refuses its automount is re-provisioned in
   place**, which the existing recovery path already does for a verification failure,
   and verified again after the restart.
3. A case per direction measured above, through the verifier's own script, and a
   mutation row per check.

⛔ **Not a new flag, and not a warning over a base that passes.** The configuration
already says what the drives must be, and a report that says so over a guest that
disagrees is the defect.

## Decision

⭐ **Approved by the operator on 2026-09-14, and first in the work order.** Apply
the approach above before any later base work relies on automount verification.
The release waits for this entry to close.

## Consumers

⚠ By [`../docs/consumers.md`](../docs/consumers.md)'s definition, a caller whose base
drifted gets a re-provision, which restarts it, where it got an exit 0 over the
drift. The changelog says so.

## Prove

On a throwaway instance, each time from a base built with the first value:

```powershell
wsl-toolkit --instance NAME base ensure
wsl.exe -d wsl-toolkit-NAME -u root --exec /usr/bin/grep -E ' /mnt/c ' /proc/mounts
```

Passing is:

- `rw` then `ro`: the ensure re-provisions, and the mount line carries `ro`;
- `off` then `ro`: the ensure re-provisions, and a mount line appears carrying `ro`;
- `ro` then `off`: the ensure re-provisions, and no mount line remains;
- `base status --probe` prints the automount the guest has in each case;
- a mutation row per check red.

---

## Closing

**Closed 2026-09-14T15:04:56Z.** The verifier reads the Windows drives from
`/proc/mounts` and prints what they are, `base ensure` provisions again a base whose
drives disagree with `base.automount`, and `base status --probe` prints the setting
beside the drives. All five passing conditions hold on the tree's build, on the
throwaway `wsl-toolkit-m84`, an arch base with interop off:

| condition | measured |
| --- | --- |
| `rw` then `ro`: the ensure re-provisions, and the mount line carries `ro` | built `rw` in 74.1 s with `/mnt/c 9p rw`; the probe exit 1; `base ensure` exit 0 in 4.6 s through `re-provisioning in place`; then `/mnt/c 9p ro`, and all ten drive mounts `ro` |
| `off` then `ro`: the ensure re-provisions, and a mount line appears carrying `ro` | built `off` in 34.7 s with no drive mount; the probe exit 1; `base ensure` exit 0 in 4.7 s, re-provisioning; then `/mnt/c 9p ro` |
| `ro` then `off`: the ensure re-provisions, and no mount line remains | built `ro` in 63.9 s with `/mnt/c 9p ro`; the probe exit 1; `base ensure` exit 0 in 4.5 s, re-provisioning; then `grep` exit 1 and no drive mount |
| `base status --probe` prints the automount the guest has in each case | before each ensure `automount ro, and the guest's drives read rw`, then `rw` over `ro` and `ro` over `off`, `usable false`; after each ensure the two agree, `usable true`, and `--json` reads `access.automount_guest` equal to `access.automount` with no problem |
| a mutation row per check red | 16 rows, below |

⭐ **The directions the premise left unmeasured also hold**, chained on the same base
from the step before: `ro` then `rw` in 4.3 s, `rw` then `off` in 4.5 s and `off`
then `rw` in 4.4 s, each re-provisioning and each probe agreeing afterwards. ⭐ **Guest
root's own remount is caught:** `mount -o remount,rw /mnt/d` in an `ro` base exit 0,
then the probe exit 1 with the drives `mixed`, 9 read-only and 1 writable, and `base
ensure` exit 0 in 4.5 s put `/mnt/d` back to `ro`. The default `wsl-toolkit` base,
built `ro`, answered `automount ro, and the guest's drives read ro` and `usable true`
with no change.

| suite | Windows, `TEMP` at the 8.3 path | `golang:1.25` |
| --- | --- | --- |
| `check-go` | exit 0 | exit 0 |
| `wsl-toolkit` top-level cases | 296: 289 passed, 7 skipped, 0 failed | 295: 294 passed, 1 skipped, 0 failed |
| ShellCheck 0.9.0 in `ubuntu:24.04` | - | exit 0 over 34 tracked scripts |

`TestTheVerifierReadsTheDrivesItPromises` runs the verifier's own drive section through
`/bin/sh`, dash in `golang:1.25`, over ten mounts tables, and skips on Windows.

### ⛔ What driving it found

| found | what was done |
| --- | --- |
| a drive refusal read `a container did not run as toolkit (exit 3): verify: automount is ro, ...`, which sends a reader after the engine | `verifyError` names the setting: `the base does not match its configuration, checked as toolkit: ...`. An engine failure keeps its words, which the stale run state remediation reads |
| the first build of that message never fired on a real guest: its case passed no error, and `Exec` answers a guest's exit 3 with a `ProcessError` beside the code | the condition reads the code alone, and the case holds exit 124 with a `verify:` line as not a refusal |
| two builds from nothing failed when `geo.mirror.pkgbuild.com` stalled, `Operation too slow`, and each rolled its distribution back | nothing in this entry; the third attempt passed. The record carries it |

### The reviews

⭐ **The door sweep found two more doors that trusted the setting over the drives, and
both are fixed.** `verify` has five callers, `Status` and four paths in `EnsureWith`,
reached by `base status`, `base ensure`, `base recreate`, `ready` and the helper
route; `run` and `matrix` reach it only to build a base that is not registered. Then
every other reader of drive state:

- ⛔ `base shell --root` read the one-letter directories under `/mnt` and said `these
  Windows drives are mounted and writable` in the default base, where every drive is
  `9p ro` and root's `touch /mnt/c/...` answered `Read-only file system`.
  `MountedWindowsDrives` reads `/proc/mounts` by the verifier's rule, and the note
  names each drive by its mode, or says the table could not be read. Driven: the
  default base lists nine read-only drives; a throwaway `rw` base lists ten writable,
  then nine writable and `/mnt/d` read-only after root remounted it.
- ⛔ `base shell --here` checked the setting alone: on a base built `off` and set to
  `ro`, it printed `starting in this Windows directory` and the shell started in
  `/home/toolkit`. It now reads the mounts and refuses a base with none, exit 2 with
  `base ensure`, and a read that fails refuses nothing. Driven: exit 2 on the drifted
  base; after `base ensure`, the shell started under `/mnt/c`.

What would have made it fire again: a fourth reader of the base's drives. `grep` for
`/mnt` over the tool's Go and shell sources finds the verifier and the two fixed, the
provisioner that writes the setting, the Windows path translation, and `ready --smoke`
asking whether a container sees `/mnt/c`, which is about the container.

⭐ **The guard mutation proved 16 rows**, each seen green unmutated first: in
`golang:1.25` all 16 went red; on Windows the 10 whose cases run there went red and
the 6 verifier rows reported skipped. The rows: the ro-or-rw comparison, the refusal
of a mounted drive under `off`, the row printing the drives, a writable mount counted
writable, a read-only mount counted read-only, a mount below a drive counted, the
refusal message, only exit 3 read as a refusal, `mixed` read from the row, the
status row, the root note's mode, a mount below a drive in the root note, the root
note's unread table, and `--here`'s refusal and its read failure.

⛔ **The claim audit corrected the manual twice.** Its first wording said a drifted
base "answers exit 1 with both counts", and the refusal under `off` names no count;
it now says the refusal names what disagrees. Its table said every drive mount was
`9p`, where the drive listing measured only each mode; the type was read for `/mnt/c`
alone, and the row now says read-only and writable.

---

## WSL-85. A base reconfigured from passwordless sudo to none keeps it, and reports none

**Source** found on 2026-09-15 while driving `WSL-67`'s provider profiles.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** S, **Status** done

---

## Problem

The low-authority profile in
[`../tools/windows/wsl-toolkit/examples/common/access-profiles.md`](../tools/windows/wsl-toolkit/examples/common/access-profiles.md)
says its account cannot use `sudo -n`. A base built with `passwordless_sudo: true` and
then configured without it kept the account's rule, and `base status --probe` and `base
ensure` both reported `passwordless false` and a usable base over an account that could
still become root.

## Premise

⭐ **Measured on 2026-09-15 on the throwaway `wsl-toolkit-p67`**, an arch base with
automount and interop off, its state and its one grant under this repository's `.tmp`:

| step | answer |
| --- | --- |
| built with `passwordless_sudo: true`, then set to `false` | `base status --probe` exit 0, healthy, no problem |
| `base ensure` | exit 0 in 1.8 s, nothing provisioned, printing `sudo passwordless false` |
| `sudo -n true` as the account | granted |

- ⭐ **Read in `internal/toolkit/verify.sh`:** the `true` arm runs `sudo -n true` and
  refuses when it fails, and the `false` arm checks nothing.
- ⭐ **Read in `internal/toolkit/provision.sh`:** under `false` provisioning removes
  `/etc/sudoers.d/wsl-toolkit-ACCOUNT`, the one rule it writes. Measured the same day:
  after a grant change made `base ensure` provision the base, `sudo -n true` was
  refused.
- ⭐ **Measured read-only the same day:** the default `wsl-toolkit` base and
  `wsl-toolkit-podbox` have no `sudo`, so a check under `false` cannot turn either red.

## Approach

1. **The verifier's `false` arm refuses an account whose `sudo -n true` is granted**, so
   `base status --probe` answers exit 1 naming it and `base ensure` provisions in place,
   which the existing recovery path already does for a refusal.
2. A case that runs the verifier's own sudo section through a POSIX shell, with a
   `sudo` the case writes, under both settings, and a mutation row.

⛔ **Not a removal of rules this tool did not write.** Provisioning removes its own, and
a rule from elsewhere leaves the base refused.

## Decision

⭐ **Approved by the operator in chat on 2026-09-15**, "approve WSL-85": filed and fixed
in this session, before `WSL-70`.

## Consumers

⚠ By [`../docs/consumers.md`](../docs/consumers.md)'s definition, a caller whose base's
sudo disagrees with its configuration gets exit 1 from `base status --probe` and a
provision from `base ensure`, which restarts the base, where both answered exit 0. The
changelog says so. No fetched file changes.

## Prove

On a throwaway instance:

```powershell
wsl-toolkit --instance NAME base status --probe --json
wsl-toolkit --instance NAME base ensure
wsl-toolkit --instance NAME base exec -c 'sudo -n true'
```

Passing is:

- set from `true` to `false`: the probe exit 1 naming the account's sudo, the ensure
  provisions again, then `sudo -n true` is refused and the probe exit 0;
- set from `false` to `true`: the same, with `sudo -n true` granted;
- a mutation row red.

---

## Closing

**Closed 2026-09-15T01:19:47Z.** The verifier refuses an account whose `sudo -n true` is
granted under `passwordless_sudo: false`, so `base status --probe` names the disagreement
and `base ensure` provisions the base again, which removes the rule this tool wrote. A
verification that fails is named by the guest's own line, past any line wsl.exe writes
about itself. All three passing conditions hold on the tree's build, on
`wsl-toolkit-p67`:

| condition | measured |
| --- | --- |
| set from `true` to `false`: the probe exit 1 naming the account's sudo, the ensure provisions again, then `sudo -n true` is refused and the probe exit 0 | the probe exit 1 in 0.5 s, `the configured account can use sudo without a password, and passwordless sudo is off`; `base ensure` exit 0 in 16.0 s through `re-provisioning in place`; then `sudo -n true` refused, and the probe exit 0 with no problem |
| set from `false` to `true`: the same, with `sudo -n true` granted | the probe exit 1 in 0.5 s, `the configured account cannot use sudo without a password`; `base ensure` exit 0 in 15.9 s, provisioning again; then `sudo -n true` granted, and the probe exit 0 |
| a mutation row red | 3 rows, below |

⭐ **A rule this tool did not write is refused and left in place.** Under `false`, a
second rule at `/etc/sudoers.d/zz-elsewhere` granting the account sudo made the probe exit
1, and `base ensure` exit 2 in 14.5 s with `re-provisioned and it still does not verify`
and the same refusal, with `sudo -n true` still granted. With that rule removed as root,
`base ensure` exit 0 in 2.5 s without provisioning, and the probe exit 0.

| suite | Windows, `TEMP` at the 8.3 path | `golang:1.25` |
| --- | --- | --- |
| `check-go` | exit 0 | exit 0 |
| `wsl-toolkit` top-level cases | 305: 297 passed, 8 skipped, 0 failed | 304: 303 passed, 1 skipped, 0 failed |
| ShellCheck 0.9.0 in `ubuntu:24.04` | - | exit 0 over 34 tracked scripts |

`TestTheVerifierReadsTheSudoItPromises` runs the verifier's own sudo section through
`/bin/sh`, dash in `golang:1.25`, under both settings, with a `sudo` the case writes on a
`PATH` that holds nothing else, and skips on Windows.

### ⛔ What driving it found

| found | what was done |
| --- | --- |
| over the rule from elsewhere, the first build's `base ensure` answered `a container did not run as agent (exit 3): wsl: Failed to start the systemd user session for 'agent'. See journalctl for more details.`: wsl.exe wrote its own line ahead of the verifier's refusal, and the message took the first line | `verifyError` takes the last `verify:` line as the refusal and names any other failure by the first line that is not wsl.exe's; driven again, the ensure named the account's sudo |
| building the throwaway printed 93 `wsl: Failed to translate` lines, for the working directory and each Windows `PATH` entry, between provisioning and the restart | nothing in this entry. Read, not measured: the provisioner writes `appendWindowsPath=false`, which WSL reads at that restart. The record carries it |

### The reviews

⭐ **The door sweep** asked what else reaches a sudo decision, and what else names an
error by a line of a guest's stderr. `verify` has five callers, `Status` and four paths in
`EnsureWith`, reached by `base status`, `base ensure`, `base recreate`, `ready` and the
helper route, so the refusal is one change. `base exec --root` and `base shell --root`
reach root through `wsl.exe -u root`, `wsl.go` line 402, and never through sudo, whatever
the setting says. `config` and `base status` print the configured value, and under
`--probe` a disagreement is a problem beside `usable false`. ⚠ **It found 23 more lines,
in nine files, that build an error from the first line of a guest command's stderr**,
where wsl.exe can write first: `adapters.go`, `base.go`, `cleanup_run.go`, `engine.go`,
`inspect.go`, `job.go`, `matrix.go`, `resources.go` and `throwaway.go`. None was measured
naming a wsl.exe line, so none is changed here, and the record carries them. What would
make the sweep fire on sudo again: a second writer of the rule, and `grep` for `sudoers`
finds only `provision.sh`.

⭐ **The guard mutation proved 3 rows**, each seen green unmutated first: the verifier
refusing sudo under `false`, a refusal read past wsl.exe's lines, and wsl.exe's lines left
out of a failure's detail. In `golang:1.25` all 3 went red; on Windows the two in `base.go`
went red and the verifier row reported skipped. ⚠ **The gate's `mutations` rule then
refused two of `WSL-84`'s rows**, which named the line `verifyError` no longer has; both
now address the refusal branch, `if code == verifyRefused {`, and both went red again on
Windows.

⭐ **The claim audit** read the manual's paragraph and table, this entry and the changelog
against the drive's logs. It corrected the manual's first draft, which said "a base that
disagrees answers exit 1" and named no command, where `base ensure` provisions and `base
status --probe` is what answers exit 1. It checked the `--root` sentence above against
`wsl.go` before it was written.

---

## WSL-86. The debian and fedora presets build a base that cannot run a container

**Source** found on 2026-09-15 while driving `WSL-70`'s four-preset prove.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** M, **Status** done

---

## Problem

`base ensure` from the `debian` and `fedora` presets exits 2. Two of the four presets
the tool offers cannot produce a base at all, and the failure is reported in the words
of a container engine rather than as the missing package or the dropped privilege it
is: debian's reads as a netavark error and fedora's as a broken image.

## Premise

⭐ **Measured on 2026-09-15**, `base ensure` with `toolset developer`, automount and
interop off, on throwaway instances under this repository's `.tmp`:

| preset | answer |
| --- | --- |
| arch | exit 0 in 51.5 s; `package manager: pacman on arch`; the probe healthy; all thirteen developer commands present |
| alpine | exit 0 in 44.2 s; `package manager: apk on alpine`; healthy; all thirteen present |
| debian | exit 2 in 123.2 s: the developer packages installed, then the QEMU binary-format installer's rootful run answered `netavark: nftables error: unable to execute nft: No such file or directory` |
| fedora | exit 2 in 97.8 s: built, then verification answered `a container did not run as agent (exit 125)`, naming podman's shared-mount warning |

- ⭐ **debian fails the same way before `WSL-70`'s change.** `4c573cf`'s build, with no
  toolset, answered the same `nft` error in 33.6 s. `base presets` still carries its
  2026-09-09 figure.
- ⭐ **fedora's own error, read with a diagnostic build that printed verification's
  whole stderr:** `newuidmap: write to uid_map failed: Operation not permitted` and
  `cannot set up namespace using "/usr/sbin/newuidmap": should have setuid or have
  filecaps setuid`. The working arch base's `newuidmap` carries `cap_setuid=ep`; the
  imported Fedora rootfs's carries neither that nor the setuid bit. Fedora had never
  been built on this host.
- ⚠ **Read, not measured:** a rootfs tarball unpacked without extended attributes
  keeps a binary's bytes and drops its file capability, which is why an imported
  Fedora differs from one installed in place.

## Approach

1. ⭐ **The seam is `internal/toolkit/provision.sh`'s engine section**, the
   per-family `case` on `$FAMILY`. The `apt` arm installs no firewall package where
   the `apk` arm installs `iptables ip6tables` and the `pacman` arm `iptables-nft`.
   Add `nftables`, which is what supplies `nft` on Debian.
2. ⭐ **The second seam is the `newuidmap`/`newgidmap` loop below it**, which asks
   `command -v` and nothing else. ⛔ **The privilege is the thing being checked, not
   the path.** It reads the setuid bit and the file capability, restores a missing
   capability with `setcap`, and refuses with the capability named when it cannot.
3. The capability tools come from each family's own spelling, and that install is
   allowed to fail so a spelling this host has not measured is named by the refusal
   rather than ending the build inside a package manager.
4. Both sections get begin and end markers, so a case can run the real text.

⛔ **Do not move the engine packages into the shared table.** `WSL-70`'s ruling keeps
them per family on purpose: the table is the tool-set names, and the engine is the
provisioner's own.

⛔ **Do not make the check pass when it cannot tell.** A guard that answers "probably
fine" over a missing capability is the guard that let this reach verification.

## Decision

⭐ **Approved by the operator in chat on 2026-09-15**, accepting the recommendation as
proposed: file it, fix both in `provision.sh` with a regression case each, and prove
it on all four preset builds, which is also `WSL-70`'s open prove.

## Consumers

**None: this change cannot reach a fetched file.** `provision.sh` is embedded in the
executable and is not fetched by URL; the shared table it reads is untouched, so
[`../docs/consumers.md`](../docs/consumers.md)'s `bootstrap.sh` row is unaffected and
the `package-table` gate rule stays green without a regeneration.

## Prove

```powershell
.tmp/wsl-toolkit.exe base ensure --preset PRESET --toolset developer --automount off --interop off
```

Passing is:

- all four presets exit 0 and their probes report healthy;
- `TestTheProvisionerInstallsWhatEachFamilysEngineNeeds` holds what each of the six
  families installs, with the `apt` arm carrying `nftables`;
- `TestTheProvisionerRestoresTheCapabilityRootlessIdMappingNeeds` goes red when the
  restore is removed and when the check is returned to asking `command -v` alone. ⛔
  Plant each and read the exit code, unpiped.

---

## Closing

**Closed 2026-09-15T05:07:50Z.** The `apt` arm installs `nftables`, so netavark finds the `nft` it
shells out to. The id-mapping check reads the privilege rather than the path: it accepts
a setuid bit or the capability, restores a capability an imported rootfs dropped, and
refuses with the capability named when it cannot. Both sections carry begin and end
markers so a case runs the real text.

⭐ **All four presets build and verify**, on throwaway instances under this repository's
`.tmp`, with automount and interop off, on 2026-09-15:

| preset | answer |
| --- | --- |
| arch | exit 0 in 77.3 s, toolset `developer`; `pacman on arch`; podman 6.1.1; probe exit 0 healthy; all thirteen developer commands present |
| alpine | exit 0 in 172.9 s, toolset `developer`; `apk on alpine`; podman 5.8.6; healthy; all thirteen |
| debian | exit 0 in 94.0 s, toolset `developer`; `apt on debian`; podman 5.4.2; healthy; all thirteen. ⭐ `/usr/bin/newuidmap is setuid` and `/usr/bin/newgidmap is setuid`: Debian needs no capability, and `nftables` is the whole of its fix |
| fedora | exit 0 in 48.4 s, toolset `none`; `dnf on fedora`; podman 5.8.4; healthy. ⭐ `/usr/sbin/newuidmap arrived without cap_setuid and now carries it`, and the same for `newgidmap` |

⚠ **fedora is proved at `toolset none` and the other three at `developer`, and that is
stated rather than smoothed over.** Its `developer` build installs 99 packages from a
mirror that answered in tens of KiB/s on the day: one such build took 420 s and a second
ran out of the 30-minute provisioning budget at 1811.7 s. What that 420 s build DID prove
is the half this entry is about - the capability was restored, podman ran, and `base exec`
found all thirteen commands. It then failed verification for an unrelated reason, below.

⛔ **And that reason is a third defect, found by driving and not by the suite.**
`base ensure` on that 420 s fedora build answered exit 2 with `/mnt/e exists even though
automount is off`. The provisioner's sweep at `provision.sh:494` removes the mount points
WSL's first start leaves behind, and it runs once: that build removed **nine** where the
arch, alpine and debian builds each removed **ten** and the 48.4 s fedora build removed
ten. This host carries ten fixed drives and E: is an external HDD. A drive WSL mounts
between the sweep and the restart that applies `automount off` is left behind, and the
verifier is right to refuse it. ⚠ Recorded in [`PROGRESS.md`](PROGRESS.md) under "Found,
and not filed"; it is not this entry's, and it is not the presets'.

### The three reviews

⭐ **1. The door sweep - what other door reaches this code?** The change touches what
`base ensure` installs and what it then checks, so the enumeration is every path into
provisioning and every other reader of the id mapping.

- **One path, checked:** `provisionRequest` in `base.go` assembles the only provisioning
  run there is, so `base ensure`, `base recreate` and a repair all get both halves.
  `TestTheProvisioningRunIsTheSharedTableThenTheProvisioner` holds that payload.
- **The other half of the mapping:** `/etc/subuid` and `/etc/subgid` are written at
  `provision.sh:399`. The binaries' privilege and the ranges they read are two resources
  and were authorized by one check; both are checked now.
- **Reaches an existing base:** `wsl-toolkit-podbox` is provisioned by this same script,
  so its next `base ensure` runs the new check over it. Recorded, not run: it is not this
  session's distribution to change.
- ⛔ **Found, recorded, not fixed here:** `verifyError` in `base.go` names the FIRST guest
  line, and podman writes a shared-mount warning before its error, so fedora's real cause
  needed a diagnostic build to read. It is `PROGRESS.md` finding 12's family seen from the
  other side - not wsl.exe's line but the engine's own - and it is tracked there as
  finding 15 rather than widened into this entry.

⭐ **2. The guard mutation - can the new guards actually fail?** `provision.sh` is inside
the Go module, so `repo mutate` can hold these, and each case passed unmutated first.
Run in `golang:1.25`, where the cases do not skip:

```text
  ok       provisioner: the apt engine arm installs the nft netavark shells out to        1 case(s), went red
  ok       provisioner: a dropped id-mapping capability is restored, not reported         1 case(s), went red
  ok       provisioner: an id-mapping capability that cannot be read is refused, not passed 1 case(s), went red

3 of 3 guards proved.
```

The third is the one that matters most: it returns the check to passing when it cannot
read the capability, which is the shape that let the defect through in the first place.

⛔ **And CI failed on that third row, which is the lens firing on its own work.** The
case it named could only be staged where no `getcap` exists at any absolute path the
section reaches for. `golang:1.25` has none and it went red there; `ubuntu-latest` carries
`/usr/sbin/getcap`, so on CI the case did not run, the other five stayed green with the
guard removed, and `repo mutate` reported **THEATRE**: `1 case(s), still green`, 284 of 286
guards proved, at 05:25:19Z on run 34931576687. ⚠ A row that goes red on one host and is
theatre on another is a row that proves nothing on the host that matters.

⭐ **Fixed by splitting it out.** `TestTheProvisionerRefusesAnIdMappingCapabilityItCannotRead`
holds that one arrangement and skips the whole case where a capability tool exists outside
`PATH`, so the row reports SKIPPED rather than theatre there - the same shape as the
`distro: NUL is not a console` row CI has always carried. Measured both ways on 2026-09-15:

```text
golang:1.25, no getcap
  ok       provisioner: an id-mapping capability that cannot be read is refused, not passed  1 case(s), went red
  3 of 3 guards proved.

golang:1.25 with libcap2-bin, getcap at /usr/sbin/getcap
  SKIPPED  provisioner: an id-mapping capability that cannot be read is refused, not passed  1 case(s), all skipped here
  MUTATE EXIT 0
```

⭐ **3. The claim audit - which sentence is not backed by an artefact?**

- ⛔ **Found the hard way, and it invalidated a first set of measurements.** The first
  four preset builds were run with an executable built BEFORE `provision.sh` was edited.
  `provision.sh` is embedded with `go:embed`, so the run measured the old provisioner and
  debian failed with the same netavark error the fix removes. The builds recorded above
  are from a rebuilt executable. ⚠ A build figure whose binary predates the change is a
  number that was not measured, and it read exactly like one that was.
- The header's claim that four of twelve managers have had a base built is now true of
  four builds taken on one day rather than four taken across three.
- `presets.go`'s figures were stale and are corrected in `WSL-87`'s closing, which found
  them.

---

## WSL-87. bootstrap.sh's CodeGraph install fails wherever /bin/sh is dash

**Source** found on 2026-09-15 while driving `WSL-70`'s agent matrix.
**Category** wsl-toolkit-go, **Priority** P1, **Effort** S, **Status** done

---

## Problem

`sh bootstrap.sh --toolset agent` installs CodeGraph by default. On Debian, Ubuntu and
Void, whose `/bin/sh` is dash, every package installed and the run then exited 1 with `npm
did not write exactly one archive into ` and no directory after it, so a caller that fetches
the file gets a failed bootstrap over a machine that has everything but CodeGraph.

## Premise

⭐ **Measured on 2026-09-15**, `matrix --images all -c 'sh /work/scripts/common/bootstrap.sh
--toolset agent'`:

| image | answer |
| --- | --- |
| debian, debian 12, ubuntu 22.04, void-musl | every package present, then `[-] npm did not write exactly one archive into ` with an empty directory, exit 1 |
| rocky 8, node v10.24.0 and npm 6.14.11 | `[-] npm could not fetch @colbymchenry/codegraph-linux-x64@1.6.0`, exit 1 |
| alpine, arch, fedora, opensuse, photon, wolfi | CodeGraph installed |

- ⭐ **Measured the same day**, a script holding `fetch_verified_npm`'s two lines that name its
  directory: under dash in `debian:latest` the directory is an empty string, and under busybox
  in `alpine` and bash in `arch` it is the directory.
- ⚠ **Read, not measured:** dash's `read` answers 1 on a last line with no newline, and `set
  -e` then ends the substitution before its `printf`. The line came in with `2ec9238` on
  2026-09-12.
- ⚠ **Read, not measured:** npm 6 has no `--pack-destination`, which the fetch passes.

## Approach

1. `fetch_verified_npm` names its directory without reading it back through `read`.
2. A too-old npm is named for what it is before the fetch, rather than as a fetch that failed.
3. A case that runs the fetch's directory step under dash, and the agent matrix again with
   CodeGraph on.

## Decision

⭐ **Approved by the operator in chat on 2026-09-15**, "approve WSL-87": filed and fixed in
this session.

## Consumers

⚠ `scripts/common/bootstrap.sh` is fetched by URL, and [`../docs/consumers.md`](../docs/consumers.md)
registers no consumer of it. A caller on a dash system gets CodeGraph and exit 0 where it got
exit 1. The changelog says so.

## Prove

```powershell
wsl-toolkit matrix --images all --workspace . -c 'sh /work/scripts/common/bootstrap.sh --toolset agent'
```

Passing is:

- debian, debian 12, ubuntu 22.04 and void-musl install CodeGraph and exit 0;
- rocky 8's npm 6 is named as too old for the fetch;
- the other images answer as they did with CodeGraph off.

---

## Amendment, 2026-09-15: built and proved by hand, and the operator checkpointed it

⭐ **Built:**

- `fetch_verified_npm` names its directory by joining the package's parts with `-`, in a
  loop, where it read the name back through `read`.
- `npm_packs_to_a_directory` answers whether an npm has `--pack-destination`, and
  `install_codegraph` refuses one that has not, naming its version, before any fetch.
- `TestBootstrapFetchesAnNpmArchiveIntoANamedDirectoryUnderDash` runs the file's own
  `fetch_verified_npm` under dash inside an `if`, with npm and the two digest readers stood
  in for, and `TestBootstrapNamesAnNpmTooOldToPackToADirectory` holds the versions below.
  Both read `scripts/common/bootstrap.sh` from the tree and skip where there is no dash.
- The scripts README says CodeGraph needs npm 7.18.0.

⭐ **The npm boundary, measured on 2026-09-15:** npm 6.14.11 on rocky 8 took the directory
for a second package, `ENOLOCAL`, wrote the archive into its working directory and exited
1; through `npx` in `debian:latest`, npm 7.17.0 exited 254 and wrote nothing, and npm
7.18.0 and 7.18.1 wrote the archive into the directory.

⛔ **No mutation row can hold these cases**, because `repo mutate` copies a module and
`bootstrap.sh` is in none. Each was planted by hand in `golang:1.25`, the case green on
the tree first:

| planted in the container's copy | the case |
| --- | --- |
| the two lines that read the directory back, as `2ec9238` wrote them | red: dash answered `refused failures=1`, with `mkdir: cannot create directory ''` |
| `7.1[0-7].*` taken out of the version check | red: `npm "7.17.0" answered yes` |
| both restored | green |

⭐ **The agent matrix with CodeGraph on, over the fix, as far as it had run at the
checkpoint:** debian, debian 12, ubuntu 22.04 and void-musl each `codegraph=1.6.0` and
`failures=0`, where each had exit 1; chimera `codegraph=1.6.0` beside its `openssh`
conflict. Rocky 8 named its npm, `codegraph is fetched with npm pack --pack-destination, which needs npm
7.18.0 or later, and this npm answers 6.14.11`, where it said `npm could not fetch`; alpine,
arch, opensuse, photon and wolfi `codegraph=1.6.0` and `failures=0`, as before; gentoo as
before, with no node. ⚠ Fedora's row was still installing from a mirror answering in KiB/s
when the operator checkpointed the session, so the run is not counted as a whole.

### Still open at the checkpoint, and answered by the closing below

1. The three reviews and the closing. ⚠ **The door sweep has begun:** every other `read`
   in `bootstrap.sh` sits in a `while`, an `if` or a `||`, so none can end a substitution
   under `set -e`; and `install_codegraph` checks the architecture and not the kernel, so
   on a BSD it would fetch the Linux package, read and not measured.
---

## Closing

**Closed 2026-09-15T05:07:50Z.** `fetch_verified_npm` names its work directory without reading it
back, an npm with no `--pack-destination` is named before any fetch, and a kernel
codegraph publishes no package for is named the same way. Three cases hold the three,
each red with its defect planted by hand.

```text
wsl-toolkit matrix --images all --workspace . --exclude .tmp --exclude .codegraph \
  --container-lifecycle ephemeral -c 'sh /work/scripts/common/bootstrap.sh --toolset agent'

ran 13  failed 3  unreached 0  timed out 0, in 1184.45 s
```

| row | answer |
| --- | --- |
| debian, debian 12, ubuntu 22.04, void-musl | `codegraph=1.6.0`, `failures=0`, exit 0. Each exited 1 with an empty directory before the fix |
| rocky 8 | `codegraph is fetched with npm pack --pack-destination, which needs npm 7.18.0 or later, and this npm answers 6.14.11`, `codegraph=none`, `failures=1`. It said `npm could not fetch` before |
| alpine, arch, fedora, opensuse, photon, wolfi | exit 0, as before |
| chimera | CodeGraph installed; it fails on its own pre-existing `openssh` conflict |
| gentoo | as before, with no node and no portage tree |

⭐ **3 failed rather than 2, and the third is the tool being honest.** Rocky 8's npm cannot
fetch codegraph and now says so, where `--codegraph none` gives 13 ran and 2 failed. The
two figures are not comparable and each is recorded with its flag.

⭐ **Run twice, and the second is over the final tree.** The first ran while the door
sweep's kernel guard was still being written, so its workspace copy predates it; the
second carries the whole change.

```text
second run, same command, 2026-09-15T04:37:04Z
ran 13  failed 4  unreached 0  timed out 1, in 1816.82 s
```

⚠ **The extra failure is fedora's mirror, not the tree.** It answered in tens of KiB/s all
session and its row hit the 30-minute budget with exit 124, where it exited 0 in the first
run. Every row this entry is about answered identically in both: the four dash images
`codegraph=1.6.0` and `failures=0`, rocky 8 naming its npm, chimera and gentoo failing as
they always have.

⭐ **The caller-visible surface is unchanged.** In `golang:1.25`, `HEAD`'s `bootstrap.sh`
and the tree's gave byte-identical `--help`, `--list-names` and `--list-providers`, each
exit 0. ⚠ The `--dry-run --toolset agent --codegraph none --json` run differed by one
line, and the difference is the staging rather than the change: `HEAD`'s copy was
extracted to `/tmp`, where `script_dir` finds no `tmux.conf` beside it, so it reported
the file as skipped where the tree's copy would install it. Read rather than reported,
because a `DIFFERS` nobody opens is a break nobody notices.

### The three reviews

⭐ **1. The door sweep - what other door reaches this code?** The fix adds three
affordances, and each has exactly one caller: `npm_packs_to_a_directory` and the kernel
`case` are called by `install_codegraph`, and `fetch_verified_npm`'s naming is reached
from its two calls there. What the enumeration missed was found by grepping for it:

- **Every `read` in the file, all eight**, at lines 237, 485, 546, 589, 774, 905, 1235
  and 1584. Each sits in a `while`, an `if` or a `||`, so none can end a command
  substitution under `set -e` the way the defect's did. The closest in shape is the
  report's `ssh -V` reader at 1584, which carries its own `|| line=""`.
- ⛔ **Found and fixed:** `install_codegraph` read `$ARCH` and never `$KERNEL`, so a
  FreeBSD, NetBSD or OpenBSD amd64 host resolved `codegraph-linux-x64` and fetched it.
  It is named before the fetch now, as the too-old npm is, and `has_upstream_route`
  three functions above was already the shape for it.
- **Found, recorded, nothing built on it:** `fetch_verified_npm` calls `split_on`, which
  lives INSIDE the shared package table block and is generated into `packages.sh` for the
  base provisioner. A change to `split_on` made for the table changes the npm work
  directory's name.
- **Found, recorded:** `bootstrapSource` reads five levels above its own package, outside
  the Go module, so a `repo mutate` row naming one of these cases would report the file
  unreadable rather than a guard's verdict. No row names one.

⭐ **2. The guard mutation - can the new guard actually fail?** The kernel guard was
planted by hand in `golang:1.25`, because `repo mutate` copies a module and
`bootstrap.sh` is in none - verified in `mutate.go`, whose `one` calls `copyTree` over
`root/module` alone.

```text
unmutated exit 0
planted: the kernel guard removed
planted exit 1
--- FAIL: TestBootstrapNamesAKernelCodegraphPublishesNoPackageFor (0.02s)
    FreeBSD amd64 reached for "@colbymchenry/codegraph-linux-x64", want ""
    FreeBSD amd64 said "step: codegraph resolves to 1.6.0\n", want a line carrying
      "codegraph publishes a Linux package only, and this kernel is FreeBSD"
    NetBSD amd64 reached for "@colbymchenry/codegraph-linux-x64", want ""
restored exit 0
```

⛔ **And the pass found a defect in its own method, which is the point of the lens.** The
FIRST plant reported `planted exit 0` over the removed guard. `go test` had served a
cached result: the only file that changed was a shell script outside the module, which
the Go build cache does not track, and the case reads it at run time. `-count=1` is what
made the guard fire. ⭐ `repo mutate` already passes `-count=1`, at `mutate.go:194`, so
its rows were never exposed to this; a hand-rolled plant is, and this one was, twice, for
about four minutes.

⭐ **3. The claim audit - which sentence is not backed by an artefact?** The amendment's
four "built" claims were re-read against the tree and each resolves: the directory naming
at `bootstrap.sh:1035`, `npm_packs_to_a_directory` at 993, its refusal at 1093, and the
scripts README at 508. The consumers claim was measured rather than asserted, above.

- ⛔ **Found and fixed:** `presets.go` published a build figure for a preset that had not
  built since. Its `debian` row carried `37s, 556 MiB disk, podman 5.4.2`, taken on
  2026-09-09, and `base ensure` from that preset has ended in netavark's `nft` error every
  time it has been run since. `fedora` said `not measured on this host`, which was honest.
  Both now carry what was measured today, with the date and the conditions on them.
- The amendment's npm boundary figures - 6.14.11 and 7.17.0 without `--pack-destination`,
  7.18.0 and 7.18.1 with - are last session's measurement and were not taken again. The
  case holds them as a table, so a future npm that disagrees fails it.

## WSL-88. The pi adapter, and herdr's first lifecycle authority in this base

**Source** the operator's work order of 2026-09-14, "Author approved entries for
`pi` and `omp` before either adapter is built", and the reference sweep of
2026-09-15 that costed both.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** M, **Status** open

⚠ **Amended 2026-09-15: the adapter is BUILT and NOT DRIVEN.**
`adapters/pi/install.sh` and `probe.sh` exist, the executable registers `pi`, and
the gate compares its generated copy. ⛔ **No base has installed it**, so every
passing condition below is still owed, and the `Presets` row naming `arch` says
where it WILL be driven rather than where it has been.

---

## Problem

`adapters/README.md` names `pi` and `omp` as the next adapters and builds
neither. A base configured for agents therefore carries Muse, which herdr can
only classify from its screen, and none of the agents herdr has **lifecycle
authority** for. The operator cannot compare an agent whose state herdr knows
exactly against one it infers.

## Premise

⭐ **Read on 2026-09-15**, from `earendil-works/pi` at commit
`f9bcd351dc3cedf9` and `herdrdev/herdr` at
`052779c4159ed851`. ⛔ **None of it was run**, and the
sweep is [`../docs/reference-sweeps/usable.md`](../docs/reference-sweeps/usable.md)
section 9.

| what | read |
| --- | --- |
| install | `npm install -g --ignore-scripts @earendil-works/pi-coding-agent`. ⭐ pi documents `--ignore-scripts` as normal: "Pi does not require install scripts for normal npm installs" |
| ⭐ no piped script | there is a `curl \| sh` installer and **nothing here needs it**, so this repository's refusal costs nothing |
| herdr integration | `herdr integration install pi`, which writes `~/.pi/agent/extensions/herdr-agent-state.ts`, or `$PI_CODING_AGENT_DIR/extensions/` when set. Uninstall removes only that file |
| authority | ⭐ **lifecycle hooks, state AND session.** herdr's agents table: "Pi \| lifecycle hooks when installed; otherwise screen manifest \| state and session" |
| restore | needs Pi integration version **2**; `herdr integration status` prints installed versions |
| ⛔ a trap | pi binds `Enter` to submit and `Shift+Enter` to newline, and tmux strips modifiers by default. `WSL-71`'s work added the version-guarded `extended-keys` to this tree's `tmux.conf` for exactly this |
| the reference to copy | herdr names `prime-agent`'s own `herdr-agent-state.ts` as the real-world example: activates only inside herdr, maps events to `working`, `idle`, `blocked`, preserves ordering, releases on exit |

⚠ **The base already has Node and npm** at `toolset developer`, so the adapter
adds a package rather than a toolchain.

## Approach

1. **`adapters/pi/install.sh`**, following `adapters/muse/` and the contract in
   [`../tools/windows/wsl-toolkit/adapters/README.md`](../tools/windows/wsl-toolkit/adapters/README.md):
   look before changing anything, end with `adapter-complete pi`.
2. ⭐ **Install with `--ignore-scripts` and pin the version**, then
   `herdr integration install pi` **in the base**, because herdr copies nothing
   onto an SSH host.
3. **`probe.sh`** prints `version`, the integration version from
   `herdr integration status`, and a `problem` line for each thing wrong.
4. ⚠ **The launcher question is `WSL-78`'s, not this entry's.** If `pi` gets a
   `pi.exe`, it inherits the wrapper problem: herdr cannot see an agent behind a
   wrapper unless `HERDR_AGENT` is set on the herdr side.

⛔ **Do not build a second install path.** `base agent` and the adapter contract
exist; this is one directory of two scripts, not a new surface. ⛔ **Do not pipe
pi's installer into a shell**, and do not add pi to `bootstrap.sh`'s table, which
is fetched by URL and is not where a herdr-specific agent belongs.

## Decision

⭐ **No fork.** The install route, the integration command and the authority model
are all documented by their own projects, and the alternative - screen detection
only - is what the base already has with Muse.

## Consumers

None: `adapters/` has no row in [`../docs/consumers.md`](../docs/consumers.md).
⚠ It stays none only while pi is kept out of `bootstrap.sh`, which is fetched by
URL.

## Prove

```powershell
wsl-toolkit --instance base base status --probe --json
wsl-toolkit --instance base base exec -c 'herdr integration status'
```

Passing is:

- the probe reports the `pi` adapter healthy with a `version` line, exit 0 read
  from the process;
- `herdr integration status` names pi at integration version **2** or later;
- ⭐ **an agent started in a pane reaches `idle` and then `working` from herdr's
  own report rather than from its screen**, shown by `herdr agent explain` naming
  a lifecycle authority rather than a manifest rule;
- `base remove --yes` takes the adapter's half of this machine away.

---

## WSL-89. The omp adapter, and the directory collision herdr refuses

**Source** the operator's work order of 2026-09-14, alongside `WSL-88`.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** M, **Status** open

⚠ **Amended 2026-09-15: the adapter is BUILT and NOT DRIVEN**, on the same terms
as `WSL-88`. ⛔ The collision refusal below is written and has never been reached
on a real base, which is the one condition here that most wants driving.

---

## Problem

The same as `WSL-88`, for omp. ⛔ **And one thing that is not the same:** a base
that carries both pi and omp can be configured so that herdr **refuses** the omp
integration, and the failure is a refusal at install time rather than anything an
operator would predict.

## Premise

⭐ **Read on 2026-09-15**, from `can1357/oh-my-pi` at commit
`6a0b915dcac4576f` and herdr at the commit above. ⛔ **None
of it was run.**

| what | read |
| --- | --- |
| what it is | ⭐ **a fork of pi**, upstream `badlogic/pi-mono`, published as `@oh-my-pi/pi-coding-agent` |
| herdr integration | `herdr integration install omp`, writing `~/.omp/agent/extensions/herdr-omp-agent-state.ts` |
| ⛔ the collision | herdr resolves omp's agent directory as `PI_CODING_AGENT_DIR` when set, else `$HOME/$PI_CONFIG_DIR/agent` when `PI_CONFIG_DIR` is set, else `~/.omp/agent`. **"If Pi and OMP resolve to the same extension directory, Herdr refuses the OMP install so the OMP extension cannot be loaded by Pi."** |
| authority | ⭐ lifecycle hooks, state **and** session, reported through herdr's socket API. It "does not require native process detection for the `omp` executable" |
| restore | `omp --resume=<session>`, integration version **3** |
| extensibility | `.omp/hooks/pre/*.ts` factories are loaded as extension modules; `--hook` is an alias for `--extension` |

⚠ **`PI_CODING_AGENT_DIR` is read by BOTH**, which is what makes the collision
reachable: one variable exported for pi silently redirects omp onto pi's
directory.

## Approach

1. **`adapters/omp/install.sh`**, the same shape as `WSL-88`'s.
2. ⛔ **Refuse the collision before herdr does, and say which variable caused
   it.** Resolve both directories the way herdr resolves them, compare, and fail
   with the two paths and the variable named. A refusal from the adapter names the
   cause; one from `herdr integration install` names only itself.
3. `probe.sh` reports the resolved agent directory as a fact, so a base whose
   environment changed later is visible in `base status --probe`.
4. ⚠ **A base carrying pi and omp together is the case to drive**, because a base
   carrying one is the case that cannot fail.

⛔ **Do not set `PI_CODING_AGENT_DIR` or `PI_CONFIG_DIR` for the account** to work
around it. A variable this tool exports to fix its own install is one the operator
cannot see and will not expect.

## Decision

⭐ **No fork on the install.** One decision is open and it is small: whether the
adapter **refuses** a colliding configuration or **separates** the directories
itself. Recommended: **refuse and name the cause.** Separating means writing an
environment variable into the account for a reason the operator never chose,
which is the thing this repository's own rules keep finding to be wrong later.

## Consumers

None, on the same terms as `WSL-88`.

## Prove

```powershell
wsl-toolkit --instance base base status --probe --json
wsl-toolkit --instance base base exec -c 'herdr integration status'
```

Passing is:

- the probe reports the `omp` adapter healthy with a `version` line and the
  resolved agent directory, exit 0 read from the process;
- `herdr integration status` names omp at integration version **3** or later;
- ⭐ **with pi installed too, both integrations are present and their directories
  differ**, read back from the probe;
- ⛔ **a base configured so the two collide is REFUSED by the adapter, naming both
  paths and the variable**, and the refusal is a mutation row.

---

## WSL-90. herdr built nightly from its development branch, published here, and followed by the herdr adapter

**Source** the operator on 2026-09-15, "let's build herdr ourself (now locally for
windows) and if it works, we will create a dedicated nightly builder for it on github
and publish it on our repo", and the rulings they gave in chat the same day, below.
**Category** wsl-toolkit-go, **Priority** P2, **Effort** L, **Status** open

---

## Problem

The herdr the base pins, 0.9.0, cannot do two things this repository's herdr guide
needs. Its Windows `--remote` client draws nothing until the window is focused and
delivers no typed text and no prefix command, and it has no `--machine` prefix. herdr's
development branch fixes the first and carries the second, and herdr publishes no
build of that branch.

## Premise

⭐ **Measured on 2026-09-15**, with every number in `WSL-76`'s amendment of that date:

- herdr's development branch at `052779c4159ed851` builds on this host for
  `x86_64-pc-windows-msvc` in 227 s and for `x86_64-unknown-linux-musl` in 223.4 s.
- Driven in a Windows pseudo console against the base's server, which is how herdr's
  collaborator verified `#4038`: the 0.9.0 client drew nothing for 10 s and drew
  after a focus-in, and ran no typed text and no new tab; the development client drew
  at once, ran the typed text, made the tab, and detached. Every result was read from
  the server.
- The development client and server together answer `--machine base agent list` with
  exit 0 in 1.3 s; the development client against a 0.9.0 server exits 1.
- ⚠ A development build answers `herdr 0.9.0` to `--version`.

⚠ **Read, not measured:** herdr's own release workflow at that commit builds Windows
for `x86_64` only, and its `windows-arm64.yml` tests the `x86_64` binary under
emulation; `packaging/windows/conpty.json` carries an `x86_64` bundle and no `aarch64`
one. A native Windows `aarch64` herdr is not something upstream makes.

⛔ **Read, and it decides the first task:** `LatestRelease` in
`tools/windows/wsl-toolkit/internal/toolkit/release.go` reads the newest 30 releases
and `Resolve-LatestTag` in `tools/windows/wsl-toolkit/consumer.ps1` lists 30. A nightly
published every day would push every `wsl-toolkit-v*` release out of both windows, and
every consumer's update check would answer that none was published.

## Approach

1. **The release lookups first.** `LatestRelease` reads every page it needs until a
   `wsl-toolkit-v*` release that is not a prerelease appears, with a bounded page
   count; `Resolve-LatestTag` excludes prereleases. A case over a release list whose
   first hundred entries are nightlies, and a mutation row per ceiling.
2. **`.github/workflows/herdr-build.yml`**, a reusable workflow that builds one herdr
   ref for Windows `x86_64` and `aarch64` and Linux `x86_64` and `aarch64` musl, with
   the Rust toolchain the ref's `rust-toolchain.toml` names and Zig checked against
   the digest ziglang.org publishes, and packages the Windows builds with a ConPTY
   bundle checked against the digests herdr pins. It publishes nothing.
3. **`.github/workflows/herdr-nightly.yml`**, daily and by dispatch: resolve herdr's
   development head, build it only when it moved and herdr's own CI passed on that
   commit, write `SHA256SUMS` and `BUILD-INFO.json`, sign every file keyless as
   `release.yml` does, publish a prerelease tagged `herdr-nightly-YYYYMMDD-SHA12`, and
   keep the newest seven.
4. **`release.yml`** builds herdr's newest stable tag through the same reusable
   workflow and publishes those four files in each `wsl-toolkit-v*` release, covered
   by its `SHA256SUMS` and signed with its identity.
5. **The herdr adapter follows a channel.** `{"name": "herdr", "channel": "nightly"}`
   resolves the newest nightly, and `install.sh` installs only a binary whose digest
   matches that release's `SHA256SUMS`. A channel with a `version` or `sha256` beside
   it is refused. On Windows the matching client is kept under the instance's state
   directory and `base attach` prints its full path.
6. **The documents**: `docs/AGENTS.md` section 1 and `TODO/RULES.md` name two published
   things, and `docs/consumers.md`, the manual and `examples/common/herdr.md` say what
   each channel installs.
7. **The pseudo-console probe becomes a tracked acceptance script** for the Windows
   client.

⛔ **No patch to herdr's source, no version stamp inside a binary, no macOS build, no
running server restarted by an ensure, and nothing sent to herdr's repository.**

## Decision

⭐ **Ruled by the operator on 2026-09-15:**

1. **Publish from ToolKit, as prereleases**, over a separate repository and over local
   builds only.
2. **Targets:** "win x64/arm64 + Linux x64/arm64, also let us publish the last stable
   herdr alongside our main stable release".
3. **Adoption:** "Follow the newest nightly", over a pin by version and digest. ⚠ The
   recommendation was the pin; the ruling is honoured by taking each digest from the
   nightly's own `SHA256SUMS`, which proves transport and not authorship, and the
   manual says so.
4. **Implemented in the session that authored it**, "Approve, implement now", which
   sets aside `docs/methodology/authoring.md`'s rule that the two are separate sessions.

## Consumers

- ⚠ **The release channel is a consumer contract.** A `herdr-nightly-*` tag is always
  a prerelease and never matches `wsl-toolkit-v*`; approach step 1 removes the window
  that would hide a `wsl-toolkit` release; the stable herdr files a `wsl-toolkit`
  release gains are additive, and `consumer.ps1` already verifies every file
  `SHA256SUMS` names.
- `scripts/common/bootstrap.sh` and every other fetched file: none.

## Prove

```powershell
go -C tools/windows/wsl-toolkit test -count=1 -run 'TestTheNewestReleaseIsFoundBehindAnyNumberOfNightlies' ./internal/toolkit/
gh workflow run herdr-nightly.yml --repo Azathothas/ToolKit
gh release view TAG --repo Azathothas/ToolKit --json isPrerelease,assets
wsl-toolkit --instance base base ensure
wsl-toolkit --instance base base attach
```

Passing is:

- the case passes, and goes red with either ceiling put back;
- a dispatched run publishes a prerelease carrying the four builds, `SHA256SUMS`,
  `BUILD-INFO.json` and a bundle for each, and a downloaded copy passes `sha256sum -c`
  and `cosign verify-blob` against `herdr-nightly.yml`'s identity;
- with `"channel": "nightly"`, `base ensure` exits 0 and installs a server whose digest
  is the nightly's; the client `base attach` prints answers `--machine base agent list`
  with exit 0;
- the pseudo-console probe passes its four signals with the nightly's Windows client;
- the gate is green and `check-record.sh` agrees.

## Amendment, 2026-09-15: steps 1 to 4 and 6 written, and step 1 proved

⭐ **Step 1 is proved.** `LatestRelease` reads a hundred releases a page until it finds
this tool's, stopping at a short page or after ten.
`TestTheNewestReleaseIsFoundBehindAnyNumberOfNightlies` puts 150 nightlies, a
prerelease and a draft of this tool in front of `wsl-toolkit-v3.1.0` and reads it on
the second page; `TestAListWithNoReleaseOfThisToolEndsAndSaysSo` ends a list of
nightlies after two pages with the refusal. **3 mutation rows, and `repo mutate --only
release:` 7 of 7 guards proved on Windows in 26.2 s.** `consumer.ps1` lists releases
with `--exclude-pre-releases`, which measured `wsl-toolkit-v2.0.2` first against this
repository on 2026-09-15. ⚠ `consumer.ps1` has no case harness, so its half has no
mutation row; the weekly release smoke drives it.

⛔ **Steps 2 to 4 are WRITTEN AND NOT RUN**: `herdr-build.yml`, `herdr-nightly.yml`,
and the stable herdr jobs in `release.yml`. They parse, and their actions are pinned
to commits resolved on 2026-09-15. Read in herdr's source while writing them:

- `app_local_conpty_path` in `vendor/portable-pty/src/win/psuedocon.rs` loads the
  bundled ConPTY only when the architecture is `x86_64`, with the bundle's three
  digests compiled in, so the `aarch64` zip carries `herdr.exe` and its licence;
- `vendor/libghostty-vt/build.zig.zon` asks for Zig 0.15.2 at `v0.9.0` and 0.16.0 on
  the development branch, and both are pinned by the digests ziglang.org's download
  index published on 2026-09-15.

Step 6 is written in `docs/AGENTS.md` section 1, `TODO/RULES.md`, `docs/consumers.md`
and the maintainers' README.

## Amendment, 2026-09-15: the build matrix's first run, and step 5 written

⭐ **`herdr-build.yml` dispatched by hand, twice, publishing nothing**: run
`34965359315` for the development head `052779c4159ed851` and run `34965362191` for
`v0.9.0`.

| ref, the Zig it asks for | Windows x86_64 | Windows aarch64 | Linux x86_64 | Linux aarch64 |
| --- | --- | --- | --- | --- |
| development head, 0.16.0 | ✅ | ❌ `zig build` exited `0xc0000005` | ✅ | ✅ |
| `v0.9.0`, 0.15.2 | ❌ Zig's build runner asserted `!std.fs.path.isAbsolute(child_cwd_rel)` in `Run.zig:662` | ✅ | ✅ | ✅ |

Both fixes are in `herdr-build.yml` and **not yet re-run**: a Windows job puts Zig's
global and local caches under the runner's temporary directory, on the checkout's
drive, where `windows-2022` otherwise leaves the global cache on `C:` beside a checkout
on `D:`; and Zig 0.16.0 on a Windows Arm host runs as its `x86_64` build under
emulation. ⚠ The first is a reading of the assertion, not yet a measurement.

⭐ **Step 5 is written and proved in the suite.** `{"name": "herdr", "channel":
"nightly"}`: the host half resolves the newest nightly and hands `install.sh` its tag,
the two Linux digests from its `SHA256SUMS` and its download base; `install.sh` accepts
only a GitHub release download base beside a version and records the release it
installed; `probe.sh` reports `release`, `sha256` and `server-binary-stale`; the host
half writes the matching Windows client under the instance's state directory only over
a matching digest and a zip whose entries resolve inside it, removes older ones, and
names a base whose machine holds none; `base attach` prints that client.

- 4 cases: `TestAChannelIsHerdrsAloneAndNeverBesideAPin`,
  `TestTheNightlyChannelInstallsWhatTheNewestNightlyPublished`,
  `TestTheWindowsClientIsWrittenOnlyOverAMatchingDigest` and
  `TestTheNightlyChannelReachesInstallThroughTheHostHalf`, each passing unmutated.
- **5 mutation rows, and the `attach` row moved to its new line: `repo mutate` 6 of 6
  guards proved on Windows.**
- ⚠ While writing the rows, the zip guard was found to be two checks for one
  condition, a `..` test and `ResolveInside`, so removing either left the case green;
  the `..` test is gone and `ResolveInside` is the containment.
- ⛔ **Not driven**: no nightly is published yet, so no base has followed the channel.

---
