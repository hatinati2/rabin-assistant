package main

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseJalaliDateTime(t *testing.T) {
	base := time.Date(2026, time.November, 5, 8, 44, 50, 0, tehran())
	cases := []struct {
		name string
		raw  string
		want time.Time
	}{
		{name: "Latin digits", raw: "1405/08/14 08:44:50", want: base},
		{name: "Persian digits", raw: "۱۴۰۵/۰۸/۱۴ ۰۸:۴۴:۵۰", want: base},
		{name: "Arabic digits and dash", raw: "١٤٠٥-٠٨-١٤ ٠٨:٤٤:٥٠", want: base},
		{name: "dot separator", raw: "1405.08.14 08:44:50", want: base},
		{name: "milliseconds", raw: "1405/08/14 08:44:50.125", want: base.Add(125 * time.Millisecond)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseJalaliDateTime(tc.raw)
			if err != nil {
				t.Fatalf("parseJalaliDateTime(%q): %v", tc.raw, err)
			}
			if !got.Equal(tc.want) {
				t.Fatalf("got %s, want %s", got.Format(time.RFC3339Nano), tc.want.Format(time.RFC3339Nano))
			}
		})
	}
}

func TestFormatJalaliDateTime(t *testing.T) {
	input := time.Date(2026, time.November, 5, 8, 44, 50, 125*int(time.Millisecond), tehran())
	got := formatJalaliDateTime(input)
	want := "۱۴۰۵/۰۸/۱۴ ۰۸:۴۴:۵۰٫۱۲۵"
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

func TestActiveAndHistorySnapshots(t *testing.T) {
	base := time.Date(2026, time.November, 5, 8, 44, 50, 0, tehran())
	store := &Store{orders: []Order{
		{ID: "scheduled", Status: statusScheduled, ExecuteAt: base.Add(time.Hour)},
		{ID: "ready", Status: statusReady, ExecuteAt: base},
		{ID: "cancelled", Status: statusCancelled, ExecuteAt: base.Add(30 * time.Minute)},
	}}

	active := store.activeSnapshot()
	if len(active) != 1 || active[0].ID != "scheduled" {
		t.Fatalf("active orders = %#v, want only scheduled order", active)
	}

	history := store.historySnapshot()
	if len(history) != 2 {
		t.Fatalf("history has %d orders, want 2", len(history))
	}
	if history[0].ID != "cancelled" || history[1].ID != "ready" {
		t.Fatalf("history order = %q, %q; want newest first", history[0].ID, history[1].ID)
	}
}

func TestCurrentJalaliParts(t *testing.T) {
	input := time.Date(2026, time.November, 5, 8, 44, 50, 0, tehran())
	y, m, d := currentJalaliParts(input)
	if y != 1405 || m != 8 || d != 14 {
		t.Fatalf("got %04d/%02d/%02d, want 1405/08/14", y, m, d)
	}
}

func TestValidateOrderAcceptsManualJalaliDateAndTimeFields(t *testing.T) {
	now := time.Date(2026, time.November, 5, 8, 40, 0, 0, tehran())
	form := url.Values{
		"symbol":          {"TEST"},
		"side":            {"buy"},
		"price":           {"1000"},
		"quantity":        {"10"},
		"execute_at_date": {"۱۴۰۵/۰۸/۱۴"},
		"execute_time":    {"08:44:50"},
	}
	req := httptest.NewRequest("POST", "/orders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	order, err := validateOrder(req, now)
	if err != nil {
		t.Fatalf("validateOrder with manual date fields: %v", err)
	}
	want := time.Date(2026, time.November, 5, 8, 44, 50, 0, tehran())
	if !order.ExecuteAt.Equal(want) {
		t.Fatalf("execute time = %s, want %s", order.ExecuteAt, want)
	}
	if order.ExecutionMode != executionModeManual {
		t.Fatalf("execution mode = %q, want %q", order.ExecutionMode, executionModeManual)
	}
}

func TestValidateOrderRejectsAPIUntilIntegrationExists(t *testing.T) {
	t.Setenv("RABIN_ENABLE_LIVE_ORDER_SUBMISSION", "false")
	now := time.Date(2026, time.November, 5, 8, 40, 0, 0, tehran())
	form := url.Values{
		"symbol":          {"TEST"},
		"side":            {"buy"},
		"price":           {"1000"},
		"quantity":        {"10"},
		"execute_at_date": {"۱۴۰۵/۰۸/۱۴"},
		"execute_time":    {"08:44:50"},
		"execution_mode":  {"api"},
	}
	req := httptest.NewRequest("POST", "/orders", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := validateOrder(req, now); err == nil || !strings.Contains(err.Error(), "فعال و پیکربندی نشده") {
		t.Fatalf("API mode error = %v, want explicit not-enabled error", err)
	}
}

func TestConfirmManualOrder(t *testing.T) {
	path := t.TempDir() + "/orders.json"
	store, err := newStore(path)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	store.orders = []Order{{
		ID: "manual-1", ExecutionMode: executionModeManual,
		Status: statusReady, ExecuteAt: time.Now().Add(-time.Minute),
	}}
	if err := store.confirmManual("manual-1"); err != nil {
		t.Fatalf("confirmManual: %v", err)
	}
	orders := store.snapshot()
	if len(orders) != 1 || orders[0].Status != statusManualDone {
		t.Fatalf("confirmed order = %#v, want status %q", orders, statusManualDone)
	}

	if err := store.confirmManual("manual-1"); err == nil {
		t.Fatal("expected a second confirmation to be rejected")
	}
}

func TestConfirmManualRejectsNonManualAndNotReadyOrders(t *testing.T) {
	store := &Store{orders: []Order{
		{ID: "scheduled", ExecutionMode: executionModeManual, Status: statusScheduled},
		{ID: "api", ExecutionMode: executionModeAPI, Status: statusReady},
	}}
	if err := store.confirmManual("scheduled"); err == nil {
		t.Fatal("expected scheduled order to be rejected")
	}
	if err := store.confirmManual("api"); err == nil {
		t.Fatal("expected API order to be rejected by manual confirmation")
	}
}

func TestSameOriginAcceptsSameHostFormRequests(t *testing.T) {
	tests := []struct {
		name   string
		header string
		value  string
	}{
		{name: "Origin", header: "Origin", value: "http://213.176.7.211:8098"},
		{name: "Referer fallback", header: "Referer", value: "http://213.176.7.211:8098/"},
		{name: "Fetch Metadata fallback", header: "Sec-Fetch-Site", value: "same-origin"},
		{name: "No browser metadata", header: "", value: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://213.176.7.211:8098/orders", nil)
			if tc.header != "" {
				req.Header.Set(tc.header, tc.value)
			}
			if !sameOrigin(req) {
				t.Fatalf("sameOrigin rejected same-host request with %s", tc.header)
			}
		})
	}
}

func TestSameOriginRejectsCrossSiteRequests(t *testing.T) {
	req := httptest.NewRequest("POST", "http://213.176.7.211:8098/orders", nil)
	req.Header.Set("Origin", "http://example.com")
	if sameOrigin(req) {
		t.Fatal("sameOrigin accepted a cross-site Origin")
	}

	req = httptest.NewRequest("POST", "http://213.176.7.211:8098/orders", nil)
	req.Header.Set("Referer", "http://example.com/form")
	if sameOrigin(req) {
		t.Fatal("sameOrigin accepted a cross-site Referer")
	}
}

func TestClearHistoryPreservesActiveAndAmbiguousOrders(t *testing.T) {
	path := t.TempDir() + "/orders.json"
	store, err := newStore(path)
	if err != nil {
		t.Fatalf("newStore: %v", err)
	}
	statuses := []string{
		statusScheduled, statusReady, statusSubmitting, statusSubmissionUnknown,
		statusCancelled, statusManualDone, statusSubmitted, statusSubmissionFailed,
		"future_status",
	}
	for _, status := range statuses {
		store.orders = append(store.orders, Order{ID: status, Status: status})
	}
	if err := store.saveLocked(); err != nil {
		t.Fatalf("save before clear: %v", err)
	}
	if err := store.clearHistory(); err != nil {
		t.Fatalf("clearHistory: %v", err)
	}
	got, err := newStore(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	want := map[string]bool{
		statusScheduled: true, statusReady: true, statusSubmitting: true,
		statusSubmissionUnknown: true, "future_status": true,
	}
	if len(got.orders) != len(want) {
		t.Fatalf("remaining orders = %d, want %d: %#v", len(got.orders), len(want), got.orders)
	}
	for _, order := range got.orders {
		if !want[order.Status] {
			t.Errorf("terminal status %q should have been removed", order.Status)
		}
		delete(want, order.Status)
	}
	if len(want) != 0 {
		t.Errorf("these active/ambiguous statuses were removed: %#v", want)
	}
}
