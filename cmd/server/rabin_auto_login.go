package main

import (
	"context"
	"errors"
	"log"
	"sync"
	"strings"
	"time"
)

// RabinAutoLogin performs one scheduled login two minutes before the next
// pending API order. It retries once after five seconds and then blocks API
// dispatch until a successful manual login.
type RabinAutoLogin struct {
	mu         sync.Mutex
	ready      bool
	blocked    bool
	running    bool
	generation uint64
}

func (m *RabinAutoLogin) Prepare(store *Store, credentials *RabinCredentialStore, now time.Time) {
	var earliest time.Time
	blockedByOrder := false
	for _, order := range store.snapshot() {
		mode := order.ExecutionMode
		if mode == "" {
			mode = executionModeManual
		}
		if mode != executionModeAPI || (order.Status != statusScheduled && order.Status != statusReady) {
			continue
		}
		if strings.HasPrefix(order.BrokerMessage, "ورود خودکار رابین ناموفق بود") {
			blockedByOrder = true
		}
		if earliest.IsZero() || order.ExecuteAt.Before(earliest) {
			earliest = order.ExecuteAt
		}
	}

	m.mu.Lock()
	if earliest.IsZero() {
		if m.running || m.ready || m.blocked {
			m.generation++
			m.running, m.ready, m.blocked = false, false, false
		}
		m.mu.Unlock()
		return
	}
	if blockedByOrder {
		m.blocked = true
		m.mu.Unlock()
		return
	}
	if m.ready || m.blocked || m.running {
		m.mu.Unlock()
		return
	}
	if token, updatedAt, err := credentials.LoadAccessToken(); err == nil && token != "" && !updatedAt.Before(earliest.Add(-2*time.Minute)) {
		m.ready = true
		m.mu.Unlock()
		return
	}
	if now.Before(earliest.Add(-2*time.Minute)) {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.generation++
	generation := m.generation
	m.mu.Unlock()

	go m.login(store, credentials, generation)
}

func (m *RabinAutoLogin) login(store *Store, credentials *RabinCredentialStore, generation uint64) {
	username, password, err := credentials.Load()
	if err == nil && (username == "" || password == "") {
		err = errors.New("اطلاعات ورود رابین ذخیره نشده است")
	}
	var token string
	if err == nil {
		for attempt := 1; attempt <= 2; attempt++ {
			ctx, cancel := context.WithTimeout(context.Background(), 13*time.Second)
			token, err = (RabinLoginClient{}).Login(ctx, username, password)
			cancel()
			if err == nil {
				break
			}
			log.Printf("rabin auto-login: attempt %d/2 failed: %v", attempt, err)
			if attempt == 1 {
				time.Sleep(5 * time.Second)
			}
		}
	}
	if err == nil {
		err = credentials.SaveAccessToken(token)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.generation != generation {
		return
	}
	m.running = false
	now := time.Now().In(tehran())
	if err != nil {
		m.blocked = true
		store.setAPIAuthMessage("ورود خودکار رابین ناموفق بود؛ پس از حداکثر دو تلاش، سفارش ارسال نمی‌شود. برای ادامه، از صفحه اتصال به رابین دستی وارد شوید.", now)
		log.Printf("rabin auto-login: stopped after at most two attempts; API orders remain queued")
		return
	}
	m.ready = true
	store.setAPIAuthMessage("", now)
	log.Printf("rabin auto-login: session refreshed successfully before scheduled API orders")
}

func (m *RabinAutoLogin) IsReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ready
}

func (m *RabinAutoLogin) MarkManualLoginSuccess(store *Store, now time.Time) {
	m.mu.Lock()
	m.generation++
	m.running = false
	m.blocked = false
	m.ready = true
	m.mu.Unlock()
	store.setAPIAuthMessage("", now)
}
