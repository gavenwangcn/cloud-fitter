package billingbatchexport

import (
	"path/filepath"
	"testing"
)

func TestResolveFile_RejectsTraversal(t *testing.T) {
	_, err := ResolveFile("../secret.xlsx")
	if err == nil {
		t.Fatal("expected error for traversal")
	}
	_, err = ResolveFile("ok.xlsx")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	abs, err := ResolveFile("ok.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(abs) != "ok.xlsx" {
		t.Fatalf("base=%q", filepath.Base(abs))
	}
}
