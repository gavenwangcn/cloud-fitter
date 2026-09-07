package billing

import (
	"reflect"
	"testing"
)

func TestMonthRangeInclusive(t *testing.T) {
	got, err := MonthRangeInclusive("2026-01", "2026-03")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"2026-01", "2026-02", "2026-03"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMonthRangeInclusive_CrossYear(t *testing.T) {
	got, err := MonthRangeInclusive("2025-11", "2026-01")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := []string{"2025-11", "2025-12", "2026-01"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestMonthRangeInclusive_StartAfterEnd(t *testing.T) {
	_, err := MonthRangeInclusive("2026-05", "2026-01")
	if err == nil {
		t.Fatal("expected error when start > end")
	}
}

func TestMonthRangeInclusive_BadFormat(t *testing.T) {
	_, err := MonthRangeInclusive("2026-1", "2026-02")
	if err == nil {
		t.Fatal("expected error for bad format")
	}
}
