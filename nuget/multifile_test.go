package nuget

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

// The real Microsoft.WindowsAppSDK 2.3.1 dependency list: nine components,
// eight with open lower bounds and one pinned exactly. It is the shape the
// fan-out resolution has to survive, so it is the fixture.
const windowsAppSDKNuspec = `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <id>Microsoft.WindowsAppSDK</id>
    <version>2.3.1</version>
    <dependencies>
      <dependency id="Microsoft.WindowsAppSDK.Base" version="2.0.4" />
      <dependency id="Microsoft.WindowsAppSDK.Foundation" version="2.3.5" />
      <dependency id="Microsoft.WindowsAppSDK.InteractiveExperiences" version="2.1.3" />
      <dependency id="Microsoft.WindowsAppSDK.WinUI" version="2.3.0" />
      <dependency id="Microsoft.WindowsAppSDK.DWrite" version="2.1.0" />
      <dependency id="Microsoft.WindowsAppSDK.Widgets" version="2.0.5" />
      <dependency id="Microsoft.WindowsAppSDK.AI" version="2.3.4" />
      <dependency id="Microsoft.WindowsAppSDK.ML" version="2.1.74" />
      <dependency id="Microsoft.WindowsAppSDK.Runtime" version="[2.3.1]" />
    </dependencies>
  </metadata>
</package>`

func TestParseDependenciesFlat(t *testing.T) {
	dependencies, err := ParseDependencies([]byte(windowsAppSDKNuspec))
	if err != nil {
		t.Fatalf("ParseDependencies: %v", err)
	}
	if len(dependencies) != 9 {
		t.Fatalf("got %d dependencies, want 9", len(dependencies))
	}
	// Declaration order is preserved: a fetch that walks the list produces a
	// stable download order, which keeps provenance diffs reviewable.
	if dependencies[0].ID != "Microsoft.WindowsAppSDK.Base" {
		t.Errorf("first dependency = %q, want Microsoft.WindowsAppSDK.Base", dependencies[0].ID)
	}
	if dependencies[8].ID != "Microsoft.WindowsAppSDK.Runtime" {
		t.Errorf("last dependency = %q, want Microsoft.WindowsAppSDK.Runtime", dependencies[8].ID)
	}
	// The mixed range forms must both resolve; rejecting the open-bound ones
	// would reject the package.
	for _, dependency := range dependencies {
		if _, err := dependency.Version(); err != nil {
			t.Errorf("%s: Version: %v", dependency.ID, err)
		}
	}
	exact := dependencies[8]
	if got, _ := exact.Version(); got != "2.3.1" {
		t.Errorf("Runtime version = %q, want 2.3.1", got)
	}
	if got, _ := dependencies[1].Version(); got != "2.3.5" {
		t.Errorf("Foundation version = %q, want 2.3.5", got)
	}
}

func TestParseDependenciesGroups(t *testing.T) {
	const nuspec = `<?xml version="1.0"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <dependencies>
      <group targetFramework="net8.0-windows">
        <dependency id="Alpha" version="1.0.0" />
        <dependency id="Beta" version="2.0.0" />
      </group>
      <group targetFramework="native0.0">
        <dependency id="Alpha" version="1.0.0" />
      </group>
    </dependencies>
  </metadata>
</package>`
	dependencies, err := ParseDependencies([]byte(nuspec))
	if err != nil {
		t.Fatalf("ParseDependencies: %v", err)
	}
	// Alpha appears in both groups at the same version and must collapse to
	// one entry — the caller downloads packages, not framework tuples.
	if len(dependencies) != 2 {
		t.Fatalf("got %d dependencies, want 2 (deduped)", len(dependencies))
	}
	if dependencies[0].ID != "Alpha" || dependencies[0].TargetFramework != "net8.0-windows" {
		t.Errorf("got %+v, want Alpha/net8.0-windows", dependencies[0])
	}
}

func TestParseDependenciesConflictingVersions(t *testing.T) {
	const nuspec = `<?xml version="1.0"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata>
    <dependencies>
      <group targetFramework="net8.0-windows"><dependency id="Alpha" version="1.0.0" /></group>
      <group targetFramework="native0.0"><dependency id="Alpha" version="2.0.0" /></group>
    </dependencies>
  </metadata>
</package>`
	// Picking one silently would make the fetch non-obvious and the
	// provenance a lie, so disagreement is an error.
	if _, err := ParseDependencies([]byte(nuspec)); err == nil {
		t.Fatal("ParseDependencies accepted conflicting versions for one ID")
	}
}

func TestParseDependenciesNone(t *testing.T) {
	const nuspec = `<?xml version="1.0"?>
<package xmlns="http://schemas.microsoft.com/packaging/2013/05/nuspec.xsd">
  <metadata><id>Leaf</id></metadata>
</package>`
	dependencies, err := ParseDependencies([]byte(nuspec))
	if err != nil {
		t.Fatalf("ParseDependencies: %v", err)
	}
	if len(dependencies) != 0 {
		t.Fatalf("got %d dependencies, want 0", len(dependencies))
	}
}

func TestDependencyVersion(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		rng     string
		want    string
		wantErr bool
	}{
		{name: "minimum inclusive", rng: "2.3.5", want: "2.3.5"},
		{name: "exact", rng: "[2.3.1]", want: "2.3.1"},
		{name: "bounded range", rng: "[1.0.0,2.0.0)", want: "1.0.0"},
		{name: "open upper bound", rng: "[1.0.0,)", want: "1.0.0"},
		{name: "spaced", rng: " [1.0.0, 2.0.0) ", want: "1.0.0"},
		{name: "prerelease", rng: "71.0.14-preview", want: "71.0.14-preview"},
		{name: "exclusive lower bound", rng: "(1.0.0,)", wantErr: true},
		{name: "empty", rng: "", wantErr: true},
		{name: "no lower bound", rng: "[,2.0.0)", wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := Dependency{ID: "Pkg", Range: testCase.rng}.Version()
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("Version(%q) = %q, want error", testCase.rng, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Version(%q): %v", testCase.rng, err)
			}
			if got != testCase.want {
				t.Errorf("Version(%q) = %q, want %q", testCase.rng, got, testCase.want)
			}
		})
	}
}

// buildNupkg produces an in-memory zip standing in for a nupkg.
func buildNupkg(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	// Sorted-free: zip order does not matter to the readers under test, but a
	// directory entry is included to prove it is filtered out.
	if _, err := archive.Create("lib/"); err != nil {
		t.Fatalf("creating directory entry: %v", err)
	}
	for name, content := range entries {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("closing archive: %v", err)
	}
	return buffer.Bytes()
}

func TestExtractMatching(t *testing.T) {
	nupkg := buildNupkg(t, map[string]string{
		"lib/uap10.0/Microsoft.UI.Xaml.winmd": "xaml",
		"lib/uap10.0/Microsoft.UI.winmd":      "ui",
		"lib/uap10.0/Microsoft.UI.Xaml.xml":   "docs",
		"runtimes/win-x64/native/Boot.dll":    "x64",
		"runtimes/win-x86/native/Boot.dll":    "x86",
	})

	winmds, err := ExtractMatching(nupkg, func(name string) bool {
		return strings.HasSuffix(name, ".winmd")
	})
	if err != nil {
		t.Fatalf("ExtractMatching: %v", err)
	}
	if len(winmds) != 2 {
		t.Fatalf("got %d winmds, want 2: %v", len(winmds), winmds)
	}
	if string(winmds["lib/uap10.0/Microsoft.UI.Xaml.winmd"]) != "xaml" {
		t.Errorf("Microsoft.UI.Xaml.winmd content = %q", winmds["lib/uap10.0/Microsoft.UI.Xaml.winmd"])
	}

	// Keying on the full path, not the base name, is what keeps the two
	// per-architecture Boot.dll entries distinct.
	dlls, err := ExtractMatching(nupkg, func(name string) bool {
		return strings.HasSuffix(name, "Boot.dll")
	})
	if err != nil {
		t.Fatalf("ExtractMatching: %v", err)
	}
	if len(dlls) != 2 {
		t.Fatalf("got %d Boot.dll entries, want 2 (one per architecture)", len(dlls))
	}
	if string(dlls["runtimes/win-x64/native/Boot.dll"]) != "x64" ||
		string(dlls["runtimes/win-x86/native/Boot.dll"]) != "x86" {
		t.Errorf("per-architecture entries collided: %v", dlls)
	}
}

func TestExtractMatchingNoMatches(t *testing.T) {
	nupkg := buildNupkg(t, map[string]string{"lib/a.txt": "a"})
	// Matching nothing is the caller's judgement call, not an error here.
	found, err := ExtractMatching(nupkg, func(string) bool { return false })
	if err != nil {
		t.Fatalf("ExtractMatching: %v", err)
	}
	if len(found) != 0 {
		t.Fatalf("got %d entries, want 0", len(found))
	}
}

func TestExtractMatchingCorrupt(t *testing.T) {
	if _, err := ExtractMatching([]byte("not a zip"), func(string) bool { return true }); err == nil {
		t.Fatal("ExtractMatching accepted a non-zip payload")
	}
}

func TestEntryNames(t *testing.T) {
	nupkg := buildNupkg(t, map[string]string{
		"lib/uap10.0/A.winmd":  "a",
		"build/native/B.props": "b",
	})
	names, err := EntryNames(nupkg)
	if err != nil {
		t.Fatalf("EntryNames: %v", err)
	}
	// The "lib/" directory entry buildNupkg adds must not be listed.
	if len(names) != 2 {
		t.Fatalf("got %d names, want 2: %v", len(names), names)
	}
	for _, name := range names {
		if strings.HasSuffix(name, "/") {
			t.Errorf("EntryNames returned a directory entry: %q", name)
		}
	}
}

func TestProvenanceFor(t *testing.T) {
	content := []byte("winmd bytes")
	record := ProvenanceFor("Microsoft.WindowsAppSDK.WinUI", "2.3.0",
		"https://example.invalid/pkg.nupkg", "lib/uap10.0/Microsoft.UI.Xaml.winmd", content)

	if record.Package != "Microsoft.WindowsAppSDK.WinUI" || record.Version != "2.3.0" {
		t.Errorf("got %+v", record)
	}
	// File keeps the full intra-package path so a layout move shows up as a
	// reviewable provenance diff rather than vanishing.
	if record.File != "lib/uap10.0/Microsoft.UI.Xaml.winmd" {
		t.Errorf("File = %q, want the full archive path", record.File)
	}
	if want := fmt.Sprintf("%x", sha256.Sum256(content)); record.SHA256 != want {
		t.Errorf("SHA256 = %q, want %q", record.SHA256, want)
	}
	if record.Fetched == "" {
		t.Error("Fetched is empty")
	}
}

func TestNuspecURL(t *testing.T) {
	got := NuspecURL("microsoft.windowsappsdk", "2.3.1")
	want := "https://api.nuget.org/v3-flatcontainer/microsoft.windowsappsdk/2.3.1/microsoft.windowsappsdk.nuspec"
	if got != want {
		t.Errorf("NuspecURL = %q, want %q", got, want)
	}
}
