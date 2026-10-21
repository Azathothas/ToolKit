// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"os"
	"path/filepath"
	"strings"
)

// StateCopy is a package's own copy of this tool's state directory. WSL-118.
//
// ⛔ WINDOWS KEEPS ONE PER PACKAGE, AND A PROCESS IT REDIRECTS SEES ONLY ITS
// OWN. A process started from inside the Claude desktop app wrote
// `%LOCALAPPDATA%\wsl-toolkit` into
// `%LOCALAPPDATA%\Packages\Claude_pzs8sxrjxfjjc\LocalCache\Local\wsl-toolkit`,
// and a terminal outside it saw none of it: not the jobs, not the ledger, not
// the --exclusive locks, not the configuration a `base grant` wrote. Two such
// callers are two machines to this tool, and neither can say so unless it looks.
type StateCopy struct {
	Package string `json:"package"`
	Path    string `json:"path"`
	Entries int    `json:"entries"`
	// ThisProcess is true where this process's own view of home IS that copy:
	// what it writes lands there, and a process outside it does not see it.
	ThisProcess bool `json:"this_process"`
}

// StateCopies finds every package that keeps its own copy of home.
//
// ⭐ IT WRITES NOTHING, so `doctor` can call it and still create no file. Which
// view this process has is read by file identity: through a redirected view,
// home's entries ARE the copy's, the same files, and through a plain one they
// are other files or absent.
func StateCopies(home string) []StateCopy {
	local := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if local == "" || home == "" {
		return nil
	}
	rel, err := filepath.Rel(local, home)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	packages, err := os.ReadDir(filepath.Join(local, "Packages"))
	if err != nil {
		return nil
	}
	var out []StateCopy
	for _, p := range packages {
		if !p.IsDir() {
			continue
		}
		path := filepath.Join(local, "Packages", p.Name(), "LocalCache", "Local", rel)
		entries, err := os.ReadDir(path)
		if err != nil || len(entries) == 0 {
			continue
		}
		c := StateCopy{Package: p.Name(), Path: path, Entries: len(entries)}
		for _, e := range entries {
			mine, err := os.Lstat(filepath.Join(home, e.Name()))
			if err != nil {
				continue
			}
			theirs, err := os.Lstat(filepath.Join(path, e.Name()))
			if err != nil {
				continue
			}
			c.ThisProcess = os.SameFile(mine, theirs)
			break
		}
		out = append(out, c)
	}
	return out
}

// StateCopyNote is the one line a caller reads about a copy.
func StateCopyNote(c StateCopy) string {
	who := "A process Windows redirects into that package writes there, and this one does not"
	if c.ThisProcess {
		who = "THIS process writes there, and a process outside that package does not see what it writes"
	}
	return c.Package + " keeps its own copy of this tool's state at " + c.Path + ". " + who +
		": jobs, records, --exclusive locks and configuration. Call this tool from one kind of shell, or set WSL_TOOLKIT_HOME outside %LOCALAPPDATA% for every caller"
}
