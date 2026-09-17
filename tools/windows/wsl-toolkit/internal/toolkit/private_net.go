// SPDX-License-Identifier: 0BSD

package toolkit

import (
	_ "embed"
	"encoding/base64"
	"errors"
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
