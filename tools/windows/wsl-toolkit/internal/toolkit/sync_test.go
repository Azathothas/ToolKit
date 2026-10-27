// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/forksafe"
)

// -- WSL-125: base sync ---------------------------------------------------------

func TestASyncWritesADirectoryOfItsOwnUnderTheHome(t *testing.T) {
	home := "/home/toolkit"
	for _, c := range []struct {
		given string
		force bool
		want  string
		bad   string
	}{
		{"~/src/p", false, "/home/toolkit/src/p", ""},
		{"/home/toolkit/src/p/", false, "/home/toolkit/src/p", ""},
		{"/opt/p", true, "/opt/p", ""},
		{"/opt/p", false, "", "Pass --force"},
		{"~", true, "", "the account's home itself"},
		{"/home/toolkit", true, "", "the account's home itself"},
		{"/", true, "", "the guest's root"},
		{"~/.wsl-toolkit", true, "", "keeps its jobs and sessions"},
		{"~/.wsl-toolkit/sessions/x", true, "", "keeps its jobs and sessions"},
		// ⛔ A parent of the home holds it, and --delete would remove it. WSL-125.
		{"/home", true, "", "holds the account's home"},
		{"/home/", true, "", "holds the account's home"},
		{"/mnt", true, "/mnt", ""},
		{"~/src/../../etc", true, "", "climbs"},
		{"src/p", true, "", "not a guest path"},
		{`C:\src\x`, true, "", "not a guest path"},
		// A sibling whose name starts with the home's is outside it.
		{home + "x/p", false, "", "Pass --force"},
		{"~/a b", false, "", "outside the alphabet"},
	} {
		got, err := syncTarget(c.given, home, c.force)
		switch {
		case c.bad == "" && (err != nil || got != c.want):
			t.Errorf("%q, force %v: %q, %v; want %q", c.given, c.force, got, err, c.want)
		case c.bad != "" && (err == nil || !strings.Contains(err.Error(), c.bad)):
			t.Errorf("%q, force %v: %q, %v; want a refusal naming %q", c.given, c.force, got, err, c.bad)
		}
	}
}

func TestTheGuestsListIsReadLineByLine(t *testing.T) {
	out := "REAL|/home/user\nHOME|/home/user\nFS|ext4\nSTATE|dir\n" +
		"E|81a4|5|1700000000|./a.txt\n" +
		"E|41ed|4096|1700000001|./dir\n" +
		"E|81ed|3|1700000002|./dir/run|me.sh\n" +
		"E|a1ff|5|1700000003|./l\n" +
		"E|bad line\n" + "a name with\n" + "E|81a4|x|1|./broken\n" +
		"L|./l\nT|a|b.txt\n" +
		"END\n"
	s := parseSyncSurvey([]byte(out))
	if s.real != "/home/user" || s.home != "/home/user" || s.fs != "ext4" || s.state != "dir" || !s.ended {
		t.Fatalf("the head reads %+v", s)
	}
	if len(s.entries) != 4 {
		t.Fatalf("%d entries were read, want 4: %+v", len(s.entries), s.entries)
	}
	if a := s.entries["a.txt"]; !a.isReg() || a.size != 5 || a.mtime != 1700000000 {
		t.Errorf("a.txt reads %+v", a)
	}
	if d := s.entries["dir"]; !d.isDir() {
		t.Errorf("dir reads %+v", d)
	}
	if r := s.entries["dir/run|me.sh"]; !r.isReg() || r.mode&0o111 == 0 {
		t.Errorf("a name with a bar reads %+v", r)
	}
	if l := s.entries["l"]; !l.isLink() || l.link != "a|b.txt" {
		t.Errorf("the link reads %+v", l)
	}
	// ⛔ A Windows mount is read with its escapes undone, and found at or under
	// the directory only. WSL-125.
	w := parseSyncSurvey([]byte("REAL|/mnt\nW|/mnt/my\\040drive|drvfs\nW|/workspaces/p|9p\nSTATE|dir\n"))
	if len(w.windows) != 2 || w.windows[0] != "/mnt/my drive (drvfs)" || w.windowsUnder() != "/mnt/my drive (drvfs)" {
		t.Errorf("the Windows mounts read %q, and under /mnt %q", w.windows, w.windowsUnder())
	}
	for real, want := range map[string]string{"/": "/mnt/my drive (drvfs)", "/workspaces/p": "/workspaces/p (9p)", "/workspaces/pq": "", "/home/user": ""} {
		w.real = real
		if got := w.windowsUnder(); got != want {
			t.Errorf("under %s: %q, want %q", real, got, want)
		}
	}
}

// syncHostTree makes a host tree and returns its list as the workspace walk keeps
// it, keyed by path.
func syncHostTree(t *testing.T, files map[string]string) (string, map[string]workspaceEntry, []workspaceEntry) {
	t.Helper()
	root := t.TempDir()
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, "/") {
			if err := os.MkdirAll(p, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := forksafe.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	var up WorkspaceUpload
	var list []workspaceEntry
	if err := walkWorkspace(root, DefaultWorkspaceLimits(), WorkspaceFilter{}, &up, func(e workspaceEntry) error {
		list = append(list, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	by := map[string]workspaceEntry{}
	for _, e := range list {
		by[e.rel] = e
	}
	return root, by, list
}

// regAs is a guest regular file holding exactly what the host entry holds.
func regAs(e workspaceEntry) guestEntry {
	mode := uint32(0o100644)
	if e.mode&0o111 != 0 {
		mode = 0o100755
	}
	return guestEntry{mode: mode, size: e.info.Size(), mtime: hostMTime(e.info)}
}

func TestASyncSendsWhatChangedAndRemovesOnlyWhatDeleteMay(t *testing.T) {
	_, by, list := syncHostTree(t, map[string]string{
		"same.txt": "same", "grown.txt": "longer now", "touched.txt": "abcd", "new.txt": "n",
		"src/": "", "src/a.c": "a", "made/": "",
	})
	touched := regAs(by["touched.txt"])
	touched.mtime--
	grown := regAs(by["grown.txt"])
	grown.size = 3
	guest := map[string]guestEntry{
		"same.txt":    regAs(by["same.txt"]),
		"grown.txt":   grown,
		"touched.txt": touched,
		"src":         {mode: 0o040755},
		"src/a.c":     regAs(by["src/a.c"]),
		// Not in the list: a stray file, a build tree, and one under a gone dir.
		"stray.txt":      {mode: 0o100644, size: 1},
		"target":         {mode: 0o040755},
		"target/x.o":     {mode: 0o100644, size: 1},
		"old":            {mode: 0o040755},
		"old/f":          {mode: 0o100644, size: 1},
		"src/removed.c":  {mode: 0o100644, size: 1},
		"src/build.log":  {mode: 0o100644, size: 1},
		"src/sub":        {mode: 0o040755},
		"src/sub/deeper": {mode: 0o100644, size: 1},
	}
	keep := func(rel string, dir bool) bool { return rel == "target" || strings.HasSuffix(rel, ".log") }

	p := planSync(list, guest, true, keep)
	var sent []string
	for _, e := range p.send {
		sent = append(sent, e.rel)
	}
	sort.Strings(sent)
	if want := "grown.txt made new.txt touched.txt"; strings.Join(sent, " ") != want {
		t.Errorf("sent %q, want %q", strings.Join(sent, " "), want)
	}
	if p.unchanged != 3 || p.sentBytes != int64(len("longer now")+len("abcd")+len("n")) {
		t.Errorf("unchanged %d, sent bytes %d", p.unchanged, p.sentBytes)
	}
	sort.Strings(p.remove)
	if want := "old src/removed.c src/sub stray.txt"; strings.Join(p.remove, " ") != want {
		t.Errorf("removes %q, want %q", strings.Join(p.remove, " "), want)
	}
	if p.deleted != 6 || p.kept != 3 {
		t.Errorf("deleted %d and kept %d, want 6 and 3: a kept directory keeps what is under it", p.deleted, p.kept)
	}

	// ⛔ WITHOUT --delete NOTHING THE LIST LACKS IS TOUCHED.
	n := planSync(list, guest, false, keep)
	if len(n.remove) != 0 || n.deleted != 0 || n.kept != 9 {
		t.Errorf("without --delete: removes %q, deleted %d, kept %d", n.remove, n.deleted, n.kept)
	}
}

func TestAKindChangeIsReplacedOnlyWhereItMayBe(t *testing.T) {
	_, by, list := syncHostTree(t, map[string]string{"was-dir.txt": "f", "was-file/": "", "same/": ""})
	guest := map[string]guestEntry{
		"was-dir.txt":     {mode: 0o040755},
		"was-dir.txt/in":  {mode: 0o100644, size: 1},
		"was-file":        regAs(by["was-dir.txt"]),
		"same":            {mode: 0o040755},
		"same/extra-file": {mode: 0o100644, size: 1},
	}
	none := func(string, bool) bool { return false }
	n := planSync(list, guest, false, none)
	sort.Strings(n.conflicts)
	if strings.Join(n.conflicts, " ") != "was-dir.txt was-file" {
		t.Fatalf("without --delete the conflicts are %q", n.conflicts)
	}
	d := planSync(list, guest, true, none)
	sort.Strings(d.remove)
	if len(d.conflicts) != 0 || d.replaced != 2 || strings.Join(d.remove, " ") != "same/extra-file was-dir.txt was-file" || d.deleted != 2 {
		t.Fatalf("with --delete: conflicts %q, replaced %d, removes %q, deleted %d", d.conflicts, d.replaced, d.remove, d.deleted)
	}
}

// TestAFileTimeIsComparedAsTheArchiveCarriesIt: tar.Writer rounds a time to the
// nearest second, so the guest's copy of a file written at .7 of a second holds
// the next second, and comparing the host's own would send it every time.
func TestAFileTimeIsComparedAsTheArchiveCarriesIt(t *testing.T) {
	parallelCase(t)
	root, _, _ := syncHostTree(t, map[string]string{"f.txt": "x"})
	at := time.Date(2026, 1, 2, 3, 4, 5, 700_000_000, time.UTC)
	if err := os.Chtimes(filepath.Join(root, "f.txt"), at, at); err != nil {
		t.Fatal(err)
	}
	var up WorkspaceUpload
	var list []workspaceEntry
	if err := walkWorkspace(root, DefaultWorkspaceLimits(), WorkspaceFilter{}, &up, func(e workspaceEntry) error {
		list = append(list, e)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if list[0].info.ModTime().Nanosecond() == 0 {
		t.Skip("this filesystem keeps no fraction of a second, so the rounding cannot be seen")
	}
	held := guestEntry{mode: 0o100644, size: 1, mtime: at.Add(300 * time.Millisecond).Unix()}
	if p := planSync(list, map[string]guestEntry{"f.txt": held}, false, nil); p.unchanged != 1 || len(p.send) != 0 {
		t.Fatalf("a file the guest holds as the archive wrote it was sent again: %+v", p)
	}
}

func TestAFileTravelsWhenOnlyItsExecutableBitChanged(t *testing.T) {
	_, by, list := syncHostTree(t, map[string]string{"run.sh": "#!/bin/sh\n"})
	g := regAs(by["run.sh"])
	if by["run.sh"].mode&0o111 == 0 {
		t.Fatal("the shebang pass did not mark run.sh, so this case asserts nothing")
	}
	g.mode = 0o100644
	if p := planSync(list, map[string]guestEntry{"run.sh": g}, false, nil); len(p.send) != 1 {
		t.Fatalf("a file whose bit differs was not sent: %+v", p)
	}
}

// TestGitDecidesWhatDeleteKeeps asks real git, where this host has it, about
// paths the host does not hold, in the checkout and in a repository nested in it.
func TestGitDecidesWhatDeleteKeeps(t *testing.T) {
	parallelCase(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("no git on this host")
	}
	root := t.TempDir()
	run := func(dir string, args ...string) {
		cmd := exec.Command(git, append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run(root, "init", "-q")
	// ⚠ *.c IS THE CHECKOUT'S RULE AND NOT THE NESTED REPOSITORY'S, so a path in
	// the nested one asked of the checkout reads as ignored, and the case sees it.
	if err := forksafe.WriteFile(filepath.Join(root, ".gitignore"), []byte("target/\n*.log\n*.c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "vendor", "lib")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	run(nested, "init", "-q")
	if err := forksafe.WriteFile(filepath.Join(nested, ".gitignore"), []byte("build/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	kept, err := gitKeeps(context.Background(), root, []string{"vendor/lib"}, map[string]bool{
		"target": true, "debug.log": false, "src": true, "vendor/lib/build": true, "vendor/lib/x.c": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for p := range kept {
		got = append(got, p)
	}
	sort.Strings(got)
	if strings.Join(got, " ") != "debug.log target vendor/lib/build" {
		t.Fatalf("git keeps %q", got)
	}
}

// TestASyncRefusesAWindowsDirectoryAndChangesNothing drives SyncBase through the
// fake wsl.exe: a guest directory on 9P is refused, and no step after the survey
// runs.
func TestASyncRefusesAWindowsDirectoryAndChangesNothing(t *testing.T) {
	g := newExecFakeGuest(t)
	survey := filepath.Join(t.TempDir(), "survey")
	t.Setenv(execFakeSurveyEnv, survey)
	host, _, _ := syncHostTree(t, map[string]string{"a.txt": "a"})
	for _, c := range []struct{ answer, want string }{
		{"REAL|/workspaces/p\nHOME|/home/user\nFS|9p\nSTATE|dir\n", "on a Windows directory (9p"},
		{"REAL|/opt\nHOME|/home/user\nFS|ext4\nSTATE|missing\n", "leads through a link to /opt"},
		{"REAL|/home/user\nHOME|/home/user\nFS|ext4\nSTATE|link\n", "is a link"},
		{"REAL|/home/user/p\nHOME|/home/user\nFS|ext4\nSTATE|dir\nE|81a4|1|1|./a.txt\n", "stopped before its end"},
	} {
		if err := forksafe.WriteFile(survey, []byte(c.answer), 0o600); err != nil {
			t.Fatal(err)
		}
		before := len(g.callsOrNone(t))
		_, err := SyncBase(context.Background(), g.wsl, SyncSpec{Distro: "wsl-toolkit", User: "toolkit", HostDir: host, GuestDir: "~/p"})
		if err == nil || !strings.Contains(err.Error(), c.want) || errors.Is(err, ErrSyncIncomplete) {
			t.Errorf("%q answered %v", c.answer, err)
		}
		if after := g.callsOrNone(t); len(after)-before != 2 {
			t.Errorf("%q: %d guest calls, want the home and the survey only", c.answer, len(after)-before)
		}
	}
}

// TestAForcedDeleteNeverReachesTheHomeOrADrive is the defect: `--force --delete`
// into /home removed the account's home, and into /mnt the Windows files a drive
// carried, because only the directory's own filesystem was read. WSL-125.
func TestAForcedDeleteNeverReachesTheHomeOrADrive(t *testing.T) {
	g := newExecFakeGuest(t)
	survey := filepath.Join(t.TempDir(), "survey")
	t.Setenv(execFakeSurveyEnv, survey)
	host, _, _ := syncHostTree(t, map[string]string{"a.txt": "a"})
	for _, c := range []struct{ guest, answer, want string }{
		{"/home", "", "holds the account's home /home/user"},
		{"/mnt", "REAL|/mnt\nHOME|/home/user\nFS|ext4\nW|/mnt/c|drvfs\nSTATE|dir\nE|41ed|0|1|./c\nEND\n", "holds a Windows directory at /mnt/c (drvfs)"},
		{"/workspaces", "REAL|/workspaces\nHOME|/home/user\nFS|ext4\nW|/workspaces/p|9p\nSTATE|dir\nE|41ed|0|1|./p\nEND\n", "holds a Windows directory at /workspaces/p (9p)"},
		{"/srv/up", "REAL|/home\nHOME|/home/user\nFS|ext4\nSTATE|dir\nE|41ed|0|1|./user\nEND\n", "which is or holds the account's home /home/user"},
	} {
		if err := forksafe.WriteFile(survey, []byte(c.answer), 0o600); err != nil {
			t.Fatal(err)
		}
		before := len(g.callsOrNone(t))
		_, err := SyncBase(context.Background(), g.wsl, SyncSpec{Distro: "wsl-toolkit", User: "toolkit", HostDir: host, GuestDir: c.guest, Force: true, Delete: true})
		if err == nil || !strings.Contains(err.Error(), c.want) || errors.Is(err, ErrSyncIncomplete) {
			t.Errorf("%s answered %v, want a refusal naming %q", c.guest, err, c.want)
		}
		for _, call := range g.callsOrNone(t)[before:] {
			if strings.Contains(call.Stdin, "rm -rf") || len(call.Files) > 0 {
				t.Errorf("%s: a refused sync still wrote the guest: %+v", c.guest, call)
			}
		}
	}
}

// TestASyncSendsTheChangeAndRemovesTheStray drives the whole sync through the
// fake wsl.exe.
func TestASyncSendsTheChangeAndRemovesTheStray(t *testing.T) {
	g := newExecFakeGuest(t)
	host, by, _ := syncHostTree(t, map[string]string{"same.txt": "same", "new.txt": "n"})
	same := regAs(by["same.txt"])
	survey := filepath.Join(t.TempDir(), "survey")
	answer := "REAL|/home/user/p\nHOME|/home/user\nFS|ext4\nSTATE|dir\n" +
		fmt.Sprintf("E|%x|%d|%d|./same.txt\n", same.mode, same.size, same.mtime) +
		"E|81a4|1|1|./stray.txt\nEND\n"
	if err := forksafe.WriteFile(survey, []byte(answer), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(execFakeSurveyEnv, survey)
	rep, err := SyncBase(context.Background(), g.wsl, SyncSpec{Distro: "wsl-toolkit", User: "toolkit", HostDir: host, GuestDir: "~/p", Delete: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.GuestDir != "/home/user/p" || rep.Entries != 2 || rep.Sent != 1 || rep.Unchanged != 1 || rep.Deleted != 1 || rep.SentBytes != 1 {
		t.Fatalf("the report is %+v", rep)
	}
	var removed, sent bool
	for _, c := range g.calls(t) {
		if strings.Contains(c.Stdin, "rm -rf -- './stray.txt'") && !strings.Contains(c.Stdin, "same.txt") {
			removed = true
		}
		if len(c.Files) > 0 {
			_, n := c.Files["new.txt"]
			_, s := c.Files["same.txt"]
			sent = n && !s
		}
	}
	if !removed || !sent {
		t.Fatalf("the stray was removed %v, and the archive held the change alone %v", removed, sent)
	}
}

// callsOrNone is calls, and none where the fake has not been called yet.
func (g *execFakeGuest) callsOrNone(t *testing.T) []execFakeCall {
	t.Helper()
	if _, err := os.Stat(g.log); err != nil {
		return nil
	}
	return g.calls(t)
}

// TestTheGuestSurveyRunsInARealShell runs the survey where this host has a POSIX
// shell, find and stat, over a tree with a file, a directory and a link.
func TestTheGuestSurveyRunsInARealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the survey is POSIX shell, and this host has none to run it in")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	home := t.TempDir()
	d := filepath.Join(home, "p")
	if err := os.MkdirAll(filepath.Join(d, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := forksafe.WriteFile(filepath.Join(d, "sub", "a.txt"), []byte("abc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub/a.txt", filepath.Join(d, "l")); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh)
	cmd.Stdin = bytes.NewReader(syncSurveyScript(d))
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("the survey failed: %v: %s", err, out)
	}
	s := parseSyncSurvey(out)
	if s.state != "dir" || !s.ended || s.home == "" || s.real == "" {
		t.Fatalf("the survey read %+v from:\n%s", s, out)
	}
	if a := s.entries["sub/a.txt"]; !a.isReg() || a.size != 3 || a.mode&0o111 == 0 {
		t.Errorf("sub/a.txt reads %+v", a)
	}
	if l := s.entries["l"]; !l.isLink() || l.link != "sub/a.txt" {
		t.Errorf("the link reads %+v", l)
	}
	if !s.entries["sub"].isDir() || len(s.entries) != 3 {
		t.Errorf("the entries read %+v", s.entries)
	}
	missing := exec.Command(sh)
	missing.Stdin = bytes.NewReader(syncSurveyScript(filepath.Join(home, "none", "deeper")))
	missing.Env = cmd.Env
	out, err = missing.Output()
	if err != nil {
		t.Fatal(err)
	}
	if m := parseSyncSurvey(out); m.state != "missing" || m.real == "" {
		t.Errorf("a missing directory read %+v", m)
	}
}
