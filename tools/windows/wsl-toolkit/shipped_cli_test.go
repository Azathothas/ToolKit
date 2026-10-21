// SPDX-License-Identifier: 0BSD

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Azathothas/ToolKit/tools/windows/wsl-toolkit/internal/toolkit"
)

// -- WSL-108, issue 37 item 4: cat and write take the one name they need ------

// ⛔ Both subcommands parsed through the guard every command shares, which
// refuses a positional argument, and then required exactly one. No spelling
// could reach the file, in a published release, and no case ran either.
func TestShippedCatWritesTheCarriedBytes(t *testing.T) {
	want, err := toolkit.ShippedBody("bootstrap.sh")
	if err != nil {
		t.Fatal(err)
	}
	var code int
	var runErr error
	got := captureStdout(t, func() {
		code, runErr = cmdShipped(context.Background(), []string{"cat", "bootstrap.sh"})
	})
	if code != exitOK || runErr != nil {
		t.Fatalf("shipped cat bootstrap.sh answered %d, %v", code, runErr)
	}
	if !bytes.Equal([]byte(got), want) {
		t.Fatalf("shipped cat wrote %d bytes that are not the %d the executable carries", len(got), len(want))
	}
}

func TestShippedWriteTakesItsNameOnEitherSideOfItsFlags(t *testing.T) {
	want, err := toolkit.ShippedBody("bootstrap.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, order := range []string{"name first", "name last", "name between"} {
		dir := t.TempDir()
		args := map[string][]string{
			"name first":   {"write", "bootstrap.sh", "--dir", dir},
			"name last":    {"write", "--dir", dir, "bootstrap.sh"},
			"name between": {"write", "--dir", dir, "bootstrap.sh", "--force"},
		}[order]
		// ⚠ THE FLAG AFTER THE NAME HAS TO MATTER, or a parse that drops it
		// passes. A different file already there is refused without --force.
		if order == "name between" {
			if err := os.WriteFile(filepath.Join(dir, "bootstrap.sh"), []byte("not the carried file\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		code, runErr := cmdShipped(context.Background(), args)
		if code != exitOK || runErr != nil {
			t.Errorf("%s: shipped %s answered %d, %v", order, strings.Join(args, " "), code, runErr)
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, "bootstrap.sh"))
		if err != nil {
			t.Errorf("%s: nothing was written into --dir: %v", order, err)
			continue
		}
		if !bytes.Equal(body, want) {
			t.Errorf("%s: the file written is not the one the executable carries", order)
		}
	}
}

func TestShippedRefusesASecondNameAndListRefusesAny(t *testing.T) {
	code, err := cmdShipped(context.Background(), []string{"cat", "bootstrap.sh", "tmux.conf"})
	if code != exitCannot || err == nil || !strings.Contains(err.Error(), `"tmux.conf" is a second`) {
		t.Errorf("a second name answered %d, %v; want a refusal naming it", code, err)
	}
	code, err = cmdShipped(context.Background(), []string{"cat"})
	if code != exitCannot || err == nil || !strings.Contains(err.Error(), "takes one name") {
		t.Errorf("no name answered %d, %v; want a refusal saying one is needed", code, err)
	}
	code, err = cmdShipped(context.Background(), []string{"list", "bootstrap.sh"})
	if code != exitCannot || err == nil || !strings.Contains(err.Error(), "not positional arguments") {
		t.Errorf("list with a name answered %d, %v; want the positional refusal", code, err)
	}
}
