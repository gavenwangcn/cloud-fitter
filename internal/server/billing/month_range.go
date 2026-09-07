package billing

import (
	"strings"
	"time"

	"github.com/pkg/errors"
)

func ParseYearMonth(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01", s, loc)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "invalid month %q, want YYYY-MM", s)
	}
	return t, nil
}

// MonthRangeInclusive 返回 [start, end] 闭区间内每个 YYYY-MM（升序）。
func MonthRangeInclusive(start, end string) ([]string, error) {
	st, err := ParseYearMonth(start)
	if err != nil {
		return nil, err
	}
	en, err := ParseYearMonth(end)
	if err != nil {
		return nil, err
	}
	if st.After(en) {
		return nil, errors.Errorf("startMonth %s after endMonth %s", start, end)
	}
	var out []string
	for cur := st; !cur.After(en); cur = cur.AddDate(0, 1, 0) {
		out = append(out, cur.Format("2006-01"))
	}
	return out, nil
}
