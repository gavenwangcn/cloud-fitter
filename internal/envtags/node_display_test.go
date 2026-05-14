package envtags

import "testing"

func TestFormatNodeTagDisplay(t *testing.T) {
	if got := FormatNodeTagDisplay("华为云", "cn-east-4", "德非图"); got != "华为云-cn-east-4-德非图" {
		t.Fatalf("full: %q", got)
	}
	if got := FormatNodeTagDisplay("华为云", "cn-east-4", ""); got != "华为云-cn-east-4" {
		t.Fatalf("base only: %q", got)
	}
}
