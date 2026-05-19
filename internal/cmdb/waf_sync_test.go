package cmdb

import (
	"testing"

	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
)

func TestParseWAFOriginIP(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"1.2.3.4:8080", "1.2.3.4"},
		{"1.2.3.4", "1.2.3.4"},
		{" 10.0.0.1:443 ", "10.0.0.1"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := parseWAFOriginIP(tc.in); got != tc.want {
			t.Errorf("parseWAFOriginIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseWAFOriginIPs(t *testing.T) {
	got := parseWAFOriginIPs("1.2.3.4:80, 5.6.7.8:443, 1.2.3.4:8080")
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 unique IPs", got)
	}
	if got[0] != "1.2.3.4" || got[1] != "5.6.7.8" {
		t.Fatalf("got %v", got)
	}
}

func TestMergeDomainNames(t *testing.T) {
	merged := mergeDomainNames("a.com、b.com", []string{"b.com", "c.com"})
	want := joinDomainNamesForCMDB([]string{"a.com", "b.com", "c.com"})
	if joinDomainNamesForCMDB(merged) != want {
		t.Fatalf("merge got %q want %q", joinDomainNamesForCMDB(merged), want)
	}
}

func TestIntersectAccountNames(t *testing.T) {
	linked := []configstore.Row{{Name: "Infra"}, {Name: "Other"}}
	got := intersectAccountNames([]string{"Infra", "X"}, linked)
	if len(got) != 1 || got[0] != "Infra" {
		t.Fatalf("got %v", got)
	}
}
