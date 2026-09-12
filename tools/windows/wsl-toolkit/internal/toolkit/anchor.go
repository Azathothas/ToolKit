package toolkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ⛔ `--workspace .` IS A PATH THE CALLER DID NOT WRITE, and the thing it
// resolves to is whatever the process's working directory happens to be. That
// is not always the project: a sandbox can reset it to the drive root or to the
// home directory, and a tool installed at `%USERPROFILE%\bin` is commonly run
// from somewhere other than the tree it is meant to act on. The failure is
// quiet and expensive in both directions - a workspace copy walks a whole drive,
// and an artifact delivery writes a job's output over a home directory.
//
// ⭐ THE ANCHOR IS THE FIRST HALF OF THE ANSWER AND THIS IS THE SECOND. Where a
// project configuration exists, a relative path is resolved against it and the
// working directory stops mattering. Where none exists there is nothing to
// anchor to, so the remaining protection is to refuse the handful of
// directories that are never somebody's project.
//
// ⚠ IT IS A REFUSAL, NOT A CORRECTION. Guessing which directory the caller
// meant is how a tool acts on the wrong tree while reporting success.

// AssertProjectPath refuses a resolved host path that is a filesystem root, a
// home directory, or a well-known system directory.
//
// `what` names the flag in the caller's own spelling, so the message says which
// argument to change.
func AssertProjectPath(what, resolved string) error {
	if resolved == "" {
		return nil
	}
	clean := filepath.Clean(resolved)
	if reason := unsafeRootReason(clean); reason != "" {
		return fmt.Errorf(
			"%w: %s %s, and %s. Pass a path to the project itself, or put a wsl-toolkit.json in the project so a relative path is resolved against it rather than against the working directory",
			ErrWorkspaceRefused, what, clean, reason)
	}
	return nil
}

// unsafeRootReason names why a path is not a project, or returns empty.
func unsafeRootReason(clean string) string {
	if isFilesystemRoot(clean) {
		return "that is a filesystem root"
	}
	for _, env := range []string{"USERPROFILE", "HOME"} {
		if home := os.Getenv(env); home != "" && pathEqual(clean, filepath.Clean(home)) {
			return "that is the home directory itself"
		}
	}
	for _, env := range []string{"SystemRoot", "ProgramFiles", "ProgramFiles(x86)", "ProgramData", "LOCALAPPDATA", "APPDATA"} {
		if dir := os.Getenv(env); dir != "" && pathEqual(clean, filepath.Clean(dir)) {
			return "that is a system directory"
		}
	}
	// ⚠ `C:\Users` is the parent of every profile on the machine and is not
	// covered by USERPROFILE. It is named by shape rather than by environment
	// because no variable points at it.
	if parent := filepath.Dir(clean); isFilesystemRoot(parent) {
		switch strings.ToLower(filepath.Base(clean)) {
		case "users", "windows", "program files", "program files (x86)", "programdata", "home":
			return "that is a system directory"
		}
	}
	return ""
}

// isFilesystemRoot reports whether a cleaned absolute path has no parent.
//
// ⚠ It answers for both grammars. `C:\` and `\\server\share` are roots on
// Windows and `/` is one everywhere, and filepath on the other host would not
// recognise the first two.
func isFilesystemRoot(clean string) bool {
	if clean == "/" || clean == "\\" {
		return true
	}
	if vol := filepath.VolumeName(clean); vol != "" {
		rest := strings.TrimPrefix(clean, vol)
		return rest == "" || rest == string(filepath.Separator) || rest == "/"
	}
	// A POSIX host reading a Windows-shaped path still has to answer.
	if len(clean) == 3 && clean[1] == ':' && (clean[2] == '\\' || clean[2] == '/') {
		return true
	}
	return false
}
