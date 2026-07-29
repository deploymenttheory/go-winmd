package nuget

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// The single-file Fetch above downloads one nupkg per file. Meta-packages
// break that model twice over: the Windows App SDK fans out across nine
// component packages, and each component carries dozens of winmds. Fetching
// per file would download the same archive dozens of times, so the API here
// separates the download (FetchArchive) from the extraction
// (ExtractMatching), and adds the nuspec dependency parsing needed to
// discover the fan-out in the first place.

// maxNuspecBytes caps a nuspec read. Nuspecs are a few kilobytes; anything
// larger is a redirect to something unexpected, and XML unmarshalling of an
// unbounded response is not something to do on a whim.
const maxNuspecBytes = 4 << 20

// FetchArchive downloads a nupkg and returns its raw bytes together with the
// flat-container URL it came from (pkg is the lowercase package ID). Callers
// pulling several entries out of one package use this with ExtractMatching
// instead of calling Fetch once per file.
func FetchArchive(client *http.Client, pkg, version string) ([]byte, string, error) {
	sourceURL := SourceURL(pkg, version)
	nupkg, err := httpGet(client, sourceURL)
	if err != nil {
		return nil, "", err
	}
	return nupkg, sourceURL, nil
}

// ExtractMatching returns every entry whose name satisfies match, keyed by
// the entry's full path within the archive. Paths are used as keys rather
// than base names because a nupkg routinely carries the same base name under
// several architectures (runtimes/win-x64/native/X.dll alongside
// runtimes/win-x86/native/X.dll); collapsing them would silently drop one.
// An entry matching nothing is not an error — the caller decides whether an
// empty result is a problem.
func ExtractMatching(nupkg []byte, match func(name string) bool) (map[string][]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(nupkg), int64(len(nupkg)))
	if err != nil {
		return nil, fmt.Errorf("opening nupkg: %w", err)
	}
	found := map[string][]byte{}
	for _, file := range archive.File {
		if file.FileInfo().IsDir() || !match(file.Name) {
			continue
		}
		content, err := readZipEntry(file)
		if err != nil {
			return nil, fmt.Errorf("reading %s from nupkg: %w", file.Name, err)
		}
		found[file.Name] = content
	}
	return found, nil
}

// EntryNames lists every file entry in a nupkg. It backs the generators'
// package-layout diagnostics: intra-package paths have moved between Windows
// App SDK releases (lib/win10-x64 → lib/native/x64 → runtimes/win-x64/native),
// and guessing is worse than looking.
func EntryNames(nupkg []byte) ([]string, error) {
	archive, err := zip.NewReader(bytes.NewReader(nupkg), int64(len(nupkg)))
	if err != nil {
		return nil, fmt.Errorf("opening nupkg: %w", err)
	}
	names := make([]string, 0, len(archive.File))
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			continue
		}
		names = append(names, file.Name)
	}
	return names, nil
}

// ProvenanceFor builds the record for one entry pulled out of an archive that
// FetchArchive already downloaded. Fetch records provenance itself; this is
// the multi-file equivalent, so a caller extracting thirty winmds from one
// nupkg writes thirty records against a single download.
func ProvenanceFor(displayName, version, sourceURL, file string, content []byte) Provenance {
	return Provenance{
		Package: displayName,
		Version: version,
		Source:  sourceURL,
		File:    file,
		SHA256:  fmt.Sprintf("%x", sha256.Sum256(content)),
		Fetched: time.Now().UTC().Format("2006-01-02"),
	}
}

// Dependency is one entry from a nuspec's <dependencies>.
type Dependency struct {
	// ID is the dependency's package ID in its canonical casing.
	ID string
	// Range is the version constraint exactly as the nuspec wrote it. NuGet
	// admits several forms, and a meta-package may mix them within one
	// dependency list: Microsoft.WindowsAppSDK 2.3.1 pins its Runtime
	// component exactly ("[2.3.1]") while giving the other eight open lower
	// bounds ("2.3.5"). Treating a non-bracketed version as malformed would
	// reject that package outright.
	Range string
	// TargetFramework is the enclosing <group>'s framework, empty when the
	// dependency was declared outside any group.
	TargetFramework string
}

// Version resolves the range to the single concrete version a deterministic
// fetch should pin: the lower bound, which NuGet guarantees is a published
// version for both the exact ("[1.2.3]") and minimum-inclusive ("1.2.3")
// forms. An exclusive lower bound ("(1.2.3,)") has no resolvable pin without
// consulting the version index, so it is reported rather than guessed at.
func (d Dependency) Version() (string, error) {
	raw := strings.TrimSpace(d.Range)
	if raw == "" {
		return "", fmt.Errorf("dependency %s declares no version", d.ID)
	}
	if !strings.ContainsAny(raw, "[](),") {
		return raw, nil
	}
	if strings.HasPrefix(raw, "(") {
		return "", fmt.Errorf("dependency %s has an exclusive lower bound %q; resolve it against the version index", d.ID, raw)
	}
	inner := strings.TrimPrefix(raw, "[")
	inner = strings.TrimSuffix(strings.TrimSuffix(inner, "]"), ")")
	lower := strings.TrimSpace(inner)
	if comma := strings.Index(lower, ","); comma >= 0 {
		lower = strings.TrimSpace(lower[:comma])
	}
	if lower == "" {
		return "", fmt.Errorf("dependency %s has no lower bound in %q", d.ID, raw)
	}
	return lower, nil
}

// NuspecURL is the flat-container nuspec URL for a package version.
func NuspecURL(pkg, version string) string {
	return fmt.Sprintf("https://api.nuget.org/v3-flatcontainer/%s/%s/%s.nuspec", pkg, version, pkg)
}

// Dependencies fetches a package's nuspec and returns its declared
// dependencies, so a generator can discover a meta-package's fan-out instead
// of hard-coding it — the Windows App SDK's component set and versions change
// with every servicing release.
//
// Both nuspec shapes are handled: <dependency> elements directly under
// <dependencies>, and elements nested in per-framework <group>s. A dependency
// declared in several groups is returned once; if the groups disagree on the
// version, that is an error rather than a silent pick.
func Dependencies(client *http.Client, pkg, version string) ([]Dependency, error) {
	nuspecURL := NuspecURL(pkg, version)
	data, err := httpGetLimit(client, nuspecURL, maxNuspecBytes)
	if err != nil {
		return nil, err
	}
	return ParseDependencies(data)
}

// ParseDependencies extracts the dependency list from raw nuspec XML.
func ParseDependencies(nuspec []byte) ([]Dependency, error) {
	var document struct {
		Metadata struct {
			Dependencies struct {
				Dependencies []nuspecDependency `xml:"dependency"`
				Groups       []struct {
					TargetFramework string             `xml:"targetFramework,attr"`
					Dependencies    []nuspecDependency `xml:"dependency"`
				} `xml:"group"`
			} `xml:"dependencies"`
		} `xml:"metadata"`
	}
	if err := xml.Unmarshal(nuspec, &document); err != nil {
		return nil, fmt.Errorf("parsing nuspec: %w", err)
	}

	var ordered []Dependency
	seen := map[string]int{} // lowercased ID → index in ordered
	add := func(dependency nuspecDependency, framework string) error {
		if dependency.ID == "" {
			return nil
		}
		key := strings.ToLower(dependency.ID)
		if at, ok := seen[key]; ok {
			if ordered[at].Range != dependency.Version {
				return fmt.Errorf("nuspec declares %s at both %q and %q",
					dependency.ID, ordered[at].Range, dependency.Version)
			}
			return nil
		}
		seen[key] = len(ordered)
		ordered = append(ordered, Dependency{
			ID:              dependency.ID,
			Range:           dependency.Version,
			TargetFramework: framework,
		})
		return nil
	}

	for _, dependency := range document.Metadata.Dependencies.Dependencies {
		if err := add(dependency, ""); err != nil {
			return nil, err
		}
	}
	for _, group := range document.Metadata.Dependencies.Groups {
		for _, dependency := range group.Dependencies {
			if err := add(dependency, group.TargetFramework); err != nil {
				return nil, err
			}
		}
	}
	return ordered, nil
}

type nuspecDependency struct {
	ID      string `xml:"id,attr"`
	Version string `xml:"version,attr"`
}

// readZipEntry reads one archive entry, closing it on every path.
func readZipEntry(file *zip.File) ([]byte, error) {
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

// httpGetLimit is httpGet with a ceiling on the response body.
func httpGetLimit(client *http.Client, url string, limit int64) ([]byte, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: response exceeds %d bytes", url, limit)
	}
	return data, nil
}
