// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// HostAddressSchema versions the answer.
const HostAddressSchema = "wsl-toolkit-hostaddress/1"

// HostAddress is the address a WSL distribution reaches this Windows host at.
//
// ⛔ THE WRONG ANSWER HERE IS PLAUSIBLE AND SILENT. A service bound to 127.0.0.1
// on the host is reachable from a distribution in mirrored mode and never
// reachable in NAT mode, and nothing on either side says why a fixture never
// received a connection.
type HostAddress struct {
	Schema    string `json:"schema"`
	Address   string `json:"address"`
	Mode      string `json:"mode"`
	Source    string `json:"source"`
	Config    string `json:"config,omitempty"`
	Interface string `json:"interface,omitempty"`
	// Loopback says whether a host service bound to 127.0.0.1 is reachable from
	// inside a distribution.
	Loopback bool `json:"loopback_reachable"`
}

// ResolveHostAddress answers without starting or creating anything: it reads
// %USERPROFILE%\.wslconfig and this host's network interfaces.
//
// ⛔ IT REFUSES RATHER THAN GUESSING. A mode with more than one right answer, or
// a NAT host whose WSL adapter is not up yet, is a refusal naming what to do.
func ResolveHostAddress() (HostAddress, error) {
	ans := HostAddress{Schema: HostAddressSchema, Mode: "nat", Source: "the WSL default, no .wslconfig"}
	if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
		path := filepath.Join(profile, ".wslconfig")
		if data, err := os.ReadFile(path); err == nil {
			ans.Config = path
			if mode := parseWslConfigMode(string(data)); mode != "" {
				ans.Mode, ans.Source = mode, ".wslconfig"
			} else {
				ans.Source = "the WSL default, no networkingMode in .wslconfig"
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return ans, fmt.Errorf("%s exists and could not be read, so the networking mode is unknown: %w", path, err)
		}
	}
	switch ans.Mode {
	case "mirrored":
		ans.Address, ans.Interface, ans.Loopback = "127.0.0.1", "loopback", true
		return ans, nil
	case "nat":
		ifaces, err := net.Interfaces()
		if err != nil {
			return ans, fmt.Errorf("this host's network interfaces could not be read: %w", err)
		}
		var adapters []adapterInfo
		for _, n := range ifaces {
			a := adapterInfo{Name: n.Name, Up: n.Flags&net.FlagUp != 0}
			if addrs, err := n.Addrs(); err == nil {
				for _, addr := range addrs {
					if ipn, ok := addr.(*net.IPNet); ok {
						a.Addrs = append(a.Addrs, ipn.IP)
					}
				}
			}
			adapters = append(adapters, a)
		}
		addr, name, ok := pickWSLAdapter(adapters)
		if !ok {
			return ans, errors.New("NAT mode, and no WSL network adapter is up on this host. WSL creates it when its " +
				"virtual machine first starts, so start any distribution and ask again. Nothing was started to find out")
		}
		ans.Address, ans.Interface = addr, name
		return ans, nil
	default:
		return ans, fmt.Errorf("networkingMode is %q, and an address is answered only for nat and mirrored. In %s mode "+
			"a distribution reaches this host at whichever address is on the network it joined, which is a choice rather "+
			"than a lookup. Read the default route from inside the distribution instead", ans.Mode, ans.Mode)
	}
}

type adapterInfo struct {
	Name  string
	Up    bool
	Addrs []net.IP
}

// pickWSLAdapter finds the IPv4 address of the adapter WSL's NAT network uses.
//
// ⚠ MATCHED ON A PREFIX. Windows has named it `vEthernet (WSL)` and
// `vEthernet (WSL (Hyper-V firewall))`, and a later build may name it again.
func pickWSLAdapter(adapters []adapterInfo) (string, string, bool) {
	for _, a := range adapters {
		if !a.Up || !strings.HasPrefix(strings.ToLower(a.Name), "vethernet (wsl") {
			continue
		}
		for _, ip := range a.Addrs {
			if v4 := ip.To4(); v4 != nil {
				return v4.String(), a.Name, true
			}
		}
	}
	return "", "", false
}

// parseWslConfigMode reads networkingMode from a .wslconfig. Empty means the key
// is not set.
//
// ⛔ A COMMENTED SETTING IS NOT A SETTING, the SECTION MATTERS, and the LAST ONE
// WINS. Real files carry the alternatives commented out above the live one, and
// `[experimental]` carries keys with related names. The value ends at the first
// space or comment character, so `nat mirrored` is `nat`.
func parseWslConfigMode(body string) string {
	section, mode := "", ""
	for _, raw := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if section != "wsl2" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "networkingMode") {
			continue
		}
		value = strings.TrimSpace(value)
		if i := strings.IndexAny(value, " \t#;"); i >= 0 {
			value = value[:i]
		}
		if value != "" {
			mode = strings.ToLower(value)
		}
	}
	return mode
}
