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

const rabinLoginEndpoint = "https://api.rabin.ir/api/authentication/login"

type RabinLoginClient struct {
	HTTP *http.Client
}

func (c RabinLoginClient) Login(ctx context.Context, username, password string) (string, error) {
	payload, err := json.Marshal(map[string]string{"userName": username, "password": password})
	if err != nil {
		return "", errors.New("ساخت درخواست ورود ناموفق بود")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rabinLoginEndpoint, bytes.NewReader(payload))
	if err != nil {
		return "", errors.New("ساخت درخواست ورود ناموفق بود")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

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
		return "", errors.New("ارتباط با درگاه ورود رابین برقرار نشد")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", errors.New("خواندن پاسخ ورود ناموفق بود")
	}
	var envelope struct {
		Success *bool           `json:"success"`
		Data    json.RawMessage `json:"data"`
		Msg     json.RawMessage `json:"msg"`
	}
	_ = json.Unmarshal(body, &envelope)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if message := rabinLoginMessage(envelope.Msg); message != "" {
			return "", fmt.Errorf("رابین ورود را نپذیرفت (HTTP %d): %s", resp.StatusCode, message)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return "", fmt.Errorf("رابین ورود را نپذیرفت (HTTP %d): نام کاربری یا رمز عبور را بررسی کنید", resp.StatusCode)
		}
		return "", fmt.Errorf("پاسخ درگاه ورود رابین ناموفق بود (HTTP %d)", resp.StatusCode)
	}
	if envelope.Success != nil && !*envelope.Success {
		if message := rabinLoginMessage(envelope.Msg); message != "" {
			return "", fmt.Errorf("ورود رابین ناموفق بود: %s", message)
		}
		return "", errors.New("رابین ورود را تأیید نکرد")
	}
	data := envelope.Data
	if len(data) == 0 || string(data) == "null" {
		data = body
	}
	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		var challenge int
		if json.Unmarshal(data, &challenge) == nil && (challenge == 1 || challenge == 2 || challenge == 3) {
			return "", errors.New("رابین ورود را به مرحله تأیید دیگری ارجاع داد؛ این نسخه فقط ورود عادی با نام کاربری و رمز را پشتیبانی می‌کند")
		}
		return "", errors.New("پاسخ رابین موفقیت ورود را تأیید نکرد")
	}
	if result.AccessToken == "" {
		return "", errors.New("پاسخ رابین فاقد توکن نشست بود")
	}
	return result.AccessToken, nil
}

func rabinLoginMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return cleanRabinLoginMessage(single)
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil {
		parts := make([]string, 0, len(many))
		for _, item := range many {
			if item = cleanRabinLoginMessage(item); item != "" {
				parts = append(parts, item)
			}
		}
		return strings.Join(parts, "؛ ")
	}
	return ""
}

func cleanRabinLoginMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 400 {
		value = value[:400]
	}
	return value
}
