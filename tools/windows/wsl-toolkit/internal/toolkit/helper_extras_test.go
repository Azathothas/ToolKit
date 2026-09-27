// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestAHelperRequestIsHeldToTheDirectRules decodes a request's devices and
// inputs and refuses what the direct path refuses, because a request is not
// trusted for arriving over a local socket.
func TestAHelperRequestIsHeldToTheDirectRules(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	ok := HelperRunRequest{
		Devices: []string{"/dev/kvm:/dev/kvm:rw"},
		Inputs: []HelperInput{
			{Name: "job.sh", B64: b64("true\n")},
		},
	}
	devices, inputs, err := ok.jobExtras()
	if err != nil || len(devices) != 1 || devices[0].Perms != "rw" || len(inputs) != 1 || string(inputs[0].Bytes) != "true\n" {
		t.Fatalf("a valid request decoded to %+v %+v, %v", devices, inputs, err)
	}
	refused := map[string]HelperRunRequest{
		"not under /dev": {Devices: []string{"/etc/passwd"}},
		"not base64": {Inputs: []HelperInput{
			{Name: "a", B64: "!!"},
		}},
		"component": {Inputs: []HelperInput{
			{Name: "../a", B64: b64("x")},
		}},
		"one request": {Inputs: []HelperInput{
			{Name: "big", B64: base64.StdEncoding.EncodeToString(make([]byte, MaxHelperInputBytes+1))},
		}},
		"names \"a\" twice": {Inputs: []HelperInput{
			{Name: "a", B64: b64("1")},
			{Name: "a", B64: b64("2")},
		}},
	}
	for want, req := range refused {
		if _, _, err := req.jobExtras(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("case %q: err = %v", want, err)
		}
	}
}
