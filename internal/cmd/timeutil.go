package cmd

import "time"

// fmtTime renders a time as local date-time; zero times render as "-".
func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// fmtTimePtr renders a *time.Time, falling back to fallback when nil.
func fmtTimePtr(t *time.Time, fallback string) string {
	if t == nil {
		return fallback
	}
	return fmtTime(*t)
}
