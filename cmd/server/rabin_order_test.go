package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestRabinOrderClientSubmitsNormalOrderOnce(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/order" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-session" {
			t.Errorf("authorization header missing or invalid")
		}
		var body struct {
			ISIN string          `json:"isin"`
			Quantity int64       `json:"quantity"`
			Price int64          `json:"price"`
			ValidityType int     `json:"validityType"`
			ValidityDate *string `json:"validityDate"`
			OrderSide int `json:"orderSide"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode order body: %v", err)
			return
		}
		if body.ISIN != "IR1234567890" || body.Quantity != 2 || body.Price != 123456 || body.ValidityType != 1 || body.ValidityDate != nil || body.OrderSide != rabinOrderSideBuy {
			t.Errorf("unexpected order payload: %+v", body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":98765}}`))
	}))
	defer server.Close()

	order := Order{ISIN: "IR1234567890", Side: "buy", Price: 123456, Quantity: 2}
	result, err := (RabinOrderClient{URL: server.URL + "/api/order", HTTP: server.Client()}).Submit(context.Background(), "test-session", order)
	if err != nil {
		t.Fatal(err)
	}
	if result.OrderID != "98765" || result.Unknown {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRabinOrderClientMarksMissingOrderIDAsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message":"accepted"}}`))
	}))
	defer server.Close()

	order := Order{ISIN: "IR1234567890", Side: "sell", Price: 123456, Quantity: 1}
	result, err := (RabinOrderClient{URL: server.URL, HTTP: server.Client()}).Submit(context.Background(), "test-session", order)
	if err == nil || !result.Unknown {
		t.Fatalf("expected ambiguous result, got result=%+v err=%v", result, err)
	}
}

func TestRabinOrderStoreNeverReclaimsUnknownSubmission(t *testing.T) {
	store, err := newStore(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(tehran())
	order := Order{
		ID: "a1", Symbol: "TEST", ISIN: "IR1234567890", Side: "buy",
		Price: 1000, Quantity: 1, ExecuteAt: now.Add(-time.Minute),
		ExecutionMode: executionModeAPI, Status: statusReady, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.add(order); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.claimReadyAPIOrders(now)
	if err != nil || len(claimed) != 1 || claimed[0].Status != statusSubmitting {
		t.Fatalf("claim failed: %+v err=%v", claimed, err)
	}
	if err := store.completeAPIOrder("a1", statusSubmissionUnknown, "", "ambiguous", now); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.claimReadyAPIOrders(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 0 {
		t.Fatalf("unknown order was reclaimed: %+v", claimed)
	}
}

func TestRabinOrderStoreMarksInterruptedSubmissionUnknown(t *testing.T) {
	store, err := newStore(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(tehran())
	if err := store.add(Order{ID: "a2", ExecutionMode: executionModeAPI, Status: statusSubmitting, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.recoverInterruptedAPIOrders(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got := store.snapshot()
	if len(got) != 1 || got[0].Status != statusSubmissionUnknown {
		t.Fatalf("interrupted submission was not recovered safely: %+v", got)
	}
}

func TestRabinBuyAndSellSideEnumsMatchPublicFrontend(t *testing.T) {
	if rabinOrderSideBuy != 1 {
		t.Fatalf("BUY enum = %d, want 1", rabinOrderSideBuy)
	}
	if rabinOrderSideSell != 2 {
		t.Fatalf("SELL enum = %d, want 2", rabinOrderSideSell)
	}
}

func TestRabinOrderIDAcceptsScalarIDs(t *testing.T) {
	if got := rabinOrderID(json.RawMessage(`12345`)); got != "12345" {
		t.Fatalf("numeric order ID = %q, want 12345", got)
	}
	if got := rabinOrderID(json.RawMessage(`"abc-123"`)); got != "abc-123" {
		t.Fatalf("string order ID = %q, want abc-123", got)
	}
}

func TestRabinOrderAPIRemainsOptIn(t *testing.T) {
	t.Setenv("RABIN_ENABLE_LIVE_ORDER_SUBMISSION", "false")
	if rabinOrderAPICapable() {
		t.Fatal("API order submission must be disabled by default")
	}
	t.Setenv("RABIN_ENABLE_LIVE_ORDER_SUBMISSION", "true")
	if !rabinOrderAPICapable() {
		t.Fatal("explicit API order opt-in was not recognized")
	}
}
