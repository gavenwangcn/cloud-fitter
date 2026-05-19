package cmdb

import (
	"fmt"
	"testing"
)

func TestIsEmptyCloudSyncValue(t *testing.T) {
	if !isEmptyCloudSyncValue(nil) {
		t.Fatal("nil")
	}
	if !isEmptyCloudSyncValue("") {
		t.Fatal("empty string")
	}
	if !isEmptyCloudSyncValue([]string{}) {
		t.Fatal("empty slice")
	}
	if isEmptyCloudSyncValue("x.com") {
		t.Fatal("non-empty string")
	}
	if isEmptyCloudSyncValue([]string{"a.com"}) {
		t.Fatal("non-empty []string")
	}
	if isEmptyCloudSyncValue(0) {
		t.Fatal("zero int is not empty")
	}
}

func TestMergeCMDBPreserveNonEmpty(t *testing.T) {
	existing := map[string]any{
		"domain_name": []string{"keep.example.com"},
		"eip_name":    "old-name",
		"private_ip":  "10.0.0.1",
	}
	want := map[string]any{
		"domain_name": []string{},
		"eip_name":    "",
		"private_ip":  "",
		"eip_status":  "&{ACTIVE}",
	}
	merged := mergeCMDBPreserveNonEmpty(want, existing)
	if fmt.Sprint(merged["domain_name"]) != fmt.Sprint(existing["domain_name"]) {
		t.Fatalf("domain_name preserved got %v", merged["domain_name"])
	}
	if merged["eip_name"] != "old-name" {
		t.Fatalf("eip_name=%v", merged["eip_name"])
	}
	if merged["private_ip"] != "10.0.0.1" {
		t.Fatalf("private_ip=%v", merged["private_ip"])
	}
	if merged["eip_status"] != "&{ACTIVE}" {
		t.Fatalf("eip_status=%v", merged["eip_status"])
	}
}
