package main

import (
	"testing"
	"time"
)

func TestDailySchedule(t *testing.T) {
	zone := time.FixedZone("fixture", 8*3600)
	for _, test := range []struct{ hour, minute, day int }{{2, 59, 30}, {3, 0, 31}, {23, 59, 31}} {
		now := time.Date(2026, 10, 30, test.hour, test.minute, 0, 0, zone)
		next := nextDaily(now)
		if next.Day() != test.day || next.Hour() != 3 || next.Minute() != 0 || next.Location() != zone || !next.After(now) {
			t.Fatal("daily schedule")
		}
	}
	// Calendar days, not 24-hour offsets, preserve 03:00 across timezone changes.
	zone, e := time.LoadLocation("Europe/Copenhagen")
	if e != nil {
		t.Fatal(e)
	}
	before := time.Date(2026, 10, 24, 4, 0, 0, 0, zone)
	if next := nextDaily(before); next.Day() != 25 || next.Hour() != 3 {
		t.Fatal("DST calendar day")
	}
}
