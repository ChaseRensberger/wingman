package trigger

import (
	"testing"
	"time"
)

func TestCronNext(t *testing.T) {
	for _, test := range []struct{ name, expression, zone, after, want string }{
		{"weekdays", "0 8 * * 1-5", "UTC", "2026-09-18T08:00:00Z", "2026-09-21T08:00:00Z"},
		{"timezone", "0 8 * * *", "America/New_York", "2026-09-16T00:00:00Z", "2026-09-16T12:00:00Z"},
		{"spring gap", "30 2 * * *", "America/New_York", "2026-03-08T05:00:00Z", "2026-03-09T06:30:00Z"},
		{"fall first", "30 1 * * *", "America/New_York", "2026-11-01T04:00:00Z", "2026-11-01T05:30:00Z"},
		{"fall repeated hour", "30 1 * * *", "America/New_York", "2026-11-01T05:30:00Z", "2026-11-01T06:30:00Z"},
		{"minute precision", "*/15 * * * *", "UTC", "2026-09-16T10:00:01Z", "2026-09-16T10:15:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			after, err := time.Parse(time.RFC3339, test.after)
			if err != nil {
				t.Fatal(err)
			}
			next, err := (Source{Type: "cron", Expression: test.expression, TimeZone: test.zone}).Next(after)
			if err != nil || next.Format(time.RFC3339) != test.want {
				t.Fatalf("next = %s, %v; want %s", next, err, test.want)
			}
		})
	}
}

func TestCronRejectsInvalidSources(t *testing.T) {
	for _, source := range []Source{
		{Type: "webhook", Expression: "* * * * *", TimeZone: "UTC"},
		{Type: "cron", Expression: "* * * * *", TimeZone: "Local"},
		{Type: "cron", Expression: "* * * * *", TimeZone: "invalid/zone"},
		{Type: "cron", Expression: "* * * * * *", TimeZone: "UTC"},
		{Type: "cron", Expression: "@every 1s", TimeZone: "UTC"},
		{Type: "cron", Expression: "0 0 31 2 *", TimeZone: "UTC"},
		{Type: "cron", Expression: "60 * * * *", TimeZone: "UTC"},
	} {
		t.Run(source.Type+source.Expression+source.TimeZone, func(t *testing.T) {
			if _, err := source.Next(time.Now()); err == nil {
				t.Fatal("accepted invalid source")
			}
		})
	}
}
