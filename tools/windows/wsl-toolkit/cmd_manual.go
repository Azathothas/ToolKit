package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

func cmdMan(ctx context.Context, args []string) (int, error) {
	fs := newFlagSet("man")
	output := fs.String("output", "", "write the generated roff man page to this file")
	noPager := fs.Bool("no-pager", false, "write the complete readable manual to stdout")
	if err := parseArgs(fs, args); err != nil {
		return exitCannot, err
	}
	if *output != "" && *noPager {
		return exitCannot, errors.New("--output and --no-pager select different output formats; use one")
	}
	if *output != "" {
		page, err := renderManPage(ctx)
		if err != nil {
			return exitCannot, err
		}
		if err := os.WriteFile(*output, []byte(page), 0o644); err != nil {
			return exitCannot, err
		}
		return exitOK, nil
	}
	text, err := renderManualText(ctx)
	if err != nil {
		return exitCannot, err
	}
	if *noPager || !isTerminal(os.Stdout) {
		_, err = fmt.Print(text)
		return exitOK, err
	}
	return exitOK, pageManual(text)
}

func renderManPage(ctx context.Context) (string, error) {
	sets, err := collectManualFlagSets(ctx)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, ".TH WSL-TOOLKIT 1 \"\" \"wsl-toolkit %s\" \"User Commands\"\n", versionString())
	out.WriteString(".SH NAME\nwsl-toolkit \\- run isolated Linux container jobs from Windows\n")
	out.WriteString(".SH SYNOPSIS\n.nf\n")
	out.WriteString(roffEscape(usage()) + "\n.fi\n")
	out.WriteString(".SH COMMAND REFERENCE\n")
	for _, spec := range commandSpecs {
		out.WriteString(".SS " + roffEscape(spec.Name) + "\n")
		out.WriteString(roffEscape(spec.Summary) + "\n")
		if len(spec.HelpForms) == 0 {
			out.WriteString(".PP\nThis command forwards its arguments to the embedded compatibility interface.\n")
			continue
		}
		for _, form := range spec.HelpForms {
			fs, ok := sets[form]
			if !ok {
				return "", fmt.Errorf("the %q help form did not register a flag set", form)
			}
			out.WriteString(".TP\n.B " + roffEscape("wsl-toolkit "+form) + "\n")
			flags := flagsInNameOrder(fs)
			if len(flags) == 0 {
				out.WriteString("This form has no options.\n")
				continue
			}
			for _, f := range flags {
				out.WriteString(".TP\n\\fB--" + roffEscape(f.Name) + "\\fR\n")
				text := strings.TrimSpace(f.Usage)
				if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
					text += ". Default: " + f.DefValue + "."
				}
				out.WriteString(roffEscape(text) + "\n")
			}
		}
	}
	out.WriteString(".SH EXAMPLES\n")
	for _, example := range examples {
		out.WriteString(".TP\n" + roffEscape(example.What) + "\n.nf\n")
		out.WriteString(roffEscape(example.Command) + "\n.fi\n")
	}
	out.WriteString(".SH SEE ALSO\n")
	// ⛔ A RAW STRING, because roff's font escape and Go's form feed are spelled
	// the same. `"\fB"` in an interpreted literal is one byte, 0x0C, so the page
	// shipped a control character where it meant to change font. The drift test
	// compared generated output with generated output and agreed with itself;
	// what caught it was reading the bytes. TestGeneratedManPageIsText is the
	// assertion that does not need a reader.
	out.WriteString(`Run \fBwsl-toolkit examples\fR for executable examples and \fBwsl-toolkit COMMAND --help\fR for direct help.` + "\n")
	return out.String(), nil
}

func renderManualText(ctx context.Context) (string, error) {
	sets, err := collectManualFlagSets(ctx)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString(usage() + "\n\n")
	out.WriteString("COMMAND REFERENCE\n")
	for _, spec := range commandSpecs {
		out.WriteString("\n" + strings.ToUpper(spec.Name) + "\n")
		out.WriteString("  " + spec.Summary + "\n")
		if len(spec.HelpForms) == 0 {
			out.WriteString("  This command forwards its arguments to the embedded compatibility interface.\n")
			continue
		}
		for _, form := range spec.HelpForms {
			fs, ok := sets[form]
			if !ok {
				return "", fmt.Errorf("the %q help form did not register a flag set", form)
			}
			out.WriteString("\n  wsl-toolkit " + form + "\n")
			for _, f := range flagsInNameOrder(fs) {
				out.WriteString("    --" + f.Name + "\n")
				out.WriteString("        " + strings.TrimSpace(f.Usage))
				if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" {
					out.WriteString(" Default: " + f.DefValue + ".")
				}
				out.WriteString("\n")
			}
		}
	}
	out.WriteString("\nEXAMPLES\n")
	for _, example := range examples {
		out.WriteString("\n  " + example.What + "\n")
		out.WriteString("    " + example.Command + "\n")
	}
	return out.String(), nil
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func pageManual(text string) error {
	for _, pager := range []struct {
		name string
		args []string
	}{
		{name: "less", args: []string{"-R"}},
		{name: "more.com"},
		{name: "more"},
	} {
		path, err := exec.LookPath(pager.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, pager.args...)
		cmd.Stdin = strings.NewReader(text)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	_, err := fmt.Print(text)
	return err
}

func collectManualFlagSets(ctx context.Context) (map[string]*flag.FlagSet, error) {
	flagSets.Lock()
	flagSets.byName = map[string]*flag.FlagSet{}
	flagSets.Unlock()

	realErr := os.Stderr
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	os.Stderr = devnull
	defer func() {
		os.Stderr = realErr
		_ = devnull.Close()
	}()

	for _, spec := range commandSpecs {
		for _, form := range spec.HelpForms {
			parts := strings.Fields(form)
			args := append(append([]string{}, parts[1:]...), "--help")
			_, callErr := spec.Run(ctx, args)
			if callErr != nil && !errors.Is(callErr, flag.ErrHelp) {
				return nil, fmt.Errorf("collect help for %q: %w", form, callErr)
			}
		}
	}

	flagSets.Lock()
	defer flagSets.Unlock()
	out := make(map[string]*flag.FlagSet, len(flagSets.byName))
	for name, fs := range flagSets.byName {
		out[name] = fs
	}
	return out, nil
}

func flagsInNameOrder(fs *flag.FlagSet) []*flag.Flag {
	var out []*flag.Flag
	fs.VisitAll(func(f *flag.Flag) { out = append(out, f) })
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func roffEscape(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	if strings.HasPrefix(value, ".") || strings.HasPrefix(value, "'") {
		value = `\&` + value
	}
	return value
}
