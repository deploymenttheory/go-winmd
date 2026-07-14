package winmd

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/deploymenttheory/go-winmd/nuget"
)

// The brute-force test suites run against the real Windows.Win32.winmd,
// pinned by testdata/PROVENANCE.json. The file itself is gitignored and
// fetched on demand (sha256-verified); offline runs skip.

var fixtureOnce sync.Once
var fixtureErr error

// testFile opens the pinned Windows.Win32.winmd, fetching it into testdata/
// on first use. Tests skip when the fixture cannot be acquired.
func testFile(t *testing.T) *File {
	t.Helper()
	records, err := nuget.ReadProvenance(filepath.Join("testdata", "PROVENANCE.json"))
	if err != nil || len(records) == 0 {
		t.Fatalf("reading testdata/PROVENANCE.json: %v", err)
	}
	pin := records[0]
	path := filepath.Join("testdata", pin.File)

	fixtureOnce.Do(func() { fixtureErr = ensureFixture(pin, path) })
	if fixtureErr != nil {
		t.Skipf("test winmd unavailable: %v", fixtureErr)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return file
}

func ensureFixture(pin nuget.Provenance, path string) error {
	if data, err := os.ReadFile(path); err == nil {
		if fmt.Sprintf("%x", sha256.Sum256(data)) == pin.SHA256 {
			return nil
		}
		// Stale or corrupt cache: refetch.
	}
	content, fetched, err := nuget.Fetch(nuget.NewClient(),
		"microsoft.windows.sdk.win32metadata", pin.Package, pin.Version, pin.File)
	if err != nil {
		return err
	}
	if fetched.SHA256 != pin.SHA256 {
		return fmt.Errorf("fetched %s sha256 %s does not match pinned %s", pin.File, fetched.SHA256, pin.SHA256)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}
