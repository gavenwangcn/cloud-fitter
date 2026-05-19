package cmdb

import "testing"

func TestCmdbSyncEIPStatusPassthrough(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"&{ACTIVE}", "&{ACTIVE}"},
		{"&{ELB}", "&{ELB}"},
		{"  &{ELB}  ", "&{ELB}"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := cmdbSyncEIPStatus(tc.in); got != tc.want {
			t.Errorf("cmdbSyncEIPStatus(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
