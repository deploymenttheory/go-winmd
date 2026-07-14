// Package nuget downloads winmd files from NuGet packages via the
// flat-container API. Standard library only. It serves the bindings
// generators' fetch-metadata commands and go-winmd's own test fixture.
package nuget

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Provenance records where a fetched file came from — the committed
// PROVENANCE.json entry shape used across the bindings repos.
type Provenance struct {
	Package string `json:"package"`
	Version string `json:"version"`
	Source  string `json:"source"`
	File    string `json:"file"`
	SHA256  string `json:"sha256"`
	Fetched string `json:"fetched"`
}

// NewClient returns an HTTP client with a timeout suited to nupkg downloads.
func NewClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Minute}
}

// LatestVersion resolves the newest published version of the package
// (NuGet flat-container index; lowercase package ID).
func LatestVersion(client *http.Client, pkg string) (string, error) {
	indexURL := fmt.Sprintf("https://api.nuget.org/v3-flatcontainer/%s/index.json", pkg)
	data, err := httpGet(client, indexURL)
	if err != nil {
		return "", err
	}
	var index struct {
		Versions []string `json:"versions"`
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return "", fmt.Errorf("parsing NuGet version index: %w", err)
	}
	if len(index.Versions) == 0 {
		return "", fmt.Errorf("NuGet index lists no versions for %s", pkg)
	}
	// The flat-container index is ordered oldest → newest.
	return index.Versions[len(index.Versions)-1], nil
}

// SourceURL is the flat-container nupkg URL for a package version.
func SourceURL(pkg, version string) string {
	return fmt.Sprintf("https://api.nuget.org/v3-flatcontainer/%s/%s/%s.%s.nupkg",
		pkg, version, pkg, version)
}

// Fetch downloads the nupkg (pkg is the lowercase package ID, displayName the
// canonical casing recorded in provenance) and extracts fileName from it.
func Fetch(client *http.Client, pkg, displayName, version, fileName string) ([]byte, Provenance, error) {
	sourceURL := SourceURL(pkg, version)
	nupkg, err := httpGet(client, sourceURL)
	if err != nil {
		return nil, Provenance{}, err
	}
	content, err := ExtractFile(nupkg, fileName)
	if err != nil {
		return nil, Provenance{}, err
	}
	return content, Provenance{
		Package: displayName,
		Version: version,
		Source:  sourceURL,
		File:    fileName,
		SHA256:  fmt.Sprintf("%x", sha256.Sum256(content)),
		Fetched: time.Now().UTC().Format("2006-01-02"),
	}, nil
}

// ExtractFile pulls the named entry out of a nupkg (a zip).
func ExtractFile(nupkg []byte, name string) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(nupkg), int64(len(nupkg)))
	if err != nil {
		return nil, fmt.Errorf("opening nupkg: %w", err)
	}
	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	return nil, fmt.Errorf("nupkg contains no %s", name)
}

// ReadProvenance loads a PROVENANCE.json holding one or more records
// (a single object is accepted for compatibility).
func ReadProvenance(path string) ([]Provenance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []Provenance
	if err := json.Unmarshal(data, &records); err == nil {
		return records, nil
	}
	var single Provenance
	if err := json.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return []Provenance{single}, nil
}

// WriteProvenance writes records as the committed PROVENANCE.json array.
func WriteProvenance(path string, records []Provenance) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func httpGet(client *http.Client, url string) ([]byte, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, response.Status)
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	return data, nil
}
