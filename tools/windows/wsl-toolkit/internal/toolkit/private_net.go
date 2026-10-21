// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"
)

//go:embed private-net.sh
var privateNetScript []byte

// PrivateNetPayload wraps a guest payload so it runs in a network namespace of
// the account's own, with the Windows host and the private ranges refused by a
// rule inside that namespace.
//
// ⭐ THE RULE IS NOT IN THE SHARED NAMESPACE. Every WSL distribution on a host
// shares one, so a rule written there would change the network of the podman
// machine and of every other base; the operator's ruling of 2026-09-14 permits
// option B only on that condition. Measured 2026-09-17: inside
// `pasta --config-net` the namespace is a different one, the shared one is
// unchanged before and after, and the Windows host went from answering ICMP to
// refused while the internet and DNS stayed up.
//
// ⛔ A CONTAINER CANNOT RUN INSIDE IT, which is why this is a flag and not how
// every command in the base starts. pasta puts the payload in a user namespace
// where it is uid 0, so podman takes itself for rootful and chooses system paths
// it cannot write. Measured three ways and it failed all three.
func PrivateNetPayload(payload []byte, hostAddress string) ([]byte, map[string]string, error) {
	if len(payload) == 0 {
		return nil, nil, errors.New("a private network namespace needs something to run in it")
	}
	env := map[string]string{
		"TK_PRIVATE_NET_PAYLOAD_B64": base64.StdEncoding.EncodeToString(payload),
		// ⚠ EMPTY IS A REAL VALUE AND NOT A DEFAULT. A host address that could not
		// be resolved means the rule that refuses it is not written, and the script
		// says so rather than dropping a guessed address.
		"TK_HOSTADDR": hostAddress,
	}
	return privateNetScript, env, nil
}

// PrivateShell places the launcher `base shell --private-net` runs, and answers
// its path in the guest: the private network wrapper with its variables in
// front, around the given shell. WSL-133.
//
// ⛔ THE LAUNCHER IS A FILE, because the terminal is the shell's stdin. `base
// exec --private-net` sends the wrapper on stdin, and an interactive shell cannot
// read its terminal from a pipe that is carrying its own script. The file travels
// through the archive channel a session's payload uses, never on a command line.
//
// ⭐ IT REMOVES ITS OWN DIRECTORY AS ITS FIRST LINE. The shell holds the file
// open, so reading on is unaffected, and a client killed later leaves nothing
// behind.
func PrivateShell(ctx context.Context, w *Wsl, distro, user string, shell []byte, hostAddress string) (string, error) {
	script, env, err := PrivateNetPayload(shell, hostAddress)
	if err != nil {
		return "", err
	}
	home, err := accountHome(ctx, w, distro, user)
	if err != nil {
		return "", err
	}
	id, err := newJobID()
	if err != nil {
		return "", err
	}
	dir := home + "/" + GuestRoot + "/shells/" + id
	if err := AssertArgvSafe([]string{dir}); err != nil {
		return "", err
	}
	errBuf := &boundedBuffer{max: 64 << 10}
	if code, err := w.ExecDirect(ctx, distro, user, "", []string{"/bin/mkdir", "-p", dir}, nil, io.Discard, errBuf, 2*time.Minute); err != nil || code != 0 {
		return "", fmt.Errorf("could not create %s in the guest (exit %d): %s", dir, code, guestFailure(errBuf.String()))
	}
	content := append([]byte("rm -rf "+shellQuote(dir)+"\n"), shellAssignments(env)...)
	content = append(content, script...)
	errBuf = &boundedBuffer{max: 64 << 10}
	if _, err := pumpArchive(
		func(pw io.Writer) (WorkspaceUpload, error) {
			return writeInputsTar(pw, []JobInput{
				{Name: privateShellName, Bytes: content},
			})
		},
		func(pr io.Reader) (int, error) {
			return w.ExecDirect(ctx, distro, user, "", []string{"/bin/tar", "-xf", "-", "-C", dir}, pr, io.Discard, errBuf, 2*time.Minute)
		},
		func() string { return errBuf.String() }); err != nil {
		return "", fmt.Errorf("the private shell's launcher: %w", err)
	}
	return dir + "/" + privateShellName, nil
}

// privateShellName is the launcher's file name in its directory.
const privateShellName = "enter.sh"
