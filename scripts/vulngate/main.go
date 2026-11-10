// Command vulngate runs govulncheck and fails on any called vulnerability that
// is not in the allowlist, and on any allowlist entry past its expiry date.
//
// Usage: go run ./scripts/vulngate <govulncheck-binary> <allowlist> [packages...]
//
// Only symbol-level findings gate, matching govulncheck's own text-mode exit
// code: module- and package-level findings stay informational.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: vulngate <govulncheck-binary> <allowlist> [packages...]")
		os.Exit(2)
	}
	allow, err := os.ReadFile(os.Args[2])
	if err != nil {
		fail(err)
	}
	pkgs := os.Args[3:]
	if len(pkgs) == 0 {
		pkgs = []string{"./..."}
	}
	// govulncheck -format json exits 0 even with findings, so a non-zero exit
	// here means the scan itself broke; fail closed instead of reading partial output.
	var out bytes.Buffer
	cmd := exec.Command(os.Args[1], append([]string{"-format", "json"}, pkgs...)...)
	cmd.Stdout, cmd.Stderr = &out, os.Stderr
	if err := cmd.Run(); err != nil {
		fail(fmt.Errorf("govulncheck: %w", err))
	}
	if err := gate(&out, string(allow), time.Now().UTC()); err != nil {
		fail(err)
	}
	fmt.Fprintln(os.Stderr, "vulngate: no unlisted called vulnerabilities")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "vulngate:", err)
	os.Exit(1)
}

type entry struct {
	expires time.Time
	reason  string
}

var idPattern = regexp.MustCompile(`^GO-\d{4}-\d+$`)

// parseAllowlist reads "<GO-ID> <YYYY-MM-DD> <reason...>" lines; blank lines
// and lines starting with # are skipped.
func parseAllowlist(src string) (map[string]entry, error) {
	allow := map[string]entry{}
	for n, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 || !idPattern.MatchString(f[0]) {
			return nil, fmt.Errorf("allowlist line %d: want \"<GO-ID> <YYYY-MM-DD> <reason>\", got %q", n+1, line)
		}
		expires, err := time.Parse(time.DateOnly, f[1])
		if err != nil {
			return nil, fmt.Errorf("allowlist line %d: %w", n+1, err)
		}
		allow[f[0]] = entry{expires: expires, reason: strings.Join(f[2:], " ")}
	}
	return allow, nil
}

type message struct {
	Config  json.RawMessage `json:"config"`
	Finding *struct {
		OSV   string `json:"osv"`
		Trace []struct {
			Function string `json:"function"`
		} `json:"trace"`
	} `json:"finding"`
}

// gate checks a govulncheck JSON stream against the allowlist. An entry is
// valid through its expiry date (UTC) and expired from the day after.
func gate(stream io.Reader, allowlist string, today time.Time) error {
	allow, err := parseAllowlist(allowlist)
	if err != nil {
		return err
	}
	var errs []error
	day := today.UTC().Truncate(24 * time.Hour)
	for id, e := range allow {
		if day.After(e.expires) {
			errs = append(errs, fmt.Errorf("%s: allowlist entry expired on %s (%s)", id, e.expires.Format(time.DateOnly), e.reason))
		}
	}

	var sawConfig bool
	var unlisted []string
	dec := json.NewDecoder(stream)
	for {
		var m message
		if err := dec.Decode(&m); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return fmt.Errorf("decode govulncheck output: %w", err)
		}
		sawConfig = sawConfig || m.Config != nil
		f := m.Finding
		if f == nil || len(f.Trace) == 0 || f.Trace[0].Function == "" {
			continue
		}
		if _, ok := allow[f.OSV]; !ok && !slices.Contains(unlisted, f.OSV) {
			unlisted = append(unlisted, f.OSV)
		}
	}
	if !sawConfig {
		return errors.New("govulncheck output has no config message; scan did not run")
	}
	slices.Sort(unlisted)
	for _, id := range unlisted {
		errs = append(errs, fmt.Errorf("%s: called vulnerability not in allowlist (see https://pkg.go.dev/vuln/%s)", id, id))
	}
	return errors.Join(errs...)
}
