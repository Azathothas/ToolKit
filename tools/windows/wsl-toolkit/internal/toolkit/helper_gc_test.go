// SPDX-License-Identifier: 0BSD

package toolkit

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// TestEveryCleanupFieldCrossesTheWire sets every field of CleanupPolicy, sends
// it the way the client does, reads it the way the helper does, and compares.
// WSL-102: Job was dropped, so a cleanup of one job through the helper removed
// every job that was not live.
func TestEveryCleanupFieldCrossesTheWire(t *testing.T) {
	var p CleanupPolicy
	v := reflect.ValueOf(&p).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		name := v.Type().Field(i).Name
		if why, ok := cleanupLocalOnly[name]; ok {
			// ⛔ A field kept off the wire must be refused there, or it is dropped.
			one := CleanupPolicy{}
			reflect.ValueOf(&one).Elem().Field(i).SetString("C:/src/a")
			if _, err := (&HelperClient{}).Cleanup(context.Background(), false, one, false); err == nil {
				t.Errorf("CleanupPolicy.%s is kept off the wire (%s) and the client sent it anyway", name, why)
			}
			continue
		}
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int64:
			f.SetInt(int64(90 * time.Second))
		case reflect.String:
			f.SetString("0123456789abcdef")
		default:
			t.Fatalf("CleanupPolicy.%s is a %s, which this walk does not set. Teach it, so the field is proved to cross", v.Type().Field(i).Name, f.Kind())
		}
	}
	b, err := json.Marshal(gcRequest(true, p, true))
	if err != nil {
		t.Fatal(err)
	}
	var req HelperGCRequest
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	got, err := req.policy()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Errorf("the helper read %+v; the client sent %+v", got, p)
	}
	if !req.Apply || !req.Images {
		t.Errorf("apply and images did not cross: %+v", req)
	}
}

// cleanupLocalOnly are the fields the client refuses rather than sends, each
// with its reason.
var cleanupLocalOnly = map[string]string{
	"Workspace": "WSL-114: the helper's jobs record its uploads, and an older helper would drop it",
}

func TestTheHelperRefusesAJobThatIsNotAnID(t *testing.T) {
	if _, err := (HelperGCRequest{Job: "../../x"}).policy(); err == nil {
		t.Error("a job that is not an id narrowed a cleanup")
	}
}
