package scope

import (
	"context"
	"testing"
)

func TestSystemListTagFilterMatches(t *testing.T) {
	ctx := WithSystemListTagFilter(context.Background(), "D-000027")
	if !SystemListTagFilterMatches(ctx, "") {
		t.Fatal("empty tag should match")
	}
	if !SystemListTagFilterMatches(ctx, "D-000027") {
		t.Fatal("same id should match")
	}
	if SystemListTagFilterMatches(ctx, "D-000029") {
		t.Fatal("other id should not match")
	}
	if !SystemListTagFilterMatches(context.Background(), "D-000029") {
		t.Fatal("no filter in ctx should allow any")
	}
}

func TestFilterSliceBySystemListTag(t *testing.T) {
	type row struct {
		tag string
	}
	items := []row{{"D-000027"}, {"D-000029"}, {""}}
	ctx := WithSystemListTagFilter(context.Background(), "D-000027")
	out := FilterSliceBySystemListTag(ctx, items, func(r row) string { return r.tag })
	if len(out) != 2 {
		t.Fatalf("want 2 rows, got %d", len(out))
	}
	if out[0].tag != "D-000027" || out[1].tag != "" {
		t.Fatalf("unexpected filter result: %+v", out)
	}
}
