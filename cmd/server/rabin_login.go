package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("ورود به رابین ناموفق بود")
	}
	var envelope struct {
		Success bool `json:"success"`
		Data struct {
			AccessToken string `json:"accessToken"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || !envelope.Success || envelope.Data.AccessToken == "" {
		return "", errors.New("رابین توکن نشست معتبر برنگرداند")
	}
	return envelope.Data.AccessToken, nil
}
