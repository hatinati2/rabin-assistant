package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"net/url"
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


func TestLookupMarketSymbolExactMatchAndPersianNormalization(t *testing.T) {
	symbols := []MarketSymbol{
		{Symbol: "خودرو", ISIN: "IRO1IKCO0001"},
		{Symbol: "فولاد", ISIN: "IRO1FOLD0001"},
		{Symbol: "ABC", ISIN: "IRO1ABC00001"},
	}

	cases := []struct {
		query string
		want  string
	}{
		{query: "خودرو", want: "IRO1IKCO0001"},
		{query: " فولاد ", want: "IRO1FOLD0001"},
		{query: "abc", want: "IRO1ABC00001"},
	}
	for _, tc := range cases {
		got, found, ambiguous := lookupMarketSymbol(symbols, tc.query)
		if !found || ambiguous || got.ISIN != tc.want {
			t.Errorf("lookupMarketSymbol(%q) = %#v, found=%v, ambiguous=%v; want ISIN %q",
				tc.query, got, found, ambiguous, tc.want)
		}
	}

	arabicKeyboard := []MarketSymbol{{Symbol: "کالا", ISIN: "IRO1KALA0001"}}
	got, found, ambiguous := lookupMarketSymbol(arabicKeyboard, "كالا")
	if !found || ambiguous || got.ISIN != "IRO1KALA0001" {
		t.Fatalf("Arabic/Persian keyboard normalization failed: %#v, found=%v, ambiguous=%v", got, found, ambiguous)
	}
}

func TestLookupMarketSymbolDoesNotGuessWhenTickerIsAmbiguous(t *testing.T) {
	symbols := []MarketSymbol{
		{Symbol: "نماد", ISIN: "IRO1AAAA0001"},
		{Symbol: "نماد", ISIN: "IRO1BBBB0001"},
	}
	if _, found, ambiguous := lookupMarketSymbol(symbols, "نماد"); found || !ambiguous {
		t.Fatalf("lookup should report ambiguity, found=%v ambiguous=%v", found, ambiguous)
	}
}

func TestLookupMarketSymbolRequiresExactTicker(t *testing.T) {
	symbols := []MarketSymbol{{Symbol: "خودرو", ISIN: "IRO1IKCO0001"}}
	if _, found, ambiguous := lookupMarketSymbol(symbols, "خود"); found || ambiguous {
		t.Fatalf("partial ticker should not resolve, found=%v ambiguous=%v", found, ambiguous)
	}
}


func TestMarketSymbolLookupHandler(t *testing.T) {
	catalog := &MarketSymbolCatalog{symbols: []MarketSymbol{
		{Symbol: "خودرو", ISIN: "IRO1IKCO0001"},
		{Symbol: "فولاد", ISIN: "IRO1FOLD0001"},
	}}
	handler := marketSymbolLookupHandler(catalog)

	req := httptest.NewRequest(http.MethodGet, "/api/symbols/lookup?symbol="+url.QueryEscape("خودرو"), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var found MarketSymbol
	if err := json.Unmarshal(recorder.Body.Bytes(), &found); err != nil {
		t.Fatalf("decode lookup response: %v", err)
	}
	if found.Symbol != "خودرو" || found.ISIN != "IRO1IKCO0001" {
		t.Fatalf("lookup response = %#v", found)
	}

	ambiguousCatalog := &MarketSymbolCatalog{symbols: []MarketSymbol{
		{Symbol: "دوگانه", ISIN: "IRO1AAAA0001"},
		{Symbol: "دوگانه", ISIN: "IRO1BBBB0001"},
	}}
	ambiguousRecorder := httptest.NewRecorder()
	ambiguousRequest := httptest.NewRequest(http.MethodGet, "/api/symbols/lookup?symbol="+url.QueryEscape("دوگانه"), nil)
	marketSymbolLookupHandler(ambiguousCatalog).ServeHTTP(ambiguousRecorder, ambiguousRequest)
	if ambiguousRecorder.Code != http.StatusConflict {
		t.Fatalf("ambiguous lookup status = %d, want %d", ambiguousRecorder.Code, http.StatusConflict)
	}
}


func TestSuggestMarketSymbolsReturnsPrefixAndSubstringMatches(t *testing.T) {
	symbols := []MarketSymbol{
		{Symbol: "شپلی", ISIN: "IRO1SHPL0001", CompanyName: "پتروشیمی شازند"},
		{Symbol: "شپنا", ISIN: "IRO1SHNA0001", CompanyName: "پالایش نفت اصفهان"},
		{Symbol: "فولاد", ISIN: "IRO1FOLD0001", CompanyName: "فولاد مبارکه"},
		{Symbol: "پلیمر", ISIN: "IRO1PLMR0001", CompanyName: "پلیمر"},
	}

	got := suggestMarketSymbols(symbols, "شپ", 12)
	if len(got) != 2 {
		t.Fatalf("suggestMarketSymbols returned %d entries, want 2: %#v", len(got), got)
	}
	if got[0].Symbol != "شپنا" && got[0].Symbol != "شپلی" {
		t.Fatalf("prefix suggestions did not include the expected ticker: %#v", got)
	}
	seen := map[string]bool{}
	for _, item := range got {
		seen[item.Symbol] = true
	}
	if !seen["شپلی"] || !seen["شپنا"] {
		t.Fatalf("suggestions for شپ = %#v; want شپلی and شپنا", got)
	}

	inside := suggestMarketSymbols(symbols, "پلی", 12)
	if len(inside) != 2 {
		t.Fatalf("substring suggestions = %#v; want شپلی and پلیمر", inside)
	}
}

func TestSuggestMarketSymbolsPrioritizesTickerPrefixAndLimitsResults(t *testing.T) {
	symbols := []MarketSymbol{
		{Symbol: "نامربوط", CompanyName: "شپ شرکت"},
		{Symbol: "فولاد", CompanyName: "فولاد"},
		{Symbol: "شپلی", ISIN: "IRO1SHPL0001"},
		{Symbol: "شپنا", ISIN: "IRO1SHNA0001"},
	}
	got := suggestMarketSymbols(symbols, "شپ", 1)
	if len(got) != 1 || (got[0].Symbol != "شپلی" && got[0].Symbol != "شپنا") {
		t.Fatalf("limited prefix results = %#v; want a ticker-prefix result, not a company-name-only match", got)
	}
}

func TestMarketSymbolSuggestHandler(t *testing.T) {
	catalog := &MarketSymbolCatalog{symbols: []MarketSymbol{
		{Symbol: "شپنا", ISIN: "IRO1SHNA0001", CompanyName: "پالایش نفت اصفهان"},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tseInstrumentShortcutPath {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("SimpleSearchText") != "شپ" ||
			r.URL.Query().Get("PageNumber") != "1" ||
			r.URL.Query().Get("PageSize") != "12" {
			t.Errorf("unexpected InstrumentShortcut query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[{"instrumentid":"IRO1SHPL0001","instrument_Name":"شپلی","company_Name_Persian":"پتروشیمی شازند","marketcategoryname":"بازار دوم"}]}`)
	}))
	defer server.Close()

	handler := marketSymbolSuggestHandler(catalog, TSESymbolClient{BaseURL: server.URL, HTTP: server.Client()})
	req := httptest.NewRequest(http.MethodGet, "/api/symbols/suggest?q="+url.QueryEscape("شپ"), nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("suggest status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var suggestions []MarketSymbol
	if err := json.Unmarshal(recorder.Body.Bytes(), &suggestions); err != nil {
		t.Fatalf("decode suggestion response: %v", err)
	}
	if len(suggestions) != 2 {
		t.Fatalf("got %d suggestions, want remote شپلی plus cached شپنا: %#v", len(suggestions), suggestions)
	}
	seen := map[string]bool{}
	for _, item := range suggestions {
		if item.ISIN == "" {
			t.Fatalf("suggestion did not include ISIN values: %#v", suggestions)
		}
		seen[item.Symbol] = true
	}
	if !seen["شپلی"] || !seen["شپنا"] {
		t.Fatalf("suggestions did not merge remote and cached symbols: %#v", suggestions)
	}
}

func TestTSESymbolClientSearchSymbolsMapsShortcutFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != tseInstrumentShortcutPath {
			t.Fatalf("path = %q, want %q", r.URL.Path, tseInstrumentShortcutPath)
		}
		if r.URL.Query().Get("SimpleSearchText") != "شپلی" ||
			r.URL.Query().Get("PageNumber") != "1" ||
			r.URL.Query().Get("PageSize") != "12" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[{"instrumentid":"IRO1SHPL0001","instrument_Name":"شپلی","company_Name_Persian":"پتروشیمی شازند","marketcategoryname":"بازار دوم"}]}`)
	}))
	defer server.Close()

	got, err := (TSESymbolClient{BaseURL: server.URL, HTTP: server.Client()}).SearchSymbols(context.Background(), "شپلی", 12)
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	if got[0].Symbol != "شپلی" || got[0].ISIN != "IRO1SHPL0001" ||
		got[0].CompanyName != "پتروشیمی شازند" || got[0].MarketType != "بازار دوم" {
		t.Fatalf("unexpected mapped instrument: %#v", got[0])
	}
}
