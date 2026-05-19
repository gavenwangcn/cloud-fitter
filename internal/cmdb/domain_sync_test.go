package cmdb

import "testing"

func TestDomainSyncPlan(t *testing.T) {
	existing := map[string]string{
		"a.com": "id1",
		"b.com": "id2",
		"old.com": "id3",
	}
	toAdd, toDel := domainSyncPlan(existing, []string{"a.com", "b.com", "c.com"})
	if len(toAdd) != 1 || toAdd[0] != "c.com" {
		t.Fatalf("toAdd=%v want [c.com]", toAdd)
	}
	if len(toDel) != 1 || toDel[0] != "old.com" {
		t.Fatalf("toDel=%v want [old.com]", toDel)
	}
}

func TestDomainSyncPlanEmptyWant(t *testing.T) {
	existing := map[string]string{"x.com": "id1"}
	_, toDel := domainSyncPlan(existing, nil)
	if len(toDel) != 1 || toDel[0] != "x.com" {
		t.Fatalf("toDel=%v", toDel)
	}
}

func TestDomainSyncPlanNoChange(t *testing.T) {
	existing := map[string]string{"a.com": "id1"}
	toAdd, toDel := domainSyncPlan(existing, []string{"a.com"})
	if len(toAdd) != 0 || len(toDel) != 0 {
		t.Fatalf("add=%v del=%v", toAdd, toDel)
	}
}
