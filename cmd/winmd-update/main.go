// Command winmd-update refreshes the pinned .winmd fixtures in
// testdata/PROVENANCE.json against the versions currently published on
// nuget.org.
//
// For every record in the provenance file it resolves the newest published
// version of that NuGet package, and when that is ahead of the pin it
// downloads the nupkg, extracts the recorded file, and rewrites the record
// with the new version, source URL, sha256 and fetch date. The extracted
// winmd is written next to the provenance file so the decode suites reuse it
// as their cache instead of downloading it a second time.
//
// A pin on a prerelease (Windows.Win32.winmd ships as "-preview") tracks
// prereleases; a pin on a stable version tracks stable versions only.
//
// Exit status is 0 whether or not anything moved — "no update available" is
// not a failure. Use -check to report without writing.
//
//	go run ./cmd/winmd-update              # update the pins in place
//	go run ./cmd/winmd-update -check       # report only
//	go run ./cmd/winmd-update -package Microsoft.Windows.SDK.Contracts
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/go-winmd/pkg/nuget"
)

func main() {
	provenancePath := flag.String("provenance", filepath.Join("testdata", "PROVENANCE.json"),
		"path to the PROVENANCE.json holding the pins")
	outDir := flag.String("out", "",
		"directory to write fetched winmd files into (default: the provenance file's directory)")
	only := flag.String("package", "",
		"restrict the check to this NuGet package (display name, as recorded in provenance)")
	check := flag.Bool("check", false,
		"report available updates without writing PROVENANCE.json or fetching")
	flag.Parse()

	if err := run(*provenancePath, *outDir, *only, *check); err != nil {
		fmt.Fprintf(os.Stderr, "winmd-update: %v\n", err)
		os.Exit(1)
	}
}

// update is one package's outcome, for the summary and the PR body.
type update struct {
	Package string
	From    string
	To      string
	File    string
	SHA256  string
	Changed bool
}

func run(provenancePath, outDir, only string, check bool) error {
	records, err := nuget.ReadProvenance(provenancePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", provenancePath, err)
	}
	if len(records) == 0 {
		return fmt.Errorf("%s holds no records", provenancePath)
	}
	if outDir == "" {
		outDir = filepath.Dir(provenancePath)
	}

	client := nuget.NewClient()
	updates := make([]update, 0, len(records))
	matched := false

	for i := range records {
		pin := &records[i]
		if only != "" && !strings.EqualFold(pin.Package, only) {
			continue
		}
		matched = true

		result, err := checkOne(client, pin, outDir, check)
		if err != nil {
			return fmt.Errorf("%s: %w", pin.Package, err)
		}
		updates = append(updates, result)
	}
	if only != "" && !matched {
		return fmt.Errorf("%s holds no record for package %q", provenancePath, only)
	}

	changed := false
	for _, u := range updates {
		if u.Changed {
			changed = true
		}
	}
	if changed && !check {
		if err := nuget.WriteProvenance(provenancePath, records); err != nil {
			return fmt.Errorf("writing %s: %w", provenancePath, err)
		}
	}

	report(updates, changed, check)
	return emitGitHubOutputs(updates, changed)
}

// checkOne resolves the newest version for one pin and, unless checking,
// fetches it and rewrites the record in place.
func checkOne(client *http.Client, pin *nuget.Provenance, outDir string, check bool) (update, error) {
	result := update{Package: pin.Package, From: pin.Version, To: pin.Version, File: pin.File, SHA256: pin.SHA256}

	pkgID := strings.ToLower(pin.Package)
	versions, err := nuget.Versions(client, pkgID)
	if err != nil {
		return result, err
	}

	latest, err := newest(versions, nuget.IsPrerelease(pin.Version))
	if err != nil {
		return result, err
	}
	if !isAhead(versions, pin.Version, latest) {
		return result, nil
	}
	result.To = latest
	result.Changed = true
	if check {
		return result, nil
	}

	content, fetched, err := nuget.Fetch(client, pkgID, pin.Package, latest, pin.File)
	if err != nil {
		return result, err
	}
	path := filepath.Join(outDir, filepath.Base(pin.File))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return result, err
	}
	// The decode suites re-hash this cached file against the new pin on their
	// next run, so a bad write surfaces there rather than being papered over.
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return result, fmt.Errorf("writing %s: %w", path, err)
	}

	*pin = fetched
	result.SHA256 = fetched.SHA256
	return result, nil
}

// newest returns the last version in the ascending flat-container index,
// skipping prereleases unless the pin itself is a prerelease.
func newest(versions []string, allowPrerelease bool) (string, error) {
	for i := len(versions) - 1; i >= 0; i-- {
		if allowPrerelease || !nuget.IsPrerelease(versions[i]) {
			return versions[i], nil
		}
	}
	return "", fmt.Errorf("the NuGet index lists no stable versions")
}

// isAhead reports whether candidate sits later than pinned in the ascending
// version index — the guard that stops a pin ever being walked backwards.
// A pin absent from the index (yanked, or unlisted) is treated as behind.
func isAhead(versions []string, pinned, candidate string) bool {
	pinnedAt, candidateAt := -1, -1
	for i, v := range versions {
		if strings.EqualFold(v, pinned) {
			pinnedAt = i
		}
		if strings.EqualFold(v, candidate) {
			candidateAt = i
		}
	}
	if candidateAt < 0 {
		return false
	}
	return candidateAt > pinnedAt
}

func report(updates []update, changed, check bool) {
	for _, u := range updates {
		switch {
		case !u.Changed:
			fmt.Printf("up to date  %s %s\n", u.Package, u.From)
		case check:
			fmt.Printf("available   %s %s → %s\n", u.Package, u.From, u.To)
		default:
			fmt.Printf("updated     %s %s → %s (sha256 %s)\n", u.Package, u.From, u.To, u.SHA256)
		}
	}
	if !changed {
		fmt.Println("\nAll pinned metadata is current.")
	}
}

// emitGitHubOutputs writes the workflow-facing outputs when running under
// GitHub Actions; a no-op elsewhere.
func emitGitHubOutputs(updates []update, changed bool) error {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := fmt.Fprintf(file, "updated=%t\n", changed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(file, "title=%s\n", prTitle(updates)); err != nil {
		return err
	}
	// Multiline values use the heredoc form; the delimiter cannot occur in
	// the body because every line of the body starts with "- ".
	if _, err := fmt.Fprintf(file, "summary<<WINMD_UPDATE_EOF\n%s\nWINMD_UPDATE_EOF\n", prBody(updates)); err != nil {
		return err
	}
	return nil
}

func prTitle(updates []update) string {
	var moved []update
	for _, u := range updates {
		if u.Changed {
			moved = append(moved, u)
		}
	}
	if len(moved) == 1 {
		return fmt.Sprintf("chore(deps): bump %s metadata to %s", moved[0].Package, moved[0].To)
	}
	return "chore(deps): bump the pinned winmd metadata"
}

func prBody(updates []update) string {
	var lines []string
	for _, u := range updates {
		if u.Changed {
			lines = append(lines, fmt.Sprintf("- `%s` %s → **%s** (`%s`, sha256 `%s`)", u.Package, u.From, u.To, u.File, u.SHA256))
		} else {
			lines = append(lines, fmt.Sprintf("- `%s` %s — unchanged", u.Package, u.From))
		}
	}
	return strings.Join(lines, "\n")
}
