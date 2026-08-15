package version

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		stamped   string
		buildInfo string
		want      string
	}{
		{"stamped wins", "0.2.0", "v0.1.1", "0.2.0"},
		{"stamped v prefix normalized", "v0.2.0", "", "0.2.0"},
		{"buildinfo when unstamped", "dev", "v0.1.1", "0.1.1"},
		{"buildinfo when empty stamp", "", "v0.1.1", "0.1.1"},
		{"devel buildinfo maps to dev", "dev", "(devel)", "dev"},
		{"dirty buildinfo maps to dev", "dev", "v0.1.2-0.20260815104549-a99bcc5c26aa+dirty", "dev"},
		{"empty buildinfo maps to dev", "dev", "", "dev"},
		{"both empty maps to dev", "", "", "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.stamped, tt.buildInfo); got != tt.want {
				t.Errorf("resolve(%q, %q) = %q, want %q", tt.stamped, tt.buildInfo, got, tt.want)
			}
		})
	}
}
