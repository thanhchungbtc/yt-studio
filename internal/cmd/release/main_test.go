package main

import "testing"

func TestNextVersion(t *testing.T) {
	cases := []struct {
		latest, bump, explicit, want string
		wantErr                      bool
	}{
		{"", "patch", "", "0.1.0", false},
		{"0.1.3", "patch", "", "0.1.4", false},
		{"0.1.3", "minor", "", "0.2.0", false},
		{"0.1.3", "major", "", "1.0.0", false},
		{"0.1.9", "patch", "", "0.1.10", false},
		{"0.1.3", "huge", "", "", true},
		{"0.1.3", "patch", "0.3.0", "0.3.0", false},
		{"0.1.3", "patch", "v0.3.0", "0.3.0", false},
		{"0.1.3", "patch", "0.1.3", "", true},
		{"0.1.3", "patch", "0.1", "", true},
		{"", "patch", "2.0.0", "2.0.0", false},
	}
	for _, c := range cases {
		got, err := nextVersion(c.latest, c.bump, c.explicit)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("nextVersion(%q, %q, %q) = %q, %v; want %q (error %v)", c.latest, c.bump, c.explicit, got, err, c.want, c.wantErr)
		}
	}
}
