package toolkit

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// doorsSample is what a healthy run of doors.sh looks like: every required door
// with the verdict given, plus the count line the script writes itself.
func doorsSample(verdicts map[string]string) string {
	var b strings.Builder
	n := 0
	for _, id := range doorsRequired {
		v, ok := verdicts[id]
		if !ok {
			v = DoorClosed
		}
		fmt.Fprintf(&b, "DOOR|%s|%s|measured\n", id, v)
		n++
	}
	fmt.Fprintf(&b, "DOORS-COMPLETE|%d\n", n)
	return b.String()
}

func TestTheDoorsProbeIsReadInFull(t *testing.T) {
	doors, err := parseDoors(doorsSample(nil))
	if err != nil {
		t.Fatalf("parseDoors() = %v, want a clean read", err)
	}
	if len(doors) != len(doorsRequired) {
		t.Fatalf("parseDoors() read %d doors, want %d", len(doors), len(doorsRequired))
	}
}

// ⛔ THE GUARD THE WHOLE COMMAND RESTS ON. The script counts what it emitted and
// this counts what it understood; a parse that stopped matching reads zero rows
// and, without this, the command renders an empty table and exits 0 over a base
// it never measured. That is the shape the gate's `powershell` check had for its
// whole life, and it is why this row exists.
func TestADoorsAnswerThatDisagreesWithItsOwnCountIsRefused(t *testing.T) {
	sample := doorsSample(nil)
	// The script says it wrote one more row than it did, which is what a reader
	// that silently dropped a row looks like from here.
	bumped := strings.Replace(sample,
		fmt.Sprintf("DOORS-COMPLETE|%d", len(doorsRequired)),
		fmt.Sprintf("DOORS-COMPLETE|%d", len(doorsRequired)+1), 1)
	if _, err := parseDoors(bumped); err == nil || !strings.Contains(err.Error(), "no longer agree") {
		t.Fatalf("parseDoors() over a disagreeing count = %v, want a refusal naming the disagreement", err)
	}
	// And the case that matters most: nothing parsed at all.
	if _, err := parseDoors("DOORS-COMPLETE|14\n"); err == nil || !strings.Contains(err.Error(), "no longer agree") {
		t.Fatalf("parseDoors() over zero understood rows = %v, want a refusal", err)
	}
}

// ⛔ A PROBE THAT STOPPED EARLY IS NOT A PROBE THAT FOUND EVERY DOOR SHUT. Its
// last line is what says it got to the end.
func TestADoorsProbeThatNeverReachedItsLastLineIsRefused(t *testing.T) {
	sample := doorsSample(nil)
	cut := sample[:strings.Index(sample, "DOORS-COMPLETE|")]
	_, err := parseDoors(cut)
	if err == nil || !strings.Contains(err.Error(), "unknown rather than closed") {
		t.Fatalf("parseDoors() over a truncated answer = %v, want a refusal saying the rest is unknown", err)
	}
}

// ⛔ A MISSING DOOR IS A REFUSAL, NOT A BLANK ROW. A reader who sees no row for a
// door concludes the door is not there.
func TestADoorsAnswerMissingARequiredDoorIsRefused(t *testing.T) {
	for _, drop := range []string{"fs.mnt-wsl-shared", "priv.passwordless-sudo", "net.internet"} {
		sample := doorsSample(nil)
		var kept []string
		n := 0
		for _, line := range strings.Split(strings.TrimRight(sample, "\n"), "\n") {
			if strings.HasPrefix(line, "DOOR|"+drop+"|") {
				continue
			}
			if strings.HasPrefix(line, "DOOR|") {
				n++
			}
			kept = append(kept, line)
		}
		// The count is corrected too, so the ONLY thing wrong is the absence.
		trimmed := strings.Join(kept[:len(kept)-1], "\n") + fmt.Sprintf("\nDOORS-COMPLETE|%d\n", n)
		_, err := parseDoors(trimmed)
		if err == nil || !strings.Contains(err.Error(), drop) {
			t.Fatalf("parseDoors() without %s = %v, want a refusal naming it", drop, err)
		}
	}
}

func TestADoorReportedTwiceIsRefused(t *testing.T) {
	sample := doorsSample(nil) + "DOOR|net.internet|closed|again\n"
	sample = strings.Replace(sample,
		fmt.Sprintf("DOORS-COMPLETE|%d", len(doorsRequired)),
		fmt.Sprintf("DOORS-COMPLETE|%d", len(doorsRequired)+1), 1)
	if _, err := parseDoors(sample); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("parseDoors() over a repeated door = %v, want a refusal", err)
	}
}

// sealedClaims is the configuration a zero-grant base carries: every door this
// tool can close, claimed.
func sealedClaims(t *testing.T) []doorClaim {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Base.Automount = AutomountOff
	cfg.Base.Interop = BaseInteropOff
	cfg.Base.PasswordlessSudo = false
	claims, err := doorClaims(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 3 {
		t.Fatalf("a zero-grant configuration claims %d doors, want 3", len(claims))
	}
	return claims
}

func TestAClaimedDoorThatIsOpenIsAProblem(t *testing.T) {
	doors, err := parseDoors(doorsSample(map[string]string{"interop.windows-path": DoorOpen}))
	if err != nil {
		t.Fatal(err)
	}
	_, problems, open := judgeDoors(doors, sealedClaims(t))
	if len(problems) != 1 || !strings.Contains(problems[0], "interop.windows-path") {
		t.Fatalf("judgeDoors() problems = %v, want one naming interop.windows-path", problems)
	}
	if len(open) != 1 || open[0] != "interop.windows-path" {
		t.Fatalf("judgeDoors() open = %v, want interop.windows-path", open)
	}
}

// ⛔ THE ONE THAT IS EASIEST TO GET WRONG. `unknown` means nothing was tried:
// no packet was sent, no mount was attempted, no refusal was seen. A base whose
// configuration promises the door is shut has NOT kept that promise just because
// the guest carried no way to test it.
func TestAClaimedDoorThatCouldNotBeTriedIsAProblem(t *testing.T) {
	doors, err := parseDoors(doorsSample(map[string]string{"priv.passwordless-sudo": DoorUnknown}))
	if err != nil {
		t.Fatal(err)
	}
	_, problems, _ := judgeDoors(doors, sealedClaims(t))
	if len(problems) != 1 {
		t.Fatalf("judgeDoors() problems = %v, want exactly one", problems)
	}
	if !strings.Contains(problems[0], "Unknown is not closed") {
		t.Fatalf("judgeDoors() problem = %q, want it to say unknown is not closed", problems[0])
	}
}

// ⭐ THE NEGATIVE, AND IT IS THE HALF THAT KEEPS THE EXIT CODE MEANING SOMETHING.
// The internet, the Windows host and the shared tmpfs are open on every base this
// tool has ever built, and no setting here closes them. If those counted, the
// command would refuse every correct base and its caller would learn to ignore it.
func TestAnOpenDoorNoSettingClosesIsReportedAndIsNotAProblem(t *testing.T) {
	doors, err := parseDoors(doorsSample(map[string]string{
		"net.internet":               DoorOpen,
		"net.windows-host-icmp":      DoorOpen,
		"fs.mnt-wsl-shared":          DoorOpen,
		"priv.unshare-user-plus-net": DoorOpen,
	}))
	if err != nil {
		t.Fatal(err)
	}
	judged, problems, open := judgeDoors(doors, sealedClaims(t))
	if len(problems) != 0 {
		t.Fatalf("judgeDoors() problems = %v, want none: no setting in this tool closes those doors", problems)
	}
	if len(open) != 4 {
		t.Fatalf("judgeDoors() open = %v, want all four reported", open)
	}
	rep := DoorsReport{Doors: judged, Problems: problems, Open: open}
	if !rep.Sealed() {
		t.Fatal("a base that keeps every promise it makes reads as not sealed")
	}
}

// ⛔ THE ONE THE MEASUREMENT FORCED. On 2026-09-17 `wsl-toolkit-base`, whose
// /etc/wsl.conf carries `[interop] enabled=false`, was found with the WSLInterop
// binfmt handler REGISTERED and answering, while `wsl-toolkit`, configured
// `enabled=true`, had no handler at all and could not execute a Windows binary.
// WSL registers it whatever this tool writes, so `base.interop = "off"` does not
// claim that door: a command that refuses every correctly built base is one its
// caller learns to ignore. It is reported, and it is not a problem.
func TestAPEHandlerLeftRegisteredByWSLIsReportedAndNotClaimed(t *testing.T) {
	doors, err := parseDoors(doorsSample(map[string]string{"interop.exec-pe": DoorOpen}))
	if err != nil {
		t.Fatal(err)
	}
	judged, problems, open := judgeDoors(doors, sealedClaims(t))
	if len(problems) != 0 {
		t.Fatalf("judgeDoors() problems = %v, want none: this tool cannot remove the handler WSL registers", problems)
	}
	if len(open) != 1 || open[0] != "interop.exec-pe" {
		t.Fatalf("judgeDoors() open = %v, want interop.exec-pe reported", open)
	}
	for _, d := range judged {
		if d.ID == "interop.exec-pe" && d.Claimed {
			t.Fatal("interop.exec-pe is claimed, which promises something WSL does not let this tool deliver")
		}
	}
}

// ⛔ A base that claims nothing cannot fail, and that is a fact about the
// configuration rather than about the doors. An ordinary base with interop on
// makes three fewer promises, and the same open interop is not a problem.
func TestABaseThatClaimsNothingDoesNotFailOnTheSameDoors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Base.PasswordlessSudo = true
	claims, err := doorClaims(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 0 {
		t.Fatalf("a default configuration claims %v, want nothing", claims)
	}
	doors, err := parseDoors(doorsSample(map[string]string{"interop.windows-path": DoorOpen, "fs.windows-drives": DoorOpen}))
	if err != nil {
		t.Fatal(err)
	}
	if _, problems, _ := judgeDoors(doors, claims); len(problems) != 0 {
		t.Fatalf("judgeDoors() problems = %v, want none when nothing was claimed", problems)
	}
}

// ⛔ THE SCRIPT AND THIS READER ARE TWO FILES AND ONE CONTRACT. A door renamed in
// one of them would otherwise be found by a live run on a real base, on a host
// that has one, and by nothing else.
func TestEveryRequiredDoorIsEmittedByTheEmbeddedProbe(t *testing.T) {
	script := string(doorsScript)
	for _, id := range doorsRequired {
		// ⛔ NOT a bare substring search. Every required id appears in a comment
		// somewhere too, so "the name is in the file" would pass over a door whose
		// emitting call had been deleted. What is asserted is the CALL: `say <id>`
		// or `try_tcp <id>`, which are the only two things that write a row.
		if !strings.Contains(script, "say "+id+" ") && !strings.Contains(script, "try_tcp "+id+" ") {
			t.Errorf("doors.sh has no say or try_tcp call for %s, which this reader requires", id)
		}
	}
	if !strings.Contains(script, "DOORS-COMPLETE|%d") {
		t.Error("doors.sh no longer writes the count line the reader compares against")
	}
}

// ⚠ The script's own refusal of a bad marker is a shell-level guard, and this
// asserts only that it is still there: the case that RUNS it needs a guest.
func TestTheDoorsProbeRefusesAMarkerThatIsNotHexadecimal(t *testing.T) {
	if !strings.Contains(string(doorsScript), "TK_DOORS_MARK is not hexadecimal") {
		t.Error("doors.sh no longer refuses a marker that could become a path")
	}
}

// ⛔ THE ANSWER A CALLER MOST WANTS IS THE ONE THAT BROKE THEM. A base with no
// problems marshalled `"problems": null`, because Go writes a nil slice as null,
// so `.problems.length` threw on exactly the healthy case while every failing
// base parsed. Found by reading this command's own --json output on 2026-09-17.
func TestAnEmptyDoorsListMarshalsAsAnArrayAndNotNull(t *testing.T) {
	blob, err := json.Marshal(DoorsReport{Schema: DoorsSchema}.withEmptyLists())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"doors":[]`, `"problems":[]`, `"open":[]`} {
		if !strings.Contains(string(blob), want) {
			t.Errorf("the answer does not carry %s: %s", want, blob)
		}
	}
	if strings.Contains(string(blob), "null") {
		t.Errorf("the answer carries a null a consumer would call .length on: %s", blob)
	}
}
