package log

import "time"

// FormatRFC3339 returns the given time in UTC with RFC3999Nano format.
func FormatRFC3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
