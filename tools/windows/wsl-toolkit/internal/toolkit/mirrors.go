// SPDX-License-Identifier: 0BSD

package toolkit

import (
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// How the base's engines reach a registry: a mirror tried before a registry, and
// a proxy for the engine's own traffic. WSL-196.
//
// ⭐ SET FROM THE HOST. WSL-188 moved provisioning's engine settings to drop-ins,
// so a mirror written by hand in a main file survived, and each base was still
// set up by hand inside the guest. base.registry names both, and the executable
// renders the two drop-ins.
//
// ⛔ A BASE WITHOUT THE KEY IS BYTE FOR BYTE UNCHANGED. Neither drop-in is written
// for it, the installer says nothing, and its status and configuration reports
// carry no new row or field.

//go:embed mirrors.sh
var mirrorsScript []byte

// RegistryConfig is base.registry. WSL-196.
type RegistryConfig struct {
	// Mirrors maps a registry, as an image reference names it, to the mirrors the
	// engine tries before it, in order. The registry itself is tried last.
	Mirrors map[string][]string `json:"mirrors,omitempty"`
	// Proxy is the HTTP or SOCKS proxy the engine's own traffic takes, such as
	// http://proxy.example:3128. A pull goes through it.
	Proxy string `json:"proxy,omitempty"`
	// NoProxy names the hosts the engine reaches without the proxy.
	NoProxy []string `json:"no_proxy,omitempty"`
}

// empty says the key states nothing, which is read as no key.
func (r *RegistryConfig) empty() bool {
	return r == nil || (len(r.Mirrors) == 0 && r.Proxy == "" && len(r.NoProxy) == 0)
}

// The two drop-ins base.registry is written into. ⚠ Both are in /etc, so root's
// engine reads them as the account's does: binfmt.sh pulls the QEMU image as root.
const (
	MirrorsDropIn = "/etc/containers/registries.conf.d/99-wsl-toolkit-mirrors.conf"
	ProxyDropIn   = "/etc/containers/containers.conf.d/99-wsl-toolkit-proxy.conf"
)

// validateRegistry holds base.registry to what the two drop-ins can carry.
//
// ⛔ EVERY VALUE IS WRITTEN INTO A TOML STRING, so a quote, a backslash or a
// control character is refused here and never escaped there.
func validateRegistry(r *RegistryConfig) error {
	if r.empty() {
		return nil
	}
	prefixes := make([]string, 0, len(r.Mirrors))
	for prefix := range r.Mirrors {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		if err := registryLocation(prefix); err != nil {
			return fmt.Errorf("base.registry.mirrors names %q: %w", prefix, err)
		}
		mirrors := r.Mirrors[prefix]
		if len(mirrors) == 0 {
			return fmt.Errorf("base.registry.mirrors[%q] is empty. Name at least one mirror, or leave the registry out", prefix)
		}
		seen := map[string]bool{}
		for _, m := range mirrors {
			if err := registryLocation(m); err != nil {
				return fmt.Errorf("base.registry.mirrors[%q] names %q: %w", prefix, m, err)
			}
			if m == prefix || seen[m] {
				return fmt.Errorf("base.registry.mirrors[%q] names %q twice. The registry itself is tried last, so leave it out of its mirrors", prefix, m)
			}
			seen[m] = true
		}
	}
	if r.Proxy != "" {
		if err := validateProxy(r.Proxy); err != nil {
			return err
		}
	}
	if len(r.NoProxy) > 0 && r.Proxy == "" {
		return errors.New("base.registry.no_proxy names the hosts a proxy is not used for, and base.registry.proxy is empty. Set the proxy, or leave no_proxy out")
	}
	for _, host := range r.NoProxy {
		if host == "" || strings.ContainsAny(host, " ,\"'\\\t\r\n") || !printable(host) {
			return fmt.Errorf("base.registry.no_proxy names %q. Each entry is one host, domain, address or range, with no space, comma or quote", host)
		}
	}
	return nil
}

// registryLocation holds a registry or a mirror to the form registries.conf reads:
// a host, an optional port, and an optional path, in lower case.
func registryLocation(v string) error {
	switch {
	case strings.Contains(v, "://"):
		return errors.New("a registry is named without a scheme, such as mirror.example:5000")
	case v == "" || strings.HasPrefix(v, "/") || strings.HasSuffix(v, "/") || strings.Contains(v, "//"):
		return errors.New("a registry is a host, an optional port and an optional path, such as mirror.example:5000/library")
	}
	for _, c := range v {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '-', c == '_', c == ':', c == '/':
		default:
			return errors.New("a registry is spelled in lower case letters, digits, dots, dashes, underscores, a colon before its port and slashes in its path")
		}
	}
	host, _, _ := strings.Cut(v, "/")
	if name, _, _ := strings.Cut(host, ":"); name == "" {
		return errors.New("a registry names its host first, such as mirror.example:5000")
	}
	return nil
}

// validateProxy holds base.registry.proxy to a URL the engine's environment can
// carry.
//
// ⛔ A PROXY THAT CARRIES A USER OR A PASSWORD IS REFUSED. `config` and `base
// status` print the key, the helper carries it, the drop-in is world-readable,
// and podman hands the proxy to each container it starts. A credential stays
// the operator's, so a proxy that needs one is reached through a local one that
// holds it.
func validateProxy(v string) error {
	if strings.ContainsAny(v, " \"'\\\t\r\n") || !printable(v) {
		return fmt.Errorf("base.registry.proxy %q carries a space, a quote or a control character", v)
	}
	u, err := url.Parse(v)
	if err != nil {
		return fmt.Errorf("base.registry.proxy %q is not a URL: %w", v, err)
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return fmt.Errorf("base.registry.proxy %q must start with http://, https:// or socks5://", v)
	}
	if u.User != nil {
		return fmt.Errorf("base.registry.proxy %q carries a user or a password, which config and base status would print and each container would see. "+
			"Name a proxy that needs none, such as a local one that holds the credential", redactedProxy(u))
	}
	if u.Hostname() == "" {
		return fmt.Errorf("base.registry.proxy %q names no host, such as http://proxy.example:3128", v)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("base.registry.proxy %q carries a path or a query, and a proxy is a scheme, a host and a port", v)
	}
	return nil
}

// redactedProxy is a proxy URL with its password left out, for a refusal.
func redactedProxy(u *url.URL) string {
	c := *u
	if c.User != nil {
		c.User = url.User(c.User.Username())
	}
	return c.String()
}

// printable says v carries no control character.
func printable(v string) bool {
	for _, c := range v {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return true
}

// mirrorsDropIn is the registries.conf drop-in base.registry.mirrors renders,
// and empty where it names none. ⭐ Sorted by registry, so the same key writes
// the same bytes and an ensure that changes nothing leaves the file alone.
func mirrorsDropIn(r *RegistryConfig) string {
	if r == nil || len(r.Mirrors) == 0 {
		return ""
	}
	prefixes := make([]string, 0, len(r.Mirrors))
	for prefix := range r.Mirrors {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	var b strings.Builder
	b.WriteString("# Written by wsl-toolkit from base.registry.mirrors. WSL-196.\n")
	for _, prefix := range prefixes {
		fmt.Fprintf(&b, "\n[[registry]]\nprefix = %q\nlocation = %q\n", prefix, prefix)
		for _, m := range r.Mirrors[prefix] {
			fmt.Fprintf(&b, "\n[[registry.mirror]]\nlocation = %q\n", m)
		}
	}
	return b.String()
}

// proxyDropIn is the containers.conf drop-in base.registry.proxy renders, and
// empty where it names none.
//
// ⚠ BOTH SPELLINGS OF EACH NAME. Go's engine reads either, and curl inside a
// container reads only the lower case one.
func proxyDropIn(r *RegistryConfig) string {
	if r == nil || r.Proxy == "" {
		return ""
	}
	env := []string{"http_proxy=" + r.Proxy, "https_proxy=" + r.Proxy, "HTTP_PROXY=" + r.Proxy, "HTTPS_PROXY=" + r.Proxy}
	if len(r.NoProxy) > 0 {
		list := strings.Join(r.NoProxy, ",")
		env = append(env, "no_proxy="+list, "NO_PROXY="+list)
	}
	quoted := make([]string, len(env))
	for i, e := range env {
		quoted[i] = fmt.Sprintf("%q", e)
	}
	return "# Written by wsl-toolkit from base.registry.proxy. WSL-196.\n[engine]\nenv = [" + strings.Join(quoted, ", ") + "]\n"
}

// registryEnv is what provision.sh and engines.sh are handed for base.registry:
// each drop-in's text, empty where the key names none.
func registryEnv(r *RegistryConfig) map[string]string {
	enc := func(s string) string {
		if s == "" {
			return ""
		}
		return base64.StdEncoding.EncodeToString([]byte(s))
	}
	return map[string]string{
		"TK_REGISTRY_MIRRORS_B64": enc(mirrorsDropIn(r)),
		"TK_REGISTRY_PROXY_B64":   enc(proxyDropIn(r)),
	}
}

// RegistryRows is base.registry as `config` and `base status` print it, one row
// each, and none where the key is not set.
func RegistryRows(r *RegistryConfig) [][2]string {
	if r.empty() {
		return nil
	}
	var rows [][2]string
	prefixes := make([]string, 0, len(r.Mirrors))
	for prefix := range r.Mirrors {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	for _, prefix := range prefixes {
		rows = append(rows, [2]string{"mirror", prefix + " through " + strings.Join(r.Mirrors[prefix], ", ") + ", then " + prefix + " itself"})
	}
	if r.Proxy != "" {
		proxy := r.Proxy
		if len(r.NoProxy) > 0 {
			proxy += ", not for " + strings.Join(r.NoProxy, ", ")
		}
		rows = append(rows, [2]string{"proxy", proxy})
	}
	return rows
}
