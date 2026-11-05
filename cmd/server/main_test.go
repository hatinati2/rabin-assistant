package main

import (
	"testing"
	"time"
)

func TestParseJalaliDateTime(t *testing.T) {
	want := time.Date(2026, time.November, 5, 8, 44, 50, 0, tehran())
	cases := []struct {
		name string
		raw  string
	}{
		{name: "Latin digits", raw: "1405/08/14 08:44:50"},
		{name: "Persian digits", raw: "۱۴۰۵/۰۸/۱۴ ۰۸:۴۴:۵۰"},
		{name: "Arabic digits and dash", raw: "١٤٠٥-٠٨-١٤ ٠٨:٤٤:٥٠"},
		{name: "dot separator", raw: "1405.08.14 08:44:50"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseJalaliDateTime(tc.raw)
			if err != nil {
				t.Fatalf("parseJalaliDateTime(%q): %v", tc.raw, err)
			}
			if !got.Equal(want) {
				t.Fatalf("got %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
			}
		})
	}
}

func TestFormatJalaliDateTime(t *testing.T) {
	input := time.Date(2026, time.November, 5, 8, 44, 50, 0, tehran())
	got := formatJalaliDateTime(input)
	want := "۱۴۰۵/۰۸/۱۴ ۰۸:۴۴:۵۰"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestParseRejectsInvalidJalaliDate(t *testing.T) {
	if _, err := parseJalaliDateTime("۱۴۰۵/۱۲/۳۰ ۰۸:۴۴:۵۰"); err == nil {
		t.Fatal("expected invalid Esfand 30 in non-leap year 1405 to be rejected")
	}
}

func TestBuildJalaliCalendarAban1405(t *testing.T) {
	cal, err := buildCalendar(1405, 8)
	if err != nil {
		t.Fatalf("buildCalendar: %v", err)
	}
	if cal.MonthName != "آبان" {
		t.Fatalf("month name = %q, want آبان", cal.MonthName)
	}
	if cal.Days != 30 {
		t.Fatalf("days = %d, want 30", cal.Days)
	}
	// 1 Aban 1405 was Friday; Saturday-based index is 6.
	if cal.FirstWeekday != 6 {
		t.Fatalf("first weekday = %d, want 6 (Friday)", cal.FirstWeekday)
	}
}

func TestCurrentJalaliParts(t *testing.T) {
	input := time.Date(2026, time.November, 5, 8, 44, 50, 0, tehran())
	y, m, d := currentJalaliParts(input)
	if y != 1405 || m != 8 || d != 14 {
		t.Fatalf("got %04d/%02d/%02d, want 1405/08/14", y, m, d)
	}
}
