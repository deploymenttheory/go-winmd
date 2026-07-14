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

// The brute-force test suites run against real winmd files pinned by
// testdata/PROVENANCE.json: the Win32 metadata (Windows.Win32.winmd) and the
// WinRT UniversalApiContract. The files themselves are gitignored and
// fetched on demand (sha256-verified); offline runs skip.

// fixtureState serializes the fetch of one pinned fixture.
type fixtureState struct {
	once sync.Once
	err  error
}

var fixtureStates sync.Map // provenance package name → *fixtureState

// fixtureFile opens the fixture pinned by the PROVENANCE record whose
// package equals matchPackage, fetching it into testdata/ on first use.
// pkgID is the lowercase NuGet package ID used in fetch URLs. Tests skip
// when the fixture cannot be acquired.
func fixtureFile(t *testing.T, pkgID, matchPackage string) *File {
	t.Helper()
	records, err := nuget.ReadProvenance(filepath.Join("testdata", "PROVENANCE.json"))
	if err != nil || len(records) == 0 {
		t.Fatalf("reading testdata/PROVENANCE.json: %v", err)
	}
	var pin *nuget.Provenance
	for i := range records {
		if records[i].Package == matchPackage {
			pin = &records[i]
			break
		}
	}
	if pin == nil {
		t.Fatalf("no PROVENANCE record for package %q", matchPackage)
	}
	// The pin's file is the path inside the nupkg (possibly nested); the
	// local cache uses just the base name.
	path := filepath.Join("testdata", filepath.Base(pin.File))

	stateAny, _ := fixtureStates.LoadOrStore(matchPackage, &fixtureState{})
	state := stateAny.(*fixtureState)
	state.once.Do(func() { state.err = ensureFixture(pkgID, *pin, path) })
	if state.err != nil {
		t.Skipf("test winmd unavailable: %v", state.err)
	}
	file, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return file
}

// testFile opens the pinned Windows.Win32.winmd.
func testFile(t *testing.T) *File {
	t.Helper()
	return fixtureFile(t, "microsoft.windows.sdk.win32metadata", "Microsoft.Windows.SDK.Win32Metadata")
}

// testWinRTFile opens the pinned WinRT Windows.Foundation.UniversalApiContract.winmd.
func testWinRTFile(t *testing.T) *File {
	t.Helper()
	return fixtureFile(t, "microsoft.windows.sdk.contracts", "Microsoft.Windows.SDK.Contracts")
}

func ensureFixture(pkgID string, pin nuget.Provenance, path string) error {
	if data, err := os.ReadFile(path); err == nil {
		if fmt.Sprintf("%x", sha256.Sum256(data)) == pin.SHA256 {
			return nil
		}
		// Stale or corrupt cache: refetch.
	}
	content, fetched, err := nuget.Fetch(nuget.NewClient(), pkgID, pin.Package, pin.Version, pin.File)
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
