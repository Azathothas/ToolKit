// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed doors.sh
var doorsScript []byte

// DoorsSchema versions the answer `base doors` writes.
const DoorsSchema = "wsl-toolkit-base-doors/1"

// What one attempt on a door came back as.
//
// ⛔ DoorUnknown IS NOT DoorClosed. Nothing was tried, so nothing was refused,
// and a caller that folds the two together publishes a containment claim over a
// door it never touched. Every reader below keeps them apart.
const (
	DoorOpen     = "open"
	DoorClosed   = "closed"
	DoorReadOnly = "readonly"
	DoorAbsent   = "absent"
	DoorUnknown  = "unknown"
	DoorInfo     = "info"
)

// Door is one door, attacked.
type Door struct {
	ID      string `json:"id"`
	Verdict string `json:"verdict"`
	Detail  string `json:"detail"`
	// Claimed is set when this base's own configuration says this door is shut.
	// A claimed door that is not closed is a problem; an unclaimed one that is
	// open is reported and is not.
	Claimed bool `json:"claimed"`
	// ClaimedBy names the setting that claims it, for the reader that has to
	// decide what to change.
	ClaimedBy string `json:"claimed_by,omitempty"`
}

// DoorsReport is what `base doors` answers.
type DoorsReport struct {
	Schema  string `json:"schema"`
	Distro  string `json:"distro"`
	Account string `json:"account"`
	// HostAddress is what the guest was told to try. Empty means this host could
	// not be resolved, and every host door then reports unknown.
	HostAddress string `json:"host_address"`
	Doors       []Door `json:"doors"`
	// Problems are the doors the configuration claims to close and that were not
	// closed. This is the only thing that decides the exit code.
	Problems []string `json:"problems"`
	// Open is every door that got through, claimed or not. ⭐ It is the honest
	// half of the answer: a base with no problems is a base that keeps its own
	// promises, NOT a base with no way out.
	Open []string `json:"open"`
}

// Sealed answers whether every door the configuration claims is in fact shut.
//
// ⛔ IT IS NOT "CONTAINED". Read Open beside it, and docs say so in the manual's
// safety model: this tool has never measured a WSL distribution to be a security
// boundary and does not claim one here.
func (r DoorsReport) Sealed() bool { return len(r.Problems) == 0 }

// doorsRequired is every door a run must come back with. ⛔ A report missing one
// is refused rather than rendered, because a table that silently lost a row is a
// table whose next reader concludes the door is not there.
//
// ⚠ Info rows are deliberately NOT in this list. They carry no verdict, so
// losing one costs a reading and not a conclusion; these are the ones a claim
// can rest on.
var doorsRequired = []string{
	"fs.windows-drives",
	"fs.mount-drvfs",
	"fs.wsl-drivers",
	"fs.mnt-wsl-shared",
	"interop.exec-pe",
	"interop.run-exe",
	"interop.windows-path",
	"net.windows-host-smb",
	"net.windows-host-rdp",
	"net.windows-host-icmp",
	"net.internet",
	"priv.passwordless-sudo",
	"priv.unshare-netns",
	"priv.unshare-userns",
	"priv.unshare-user-plus-net",
}

// doorClaim is a door this tool's own configuration says is shut, and the
// setting that says it.
type doorClaim struct {
	door string
	by   string
}

// doorClaims reads the claims out of a configuration.
//
// ⭐ THE LIST IS SHORT ON PURPOSE. It holds exactly the doors a setting in this
// tool closes, and nothing that merely happens to be shut on this host. A claim
// added here is a promise the tool has to keep on every base it builds.
func doorClaims(cfg Config) ([]doorClaim, error) {
	automount, err := NormalizeAutomount(cfg.Base.Automount)
	if err != nil {
		return nil, err
	}
	interop, err := NormalizeBaseInterop(cfg.Base.Interop)
	if err != nil {
		return nil, err
	}
	var claims []doorClaim
	if automount == AutomountOff {
		claims = append(claims, doorClaim{"fs.windows-drives", "base.automount = off"})
	}
	if interop == BaseInteropOff {
		// ⛔ ONE INTEROP DOOR IS CLAIMED, AND IT IS NOT THE OBVIOUS ONE. Measured
		// on 2026-09-17: WSL registers the WSLInterop binfmt handler on a base
		// whose /etc/wsl.conf says `[interop] enabled=false`, and `interop.exec-pe`
		// is open there. This tool cannot remove that handler, so promising it
		// would be a promise WSL does not let it keep, and a command that refuses
		// every correctly built base is a command its caller learns to ignore.
		// `interop.run-exe` is not claimed either: a base with no Windows path
		// reachable has no executable to try, and `unknown` would be the answer
		// forever. What IS claimed is appendWindowsPath, which the provisioner
		// writes and the probe can decide on every base.
		claims = append(claims, doorClaim{"interop.windows-path", `base.interop = "off"`})
	}
	if !cfg.Base.PasswordlessSudo {
		claims = append(claims, doorClaim{"priv.passwordless-sudo", "base.passwordless_sudo = false"})
	}
	return claims, nil
}

// parseDoors reads what doors.sh wrote.
//
// ⛔ THE COUNT IS COMPARED, AND IT IS THE GUARD THAT MATTERS. The script counts
// the rows it emitted; this counts the rows it understood. A parse that stops
// matching the script's format reads zero rows, and without this comparison the
// command would render an empty table and exit 0 over a base it never measured -
// which is exactly what the gate's `powershell` check did for its whole life
// before 2026-09-16.
func parseDoors(out string) ([]Door, error) {
	var doors []Door
	claimed := -1
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(line, "DOOR|"):
			parts := strings.SplitN(strings.TrimPrefix(line, "DOOR|"), "|", 3)
			if len(parts) != 3 {
				return nil, fmt.Errorf("a door row had %d fields rather than 3: %q", len(parts), line)
			}
			id, verdict := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
			if id == "" || verdict == "" {
				return nil, fmt.Errorf("a door row named nothing: %q", line)
			}
			if seen[id] {
				return nil, fmt.Errorf("the door %s was reported twice", id)
			}
			seen[id] = true
			doors = append(doors, Door{ID: id, Verdict: verdict, Detail: strings.TrimSpace(parts[2])})
		case strings.HasPrefix(line, "DOORS-COMPLETE|"):
			n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "DOORS-COMPLETE|")))
			if err != nil {
				return nil, fmt.Errorf("the last line did not carry a number: %q", line)
			}
			claimed = n
		}
	}
	if claimed < 0 {
		return nil, errors.New("the probe exited without reaching its last line, so the doors it did not reach are unknown rather than closed")
	}
	if claimed != len(doors) {
		return nil, fmt.Errorf("the probe says it reported %d doors and %d were understood here, so this reader and that script no longer agree", claimed, len(doors))
	}
	var missing []string
	for _, want := range doorsRequired {
		if !seen[want] {
			missing = append(missing, want)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("the probe reported no verdict for %s", strings.Join(missing, ", "))
	}
	return doors, nil
}

// judgeDoors marks each door against the configuration's claims and collects
// what disagrees.
//
// ⛔ A CLAIMED DOOR PASSES ON `closed` AND ON NOTHING ELSE. `unknown` is a door
// nothing could try and `info` carries no verdict at all; both are problems when
// a setting has promised the door is shut, because the promise is unproven.
// `readonly` and `absent` are the two honest ways a filesystem door is not a way
// out, and they pass.
func judgeDoors(doors []Door, claims []doorClaim) ([]Door, []string, []string) {
	byID := map[string]doorClaim{}
	for _, c := range claims {
		byID[c.door] = c
	}
	var problems, open []string
	for i := range doors {
		d := &doors[i]
		if c, ok := byID[d.ID]; ok {
			d.Claimed, d.ClaimedBy = true, c.by
		}
		if d.Verdict == DoorOpen {
			open = append(open, d.ID)
		}
		if !d.Claimed {
			continue
		}
		switch d.Verdict {
		case DoorClosed, DoorReadOnly, DoorAbsent:
		case DoorUnknown:
			problems = append(problems, fmt.Sprintf("%s claims %s is shut, and the probe could not try it: %s. Unknown is not closed", d.ClaimedBy, d.ID, d.Detail))
		default:
			problems = append(problems, fmt.Sprintf("%s claims %s is shut and it is %s: %s", d.ClaimedBy, d.ID, d.Verdict, d.Detail))
		}
	}
	// ⛔ A CLAIM WITH NO ROW IS A PROBLEM, not a silence. parseDoors refuses a
	// missing required door, so this can only fire for a claim on a door outside
	// that list - which is a bug in this file and says so.
	ids := map[string]bool{}
	for _, d := range doors {
		ids[d.ID] = true
	}
	for _, c := range claims {
		if !ids[c.door] {
			problems = append(problems, fmt.Sprintf("%s claims %s is shut and the probe reported no such door at all", c.by, c.door))
		}
	}
	sort.Strings(problems)
	return doors, problems, open
}

// doorsMark is the per-run name every file this probe writes carries, so two
// runs cannot mistake one another's evidence for their own.
func doorsMark() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// ⚠ NOT a reason to refuse to probe, and not a constant either: a fixed
		// name would collide with a concurrent run. The clock is the fallback.
		return fmt.Sprintf("%012x", time.Now().UnixNano()&0xffffffffffff)
	}
	return hex.EncodeToString(raw[:])
}

// Doors runs the probe in the base, as the unprivileged account, and reports
// what got through.
//
// ⚠ IT WRITES INTO THE GUEST AND CLEANS UP AFTER ITSELF. The attempts leave a
// directory under /tmp and, where the door is open, one file under /mnt/wsl and
// one under /usr/lib/wsl/drivers; each is removed by the line that made it, and
// the /mnt/wsl row reports whether its own file went away.
func (b *Base) Doors(ctx context.Context) (DoorsReport, error) {
	rep := DoorsReport{Schema: DoorsSchema, Distro: b.cfg.Base.Name, Account: b.cfg.Base.User}
	claims, err := doorClaims(b.cfg)
	if err != nil {
		return rep, err
	}
	// ⚠ A HOST ADDRESS THAT CANNOT BE RESOLVED IS PASSED AS EMPTY, not guessed.
	// The guest then reports every host door `unknown`, which is the true answer:
	// nothing was tried.
	if host, err := ResolveHostAddress(); err == nil {
		rep.HostAddress = host.Address
	} else {
		b.log("the address this host answers at could not be resolved, so the Windows host doors report unknown: " + err.Error())
	}
	out, stderr, code, err := b.captureAs(ctx, b.cfg.Base.User, doorsScript, map[string]string{
		"TK_USER":       b.cfg.Base.User,
		"TK_HOSTADDR":   rep.HostAddress,
		"TK_DOORS_MARK": doorsMark(),
	}, 10*time.Minute)
	if err != nil {
		return rep, fmt.Errorf("the doors probe could not run in %s: %w", b.cfg.Base.Name, err)
	}
	if code != 0 {
		return rep, fmt.Errorf("the doors probe exited %d in %s: %s", code, b.cfg.Base.Name, firstLine(stderr+out))
	}
	doors, err := parseDoors(out)
	if err != nil {
		return rep, fmt.Errorf("the doors probe ran in %s and its answer could not be read: %w", b.cfg.Base.Name, err)
	}
	rep.Doors, rep.Problems, rep.Open = judgeDoors(doors, claims)
	return rep.withEmptyLists(), nil
}

// withEmptyLists makes every list in the answer an empty array rather than null.
//
// ⛔ A CONSUMER READS `.problems.length`, AND NULL IS NOT A LIST. Go marshals a
// nil slice as `null`, so the ONE answer a caller most wants - a base with no
// problems - is the one that breaks them, while every failing base parses. It is
// the convention `base grant` already keeps, and it was caught here by reading
// this command's own JSON rather than by any check.
func (r DoorsReport) withEmptyLists() DoorsReport {
	if r.Doors == nil {
		r.Doors = []Door{}
	}
	if r.Problems == nil {
		r.Problems = []string{}
	}
	if r.Open == nil {
		r.Open = []string{}
	}
	return r
}
