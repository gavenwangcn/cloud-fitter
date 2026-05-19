package cmdb

import (
	"encoding/json"
	"testing"
)

func TestEncodeCIFieldValueMultiDomain(t *testing.T) {
	got := encodeCIFieldValue([]string{"a.com", "b.com"})
	if got != "a.com,b.com" {
		t.Fatalf("got %q", got)
	}
}

func TestCIJSONPayloadDomainNameNotArray(t *testing.T) {
	p := ciParamsFromFields(map[string]any{
		"ci_type":     "EIP",
		"domain_name": []string{"a.com", "b.com"},
	})
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) == "" || string(raw)[0] != '{' {
		t.Fatalf("payload %s", raw)
	}
	if _, ok := p["domain_name"].([]string); ok {
		t.Fatal("domain_name must be encoded to string before marshal")
	}
	if p["domain_name"] != "a.com,b.com" {
		t.Fatalf("domain_name=%v", p["domain_name"])
	}
}

func TestIsScalarForSignRejectsStringSlice(t *testing.T) {
	if isScalarForSign([]string{"a"}) {
		t.Fatal("[]string must not be treated as scalar for API sign")
	}
}
