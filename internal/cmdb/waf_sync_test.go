package cmdb

import (
	"testing"

	"github.com/cloud-fitter/cloud-fitter/internal/configstore"
	"github.com/cloud-fitter/cloud-fitter/internal/server/cert"
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

func TestCmdbDateFromHuaweiTime(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"2026-05-19", "2026-05-19"},
		{"2026-05-19 23:59:59", "2026-05-19"},
		{"2026-05-19T23:59:59.000Z", "2026-05-19"},
	}
	for _, tc := range tests {
		got := cmdbDateFromHuaweiTime(tc.in)
		if got != tc.want {
			t.Errorf("cmdbDateFromHuaweiTime(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCertValidToForCMDB(t *testing.T) {
	c := &cert.Instance{
		ExpireTime: "2026-01-01",
		NotAfter:   "2026-12-31",
	}
	if got := certValidToForCMDB(c); got != "2026-12-31" {
		t.Fatalf("prefer not_after, got %q", got)
	}
	c.NotAfter = ""
	if got := certValidToForCMDB(c); got != "2026-01-01" {
		t.Fatalf("fallback expire_time, got %q", got)
	}
}

func TestIntersectAccountNames(t *testing.T) {
	linked := []configstore.Row{{Name: "Infra"}, {Name: "Other"}}
	got := intersectAccountNames([]string{"Infra", "X"}, linked)
	if len(got) != 1 || got[0] != "Infra" {
		t.Fatalf("got %v", got)
	}
}
