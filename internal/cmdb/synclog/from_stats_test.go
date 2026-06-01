package synclog

import (
	"testing"
	"time"
)

func TestBuildRunSummary(t *testing.T) {
	started := time.Date(2026, 5, 22, 2, 0, 0, 0, time.UTC)
	finished := started.Add(5 * time.Minute)
	systems := []SystemDetail{
		{
			SystemID:   "sys-1",
			SystemName: "A",
			Status:     "success",
		},
		{
			SystemID:   "sys-2",
			SystemName: "B",
			Status:     "failed",
			Resources: map[string]ResourceFailure{
				"ECS": {FailCount: 1, FailedIDs: []string{"ecs-1"}},
				"RDS": {FailCount: 2, FailedIDs: []string{"rds-1", "rds-2"}},
			},
		},
	}
	summary := BuildRunSummary(TriggerScheduled, "", "", started, finished, systems)
	if summary.TotalSystems != 2 {
		t.Fatalf("total=%d want 2", summary.TotalSystems)
	}
	if summary.SuccessSystems != 1 || summary.FailedSystems != 1 {
		t.Fatalf("success=%d failed=%d", summary.SuccessSystems, summary.FailedSystems)
	}
	if summary.Status != "partial" {
		t.Fatalf("status=%q want partial", summary.Status)
	}
	ecs := summary.ResourceTotals["ECS"]
	if ecs.FailCount != 1 || len(ecs.Items) != 1 {
		t.Fatalf("ecs totals=%v", ecs)
	}
	if ecs.Items[0].SystemName != "B" || ecs.Items[0].FailedIDs[0] != "ecs-1" {
		t.Fatalf("ecs items=%v", ecs.Items)
	}
	rds := summary.ResourceTotals["RDS"]
	if rds.FailCount != 2 || len(rds.Items) != 1 || len(rds.Items[0].FailedIDs) != 2 {
		t.Fatalf("rds totals=%v", rds)
	}
}

func TestParseResourceTotalsJSONLegacy(t *testing.T) {
	got := parseResourceTotalsJSON(`{"ECS":3,"RDS":2}`)
	if got["ECS"].FailCount != 3 || got["RDS"].FailCount != 2 {
		t.Fatalf("legacy=%v", got)
	}
	if len(got["ECS"].Items) != 0 {
		t.Fatalf("legacy should have no items")
	}
}

func TestParseResourceTotalsJSONDetailed(t *testing.T) {
	raw := `{"ECS":{"fail_count":1,"items":[{"system_id":"s1","system_name":"A","fail_count":1,"failed_ids":["id-1"]}]}}`
	got := parseResourceTotalsJSON(raw)
	if got["ECS"].FailCount != 1 || len(got["ECS"].Items) != 1 || got["ECS"].Items[0].FailedIDs[0] != "id-1" {
		t.Fatalf("detailed=%v", got)
	}
}

func TestResourceFailureWithReasons(t *testing.T) {
	stats := SystemStats{
		EIP: ComponentStats{
			Errors: 1,
			Failures: []FailureEntry{{
				ResourceID: "eip-1",
				Reason:     "CMDB AddCI EIP: duplicate uuid",
			}},
		},
	}
	detail := SystemDetailFrom("sid", "name", "failed", "", stats)
	rf := detail.Resources["EIP"]
	if len(rf.Failures) != 1 || rf.Failures[0].Reason == "" {
		t.Fatalf("failures=%v", rf.Failures)
	}
}

func TestSystemResourcesMiddlewareTypes(t *testing.T) {
	stats := SystemStats{
		MiddlewareByType: map[string]ComponentStats{
			"RDS_INS":      {Errors: 1, FailedIDs: []string{"rds-a"}},
			"DCS_REDIS":    {Errors: 1, FailedIDs: []string{"dcs-a"}},
			"DMS_ROCKETMQ": {Errors: 2, FailedIDs: []string{"kafka-a", "kafka-b"}},
		},
	}
	detail := SystemDetailFrom("sid", "name", "failed", "", stats)
	if detail.Resources["RDS"].FailCount != 1 {
		t.Fatalf("rds=%v", detail.Resources["RDS"])
	}
	if detail.Resources["DCS"].FailCount != 1 {
		t.Fatalf("dcs=%v", detail.Resources["DCS"])
	}
	if detail.Resources["Kafka"].FailCount != 2 {
		t.Fatalf("kafka=%v", detail.Resources["Kafka"])
	}
}
