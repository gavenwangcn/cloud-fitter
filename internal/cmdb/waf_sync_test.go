package cmdb

import (
	"testing"

	"github.com/cloud-fitter/cloud-fitter/internal/server/cert"
	"github.com/cloud-fitter/cloud-fitter/internal/server/eip"
	"github.com/cloud-fitter/cloud-fitter/internal/server/waf"
	"github.com/cloud-fitter/cloud-fitter/internal/wafbind"
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
		if got := wafbind.ParseOriginIP(tc.in); got != tc.want {
			t.Errorf("ParseOriginIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseWAFOriginIPs(t *testing.T) {
	got := wafbind.ParseOriginIPs("1.2.3.4:80, 5.6.7.8:443, 1.2.3.4:8080")
	if len(got) != 2 {
		t.Fatalf("got %v, want 2 unique IPs", got)
	}
	if got[0] != "1.2.3.4" || got[1] != "5.6.7.8" {
		t.Fatalf("got %v", got)
	}
}

func TestMergeDomainNames(t *testing.T) {
	merged := mergeDomainNames([]string{"a.com", "b.com"}, []string{"b.com", "c.com"})
	want := []string{"a.com", "b.com", "c.com"}
	if !domainNamesEqual(merged, want) {
		t.Fatalf("merge got %v want %v", merged, want)
	}
}

func TestParseCMDBDomainNamesMultiValue(t *testing.T) {
	row := map[string]any{"domain_name": []any{"a.com", "b.com"}}
	got := parseCMDBDomainNames(row)
	if !domainNamesEqual(got, []string{"a.com", "b.com"}) {
		t.Fatalf("got %v", got)
	}
	row2 := map[string]any{"domain_name": "a.com、b.com"}
	got2 := parseCMDBDomainNames(row2)
	if !domainNamesEqual(got2, []string{"a.com", "b.com"}) {
		t.Fatalf("got2 %v", got2)
	}
}

func TestDomainNamesForCMDBIsSlice(t *testing.T) {
	got := domainNamesForCMDB([]string{"x.com", "y.com"})
	if len(got) != 2 || got[0] != "x.com" {
		t.Fatalf("got %v", got)
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
	got := wafbind.IntersectAccountNames([]string{"Infra", "X"}, []string{"Infra", "Other"})
	if len(got) != 1 || got[0] != "Infra" {
		t.Fatalf("got %v", got)
	}
}

func TestFilterWAFRowsByAccount(t *testing.T) {
	rows := []*waf.Instance{
		{AccountName: "DFT_LC_infra", Hostname: "a.com"},
		{AccountName: "DFT_LC_BPM", Hostname: "b.com"},
	}
	got := wafbind.FilterWAFRows(rows, []string{"DFT_LC_infra"})
	if len(got) != 1 || got[0].Hostname != "a.com" {
		t.Fatalf("got %v", got)
	}
}

func TestFilterCertRowsByAccount(t *testing.T) {
	rows := []*cert.Instance{
		{AccountName: "infra", Name: "c1"},
		{AccountName: "bpm", Name: "c2"},
	}
	got := wafbind.FilterCertRows(rows, []string{"infra"})
	if len(got) != 1 || got[0].Name != "c1" {
		t.Fatalf("got %v", got)
	}
}

func TestCertificateCMDBUUID(t *testing.T) {
	got := certificateCMDBUUID("scs1768986141935", "D-000011")
	want := "scs1768986141935|D-000011"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if certificateCMDBUUID("scs", "") != "" {
		t.Fatal("missing system_id should yield empty uuid")
	}
}

func TestBuildCrossAccountWAFToEIP(t *testing.T) {
	wafRows := []*waf.Instance{{
		AccountName:     "DFT_LC_infra",
		Hostname:        "app.example.com",
		OriginServers:   "1.2.3.4:443",
		CertificateName: "my-cert",
	}}
	eips := []*eip.Instance{{
		AccountName:  "DFT_LC_BPM",
		Eip:          "1.2.3.4",
		EipId:        "eip-uuid-1",
		RegionName:   "cn-east-4",
		NodeTagValue: "node-a",
		Provider:     "huawei",
	}}
	bind := wafbind.Build(eips, wafRows, []string{"DFT_LC_infra"})
	if len(bind.EIPDomains) != 1 {
		t.Fatalf("EIPDomains=%d want 1", len(bind.EIPDomains))
	}
	if bind.EIPDomains[0].Domains[0] != "app.example.com" {
		t.Fatalf("domains=%v", bind.EIPDomains[0].Domains)
	}
	if len(bind.CertJobs) != 1 || bind.CertJobs[0].AccountName != "DFT_LC_infra" {
		t.Fatalf("cert jobs=%v", bind.CertJobs)
	}
}
