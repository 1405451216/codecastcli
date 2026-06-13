package version

import "testing"

func TestFullVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		commit    string
		buildDate string
		want      string
	}{
		{
			name:      "默认值",
			version:   "0.1.0",
			commit:    "unknown",
			buildDate: "unknown",
			want:      "codecast v0.1.0 (commit: unknown, built: unknown)",
		},
		{
			name:      "构建时注入",
			version:   "0.2.0",
			commit:    "abc1234",
			buildDate: "2026-06-12",
			want:      "codecast v0.2.0 (commit: abc1234, built: 2026-06-12)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origVersion := Version
			origCommit := GitCommit
			origBuildDate := BuildDate
			defer func() {
				Version = origVersion
				GitCommit = origCommit
				BuildDate = origBuildDate
			}()

			Version = tt.version
			GitCommit = tt.commit
			BuildDate = tt.buildDate

			got := FullVersion()
			if got != tt.want {
				t.Errorf("FullVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "默认版本",
			version: "0.1.0",
			want:    "v0.1.0",
		},
		{
			name:    "其他版本",
			version: "1.2.3",
			want:    "v1.2.3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origVersion := Version
			defer func() { Version = origVersion }()

			Version = tt.version

			got := ShortVersion()
			if got != tt.want {
				t.Errorf("ShortVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionDefaults(t *testing.T) {
	if Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", Version, "0.1.0")
	}
	if GitCommit != "unknown" {
		t.Errorf("GitCommit = %q, want %q", GitCommit, "unknown")
	}
	if BuildDate != "unknown" {
		t.Errorf("BuildDate = %q, want %q", BuildDate, "unknown")
	}
}

func TestFullVersionFormat(t *testing.T) {
	got := FullVersion()
	prefix := "codecast v"
	if len(got) < len(prefix) || got[:len(prefix)] != prefix {
		t.Errorf("FullVersion() should start with %q, got %q", prefix, got)
	}
}

func TestShortVersionFormat(t *testing.T) {
	got := ShortVersion()
	if got[0] != 'v' {
		t.Errorf("ShortVersion() should start with 'v', got %q", got)
	}
}
