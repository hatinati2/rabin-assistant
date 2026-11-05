package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTSESymbolClientFetchesCashAndETF(t *testing.T) {
	var cashCalled, etfCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.Header.Get("User-Agent") == "" || r.Header.Get("Referer") != "https://www.tse.ir/" {
			t.Fatalf("missing expected gateway request headers: %#v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case tseMarketWatchCashPath:
			cashCalled = true
			fmt.Fprint(w, `{"Items":[{"instrumentId":"IRO1IKCO0001","instrumentName":"خودرو","companyNamePersian":"ایران خودرو"},{"instrumentId":"IRO1FOLD0001","instrumentName":"فولاد","companyNamePersian":"فولاد مبارکه"}]}`)
		case tseMarketWatchETFPath:
			etfCalled = true
			fmt.Fprint(w, `{"Items":[{"instrumentId":"IRT1AHRA0001","instrumentName":"اهرم","companyNamePersian":"صندوق اهرمی"}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := TSESymbolClient{BaseURL: server.URL, HTTP: server.Client()}
	got, err := client.FetchSymbols(context.Background())
	if err != nil {
		t.Fatalf("FetchSymbols: %v", err)
	}
	if !cashCalled || !etfCalled {
		t.Fatalf("cashCalled=%v, etfCalled=%v; want both endpoints", cashCalled, etfCalled)
	}
	if len(got) != 3 {
		t.Fatalf("got %d symbols, want 3: %#v", len(got), got)
	}
	found := map[string]MarketSymbol{}
	for _, item := range got {
		found[item.ISIN] = item
	}
	if found["IRO1IKCO0001"].Symbol != "خودرو" || found["IRO1IKCO0001"].CompanyName != "ایران خودرو" {
		t.Fatalf("unexpected خودرو item: %#v", found["IRO1IKCO0001"])
	}
	if found["IRT1AHRA0001"].MarketType != "صندوق ETF" {
		t.Fatalf("ETF market type = %q", found["IRT1AHRA0001"].MarketType)
	}
}

func TestTSESymbolClientRejectsEmptyOrInvalidISINs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Items":[{"instrumentId":"NOT-ISIN","instrumentName":"نماد نامعتبر"}]}`)
	}))
	defer server.Close()

	client := TSESymbolClient{BaseURL: server.URL, HTTP: server.Client()}
	if _, err := client.FetchSymbols(context.Background()); err == nil || !strings.Contains(err.Error(), "خالی") {
		t.Fatalf("FetchSymbols error = %v, want empty/invalid catalog error", err)
	}
}

func TestMarketSymbolCatalogFallbackAndRefresh(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "symbols.json")
	catalog := NewMarketSymbolCatalog(cachePath)
	items, updatedAt, _ := catalog.Snapshot()
	if len(items) != 1 || items[0].ISIN != "IRO1IKCO0001" || !updatedAt.IsZero() {
		t.Fatalf("unexpected initial fallback: %#v, %v", items, updatedAt)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == tseMarketWatchCashPath {
			fmt.Fprint(w, `{"Items":[{"instrumentId":"IRO1IKCO0001","instrumentName":"خودرو","companyNamePersian":"ایران خودرو"}]}`)
			return
		}
		fmt.Fprint(w, `{"Items":[{"instrumentId":"IRT1AHRA0001","instrumentName":"اهرم","companyNamePersian":"صندوق اهرمی"}]}`)
	}))
	defer server.Close()

	if err := catalog.Refresh(context.Background(), TSESymbolClient{BaseURL: server.URL, HTTP: server.Client()}); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	items, updatedAt, lastError := catalog.Snapshot()
	if len(items) != 2 || updatedAt.IsZero() || lastError != "" {
		t.Fatalf("unexpected refreshed snapshot: %d items, %v, %q", len(items), updatedAt, lastError)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("cache file was not written: %v", err)
	}

	loaded := NewMarketSymbolCatalog(cachePath)
	loadedItems, loadedAt, _ := loaded.Snapshot()
	if len(loadedItems) != 2 || loadedAt.IsZero() {
		t.Fatalf("cache was not loaded: %d items, %v", len(loadedItems), loadedAt)
	}
}

func TestMarketSymbolCatalogKeepsLastGoodListOnRefreshFailure(t *testing.T) {
	catalog := NewMarketSymbolCatalog("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	err := catalog.Refresh(context.Background(), TSESymbolClient{BaseURL: server.URL, HTTP: server.Client()})
	if err == nil {
		t.Fatal("expected refresh failure")
	}
	items, updatedAt, lastError := catalog.Snapshot()
	if len(items) != 1 || items[0].ISIN != "IRO1IKCO0001" || !updatedAt.IsZero() || lastError == "" {
		t.Fatalf("fallback was not retained after failure: %#v, %v, %q", items, updatedAt, lastError)
	}
}
