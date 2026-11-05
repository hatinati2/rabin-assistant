package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	tseGatewayBaseURL       = "https://webgw.tse.ir"
	tseMarketWatchCashPath  = "/InstrumentProvider/api/v1/MarketWatch/MarketWatchCash/fa"
	tseMarketWatchETFPath   = "/InstrumentProvider/api/v1/MarketWatch/MarketWatchEtf/fa"
	tseMaxResponseBytes     = 16 << 20
)

var marketISINPattern = regexp.MustCompile(`^[A-Z]{2}[A-Z0-9]{9}[0-9]$`)

type MarketSymbol struct {
	Symbol      string `json:"symbol"`
	ISIN        string `json:"isin"`
	CompanyName string `json:"company_name,omitempty"`
	MarketType  string `json:"market_type,omitempty"`
}

type tseMarketWatchResponse struct {
	Items []struct {
		InstrumentID      string `json:"instrumentId"`
		InstrumentName    string `json:"instrumentName"`
		CompanyNamePersian string `json:"companyNamePersian"`
	} `json:"Items"`
}

type TSESymbolClient struct {
	BaseURL string
	HTTP    *http.Client
}

// FetchSymbols reads the current cash-market shares and ETF lists from TSE's
// public gateway. It only issues GET requests and never interacts with Rabin.
func (c TSESymbolClient) FetchSymbols(ctx context.Context) ([]MarketSymbol, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if baseURL == "" {
		baseURL = tseGatewayBaseURL
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	endpoints := []struct {
		path string
		kind string
	}{
		{path: tseMarketWatchCashPath, kind: "سهام"},
		{path: tseMarketWatchETFPath, kind: "صندوق ETF"},
	}

	byISIN := make(map[string]MarketSymbol)
	for _, endpoint := range endpoints {
		items, err := c.fetchMarketWatch(ctx, client, baseURL+endpoint.path)
		if err != nil {
			return nil, fmt.Errorf("دریافت فهرست %s از درگاه TSE: %w", endpoint.kind, err)
		}
		for _, item := range items {
			symbol := strings.TrimSpace(item.InstrumentName)
			isin := strings.ToUpper(strings.TrimSpace(item.InstrumentID))
			if symbol == "" || !marketISINPattern.MatchString(isin) {
				continue
			}
			entry := MarketSymbol{
				Symbol:      symbol,
				ISIN:        isin,
				CompanyName: strings.TrimSpace(item.CompanyNamePersian),
				MarketType:  endpoint.kind,
			}
			if previous, exists := byISIN[isin]; exists {
				if entry.CompanyName == "" {
					entry.CompanyName = previous.CompanyName
				}
				if entry.Symbol == "" {
					entry.Symbol = previous.Symbol
				}
			}
			byISIN[isin] = entry
		}
	}

	if len(byISIN) == 0 {
		return nil, errors.New("درگاه TSE فهرست خالی یا فاقد ISIN معتبر برگرداند")
	}

	result := make([]MarketSymbol, 0, len(byISIN))
	for _, item := range byISIN {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Symbol == result[j].Symbol {
			return result[i].ISIN < result[j].ISIN
		}
		return result[i].Symbol < result[j].Symbol
	})
	return result, nil
}

func (c TSESymbolClient) fetchMarketWatch(ctx context.Context, client *http.Client, endpoint string) ([]struct {
	InstrumentID       string `json:"instrumentId"`
	InstrumentName     string `json:"instrumentName"`
	CompanyNamePersian string `json:"companyNamePersian"`
}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/126 Safari/537.36")
	req.Header.Set("Accept", "application/json,text/plain,*/*")
	req.Header.Set("Referer", "https://www.tse.ir/")
	req.Header.Set("Origin", "https://www.tse.ir")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("پاسخ HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, tseMaxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > tseMaxResponseBytes {
		return nil, errors.New("اندازه پاسخ از حد مجاز بیشتر است")
	}

	var payload tseMarketWatchResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("ساختار JSON نامعتبر است: %w", err)
	}
	return payload.Items, nil
}

type marketSymbolCache struct {
	UpdatedAt time.Time     `json:"updated_at"`
	Symbols   []MarketSymbol `json:"symbols"`
}

type MarketSymbolCatalog struct {
	mu        sync.RWMutex
	cachePath string
	symbols   []MarketSymbol
	updatedAt time.Time
	lastError string
}

func NewMarketSymbolCatalog(cachePath string) *MarketSymbolCatalog {
	catalog := &MarketSymbolCatalog{
		cachePath: cachePath,
		symbols:   defaultMarketSymbols(),
	}
	if strings.TrimSpace(cachePath) == "" {
		return catalog
	}
	body, err := os.ReadFile(cachePath)
	if err != nil {
		return catalog
	}
	var cached marketSymbolCache
	if err := json.Unmarshal(body, &cached); err != nil || len(cached.Symbols) == 0 {
		return catalog
	}
	valid := make([]MarketSymbol, 0, len(cached.Symbols))
	for _, item := range cached.Symbols {
		item.Symbol = strings.TrimSpace(item.Symbol)
		item.ISIN = strings.ToUpper(strings.TrimSpace(item.ISIN))
		if item.Symbol != "" && marketISINPattern.MatchString(item.ISIN) {
			valid = append(valid, item)
		}
	}
	if len(valid) > 0 {
		catalog.symbols = valid
		catalog.updatedAt = cached.UpdatedAt
	}
	return catalog
}

func defaultMarketSymbols() []MarketSymbol {
	return []MarketSymbol{{
		Symbol:      "خودرو",
		ISIN:        "IRO1IKCO0001",
		CompanyName: "ایران‌خودرو",
		MarketType:  "سهام",
	}}
}

func (c *MarketSymbolCatalog) Refresh(ctx context.Context, client TSESymbolClient) error {
	symbols, err := client.FetchSymbols(ctx)
	if err != nil {
		c.mu.Lock()
		c.lastError = err.Error()
		c.mu.Unlock()
		return err
	}

	updatedAt := time.Now().In(tehran())
	cacheErr := error(nil)
	if c.cachePath != "" {
		// The in-memory result is still useful even when the disk cache cannot be written.
		cacheErr = c.saveCache(marketSymbolCache{UpdatedAt: updatedAt, Symbols: symbols})
	}

	c.mu.Lock()
	c.symbols = append([]MarketSymbol(nil), symbols...)
	c.updatedAt = updatedAt
	c.lastError = ""
	if cacheErr != nil {
		c.lastError = "فهرست دریافت شد، اما ذخیره نسخه محلی ممکن نشد"
	}
	c.mu.Unlock()
	return nil
}

func (c *MarketSymbolCatalog) saveCache(cached marketSymbolCache) error {
	body, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0700); err != nil {
		return err
	}
	tmp := c.cachePath + ".tmp"
	if err := os.WriteFile(tmp, body, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.cachePath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (c *MarketSymbolCatalog) Snapshot() ([]MarketSymbol, time.Time, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := append([]MarketSymbol(nil), c.symbols...)
	return items, c.updatedAt, c.lastError
}

func startMarketSymbolRefresh(catalog *MarketSymbolCatalog, client TSESymbolClient) {
	go func() {
		refresh := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			if err := catalog.Refresh(ctx, client); err != nil {
				log.Printf("TSE symbol catalog refresh failed: %v", err)
				return
			}
			items, _, _ := catalog.Snapshot()
			log.Printf("TSE symbol catalog refreshed: %d cash/ETF symbols", len(items))
		}

		refresh()
		ticker := time.NewTicker(12 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			refresh()
		}
	}()
}

func marketSymbolCatalogStatus(catalog *MarketSymbolCatalog) ([]MarketSymbol, string) {
	items, updatedAt, lastError := catalog.Snapshot()
	if !updatedAt.IsZero() {
		return items, fmt.Sprintf("فهرست %d نماد سهام و صندوق از درگاه TSE · آخرین به‌روزرسانی: %s", len(items), updatedAt.In(tehran()).Format("2006/01/02 15:04"))
	}
	if lastError != "" {
		return items, "اتصال به درگاه TSE برقرار نشد؛ فعلاً فهرست پشتیبان خودرو نمایش داده می‌شود."
	}
	return items, "در حال دریافت فهرست نمادها از درگاه TSE؛ در صورت نبود دسترسی، فهرست پشتیبان نمایش داده می‌شود."
}


// normalizeMarketSymbol handles common Arabic/Persian keyboard variants while
// keeping punctuation that may be part of a ticker.
func normalizeMarketSymbol(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer(
		"ي", "ی",
		"ك", "ک",
		"\u200c", "",
	).Replace(value)
	return strings.ToLower(value)
}

// lookupMarketSymbol only resolves exact ticker matches. When a ticker maps to
// multiple ISINs, it reports ambiguity instead of guessing the instrument.
func lookupMarketSymbol(symbols []MarketSymbol, query string) (MarketSymbol, bool, bool) {
	normalized := normalizeMarketSymbol(query)
	if normalized == "" {
		return MarketSymbol{}, false, false
	}

	var match MarketSymbol
	found := false
	for _, item := range symbols {
		if normalizeMarketSymbol(item.Symbol) != normalized {
			continue
		}
		if found && match.ISIN != item.ISIN {
			return MarketSymbol{}, false, true
		}
		match = item
		found = true
	}
	return match, found, false
}
