package main

import "testing"

// The flat-container index is ascending, so "newest" is a backwards scan and
// "is this ahead of the pin?" is an index comparison. Both decide whether a
// pin moves, so they are the parts worth nailing down.

func TestNewestSkipsPrereleasesForStablePins(t *testing.T) {
	versions := []string{"10.0.26100.8249", "10.0.28000.2526", "10.0.29000.1-preview"}

	got, err := newest(versions, false)
	if err != nil {
		t.Fatalf("newest(stable): %v", err)
	}
	if want := "10.0.28000.2526"; got != want {
		t.Errorf("newest(stable) = %q, want %q", got, want)
	}

	got, err = newest(versions, true)
	if err != nil {
		t.Fatalf("newest(prerelease): %v", err)
	}
	if want := "10.0.29000.1-preview"; got != want {
		t.Errorf("newest(prerelease) = %q, want %q", got, want)
	}
}

func TestNewestErrorsWhenOnlyPrereleasesExist(t *testing.T) {
	if _, err := newest([]string{"1.0.0-preview", "2.0.0-preview"}, false); err == nil {
		t.Error("newest over an all-prerelease index should error for a stable pin")
	}
}

func TestIsAhead(t *testing.T) {
	versions := []string{"70.0.1-preview", "71.0.14-preview", "71.0.15-preview"}

	cases := []struct {
		name      string
		pinned    string
		candidate string
		want      bool
	}{
		{"newer", "71.0.14-preview", "71.0.15-preview", true},
		{"same", "71.0.14-preview", "71.0.14-preview", false},
		{"older never walks the pin backwards", "71.0.14-preview", "70.0.1-preview", false},
		{"candidate absent from the index", "71.0.14-preview", "99.0.0", false},
		{"pin absent from the index counts as behind", "68.0.0-preview", "71.0.15-preview", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAhead(versions, tc.pinned, tc.candidate); got != tc.want {
				t.Errorf("isAhead(%q, %q) = %t, want %t", tc.pinned, tc.candidate, got, tc.want)
			}
		})
	}
}

func TestPRTitleNamesTheSolePackage(t *testing.T) {
	one := []update{
		{Package: "Microsoft.Windows.SDK.Contracts", From: "10.0.26100.8249", To: "10.0.28000.2526", Changed: true},
		{Package: "Microsoft.Windows.SDK.Win32Metadata", From: "71.0.14-preview", To: "71.0.14-preview"},
	}
	want := "chore(deps): bump Microsoft.Windows.SDK.Contracts metadata to 10.0.28000.2526"
	if got := prTitle(one); got != want {
		t.Errorf("prTitle = %q, want %q", got, want)
	}

	two := []update{
		{Package: "A", To: "2", Changed: true},
		{Package: "B", To: "3", Changed: true},
	}
	if got, want := prTitle(two), "chore(deps): bump the pinned winmd metadata"; got != want {
		t.Errorf("prTitle(multiple) = %q, want %q", got, want)
	}
}

// The PR body is fed to GITHUB_OUTPUT through a heredoc whose delimiter must
// not occur in the body.
func TestPRBodyCannotCollideWithTheHeredocDelimiter(t *testing.T) {
	body := prBody([]update{
		{Package: "Microsoft.Windows.SDK.Contracts", From: "1", To: "2", File: "ref/x.winmd", SHA256: "abc", Changed: true},
		{Package: "Microsoft.Windows.SDK.Win32Metadata", From: "71.0.14-preview"},
	})
	if body == "" {
		t.Fatal("prBody returned nothing")
	}
	for _, line := range splitLines(body) {
		if len(line) < 2 || line[:2] != "- " {
			t.Errorf("every body line must start with %q, got %q", "- ", line)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return append(lines, s[start:])
}
