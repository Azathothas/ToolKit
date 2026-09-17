package toolkit

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestAPrivateNetPayloadCrossesNoShell(t *testing.T) {
	// ⛔ A payload full of everything a shell would fight over. It arrives base64
	// and is decoded through a file in the guest, so none of it is ever parsed.
	payload := []byte("echo \"it's $HOME\" `date` && printf '%s\n' \"|;&<>\"\n")
	script, env, err := PrivateNetPayload(payload, "172.23.96.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(script) == 0 {
		t.Fatal("PrivateNetPayload returned no script to run")
	}
	got, err := base64.StdEncoding.DecodeString(env["TK_PRIVATE_NET_PAYLOAD_B64"])
	if err != nil {
		t.Fatalf("the payload did not survive as base64: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("the payload changed crossing the boundary:\n want %q\n got  %q", payload, got)
	}
	if strings.Contains(string(script), string(payload)) {
		t.Fatal("the payload is pasted into the script, where the guest's shell would parse it")
	}
}

// ⚠ EMPTY IS A REAL VALUE. A host address that could not be resolved means the
// rule refusing it is not written, and the script says so rather than dropping a
// guessed address that might be somebody else's.
func TestAnUnresolvedHostAddressIsPassedEmptyRatherThanGuessed(t *testing.T) {
	_, env, err := PrivateNetPayload([]byte("true\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := env["TK_HOSTADDR"]; !ok || got != "" {
		t.Fatalf("TK_HOSTADDR = %q, present %v; want it present and empty", got, ok)
	}
	if !strings.Contains(string(privateNetScript), `if [ -n "$TK_HOSTADDR" ]; then`) {
		t.Error("the script writes the host rule unconditionally, so an empty address would become a rule")
	}
}

func TestAnEmptyPrivateNetPayloadIsRefused(t *testing.T) {
	if _, _, err := PrivateNetPayload(nil, "172.23.96.1"); err == nil {
		t.Fatal("PrivateNetPayload(nil) was accepted, so a namespace would be made with nothing to run in it")
	}
}

// ⛔ THE ORDER OF THE RULES IS THE ORDER OF THE CHAIN, and WSL's resolver lives at
// a 10/8 address. A drop of the private ranges with no exception ahead of it takes
// DNS with it, and the payload then fails for a reason that looks nothing like a
// firewall. Measured 2026-09-17: with the resolver accepted first, DNS stayed up
// while the Windows host went from answering ICMP to refused.
func TestTheResolverIsAcceptedBeforeThePrivateRangesAreDropped(t *testing.T) {
	script := string(privateNetScript)
	accept := strings.Index(script, "out ip daddr %s accept")
	drop := strings.Index(script, "10.0.0.0/8, 172.16.0.0/12")
	if accept < 0 || drop < 0 {
		t.Fatal("the script no longer writes both a resolver exception and a private-range drop")
	}
	if accept > drop {
		t.Fatal("the private ranges are dropped before the resolver is accepted, so this base would lose DNS")
	}
	if !strings.Contains(script, "/etc/resolv.conf 2>/dev/null |") {
		t.Error("the exception no longer reads the resolvers this base actually uses")
	}
}

// ⛔ pasta WRITES TWO LINES TO stderr WITHOUT --quiet, and this wraps somebody
// else's command. Measured 2026-09-17: with it, `base exec --private-net` added
// ZERO bytes to stderr against the same command run plain, 116 against 116.
func TestTheNamespaceWrapperAddsNothingToTheCallersStreams(t *testing.T) {
	script := string(privateNetScript)
	if !strings.Contains(script, "exec pasta --quiet --config-net --") {
		t.Fatal("pasta is no longer run with --quiet, so every wrapped command gains two stderr lines")
	}
	// ⛔ exec, not a call: a wrapper that waits for its child owns the exit code,
	// and the payload's own status is what a caller reads.
	if !strings.Contains(script, `exec /bin/sh "$0.payload"`) {
		t.Error("the payload is no longer exec'd, so its exit status would be the wrapper's")
	}
}

// ⛔ A CONTAINER CANNOT RUN INSIDE THE NAMESPACE, and the file has to say so,
// because the base's whole purpose is running containers as this account. pasta
// puts the payload in a user namespace where it is uid 0, podman then takes
// itself for rootful and chooses system paths it cannot write.
func TestTheWrapperSaysAContainerCannotRunInside(t *testing.T) {
	if !strings.Contains(string(privateNetScript), "A CONTAINER CANNOT RUN IN HERE") {
		t.Error("the script no longer records that a container cannot run inside it, which is why this is a flag and not a default")
	}
}
