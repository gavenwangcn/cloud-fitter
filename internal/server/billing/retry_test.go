package billing

import (
	"errors"
	"testing"
)

func TestIsTimeoutErr(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("something else"), false},
		{errors.New("net/http: request canceled (Client.Timeout exceeded while awaiting headers)"), true},
		{errors.New("i/o timeout"), true},
		{errors.New("context deadline exceeded"), true},
		{errors.New("Timeout: read tcp"), true},
	}
	for _, c := range cases {
		if got := IsTimeoutErr(c.err); got != c.want {
			t.Errorf("IsTimeoutErr(%v)=%v want %v", c.err, got, c.want)
		}
	}
}
