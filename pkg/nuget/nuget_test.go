package nuget

import "testing"

func TestIsPrerelease(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"71.0.14-preview", true},
		{"10.0.26100.8249", false},
		{"1.0.0-rc.1", true},
		// Build metadata is not a prerelease marker (§SemVer 2.0 clause 10).
		{"1.0.0+build.5", false},
		{"1.0.0-beta+build.5", true},
	}
	for _, tc := range cases {
		if got := IsPrerelease(tc.version); got != tc.want {
			t.Errorf("IsPrerelease(%q) = %t, want %t", tc.version, got, tc.want)
		}
	}
}

func TestSourceURL(t *testing.T) {
	got := SourceURL("microsoft.windows.sdk.win32metadata", "71.0.14-preview")
	want := "https://api.nuget.org/v3-flatcontainer/microsoft.windows.sdk.win32metadata/" +
		"71.0.14-preview/microsoft.windows.sdk.win32metadata.71.0.14-preview.nupkg"
	if got != want {
		t.Errorf("SourceURL = %q, want %q", got, want)
	}
}
