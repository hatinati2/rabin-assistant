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
