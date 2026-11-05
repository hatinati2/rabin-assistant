package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const rabinOrderEndpoint = "https://api.rabin.ir/api/order"

type RabinOrderClient struct {
	URL    string
	HTTP   *http.Client
}

type rabinOrderPayload struct {
	ISIN         string          `json:"isin"`
	Quantity     int64           `json:"quantity"`
	Price        int64           `json:"price"`
	ValidityType int             `json:"validityType"`
	ValidityDate *string         `json:"validityDate"`
	OrderSide    json.RawMessage `json:"orderSide"`
}

type RabinSubmitResult struct {
	OrderID string
	Message string
	Unknown bool
}

func (c RabinOrderClient) Submit(ctx context.Context, token string, order Order, orderSideJSON string) (RabinSubmitResult, error) {
	if strings.TrimSpace(token) == "" {
		return RabinSubmitResult{}, errors.New("نشست رابین موجود نیست؛ ابتدا اتصال رابین را آزمایش کنید")
	}
	if strings.TrimSpace(order.ISIN) == "" {
		return RabinSubmitResult{}, errors.New("شناسه ISIN برای ارسال سفارش وارد نشده است")
	}
	side := json.RawMessage(strings.TrimSpace(orderSideJSON))
	if len(side) == 0 || !json.Valid(side) {
		return RabinSubmitResult{}, errors.New("کد نوع سفارش خرید/فروش در تنظیمات سرور معتبر نیست")
	}
	if order.Side != "buy" && order.Side != "sell" {
		return RabinSubmitResult{}, errors.New("نوع سفارش نامعتبر است")
	}
	payload := rabinOrderPayload{
		ISIN: strings.TrimSpace(order.ISIN),
		Quantity: order.Quantity,
		Price: order.Price,
		ValidityType: 1,
		ValidityDate: nil,
		OrderSide: side,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return RabinSubmitResult{}, errors.New("ساخت درخواست سفارش رابین ناموفق بود")
	}
	endpoint := c.URL
	if endpoint == "" {
		endpoint = rabinOrderEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return RabinSubmitResult{}, errors.New("ساخت درخواست سفارش رابین ناموفق بود")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := c.HTTP
	if client == nil {
		client = &http.Client{
			Timeout: 12 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		// The server may have accepted the request before the connection failed.
		return RabinSubmitResult{Unknown: true}, errors.New("پاسخ قطعی از رابین دریافت نشد؛ برای جلوگیری از ثبت تکراری، سفارش خودکار تکرار نمی‌شود")
	}
	defer resp.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return RabinSubmitResult{Unknown: true}, errors.New("پاسخ سفارش از رابین کامل دریافت نشد؛ سفارش خودکار تکرار نمی‌شود")
	}

	var envelope struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
		Msg     json.RawMessage `json:"msg"`
	}
	_ = json.Unmarshal(responseBody, &envelope)
	message := rabinOrderMessage(envelope.Msg)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode >= 500 {
			if message == "" {
				message = fmt.Sprintf("پاسخ سرور رابین HTTP %d بود", resp.StatusCode)
			}
			return RabinSubmitResult{Unknown: true, Message: message}, errors.New("نتیجه ثبت سفارش از سمت رابین قطعی نیست؛ از ارسال مجدد خودکار جلوگیری شد: " + message)
		}
		if message == "" {
			message = fmt.Sprintf("رابین درخواست سفارش را با HTTP %d رد کرد", resp.StatusCode)
		}
		return RabinSubmitResult{Message: message}, errors.New(message)
	}
	if envelope.Success != nil && !*envelope.Success {
		if message == "" {
			message = "رابین درخواست سفارش را نپذیرفت"
		}
		return RabinSubmitResult{Message: message}, errors.New(message)
	}
	orderID := rabinOrderID(envelope.Data)
	if orderID == "" {
		orderID = rabinOrderID(responseBody)
	}
	if orderID == "" {
		return RabinSubmitResult{Unknown: true, Message: "پاسخ موفق HTTP دریافت شد اما شناسه سفارش در پاسخ نبود"}, errors.New("رابین پاسخ موفق داد اما شناسه سفارش را برنگرداند؛ برای جلوگیری از ثبت تکراری، سفارش خودکار تکرار نمی‌شود")
	}
	return RabinSubmitResult{OrderID: orderID, Message: message}, nil
}

func rabinOrderID(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return ""
	}
	for _, key := range []string{"orderId", "id", "orderID"} {
		if value := values[key]; len(value) != 0 {
			var text string
			if json.Unmarshal(value, &text) == nil && text != "" {
				return text
			}
			var number json.Number
			if json.Unmarshal(value, &number) == nil && number.String() != "" {
				return number.String()
			}
		}
	}
	if nested := values["data"]; len(nested) != 0 {
		return rabinOrderID(nested)
	}
	return ""
}

func rabinOrderMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return strings.Join(many, "؛ ")
	}
	return ""
}
