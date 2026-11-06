package main

import (
	"context"
	"log"
	"sync"
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
	for _, order := range store.snapshot() {
		mode := order.ExecutionMode
		if mode == "" {
			mode = executionModeManual
		}
		if mode != executionModeAPI || (order.Status != statusScheduled && order.Status != statusReady) {
			continue
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
	if m.ready || m.blocked || m.running || now.Before(earliest.Add(-2*time.Minute)) {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.generation++
	generation := m.generation
	m.mu.Unlock()

	go m.login(store, credentials, now, generation)
}

func (m *RabinAutoLogin) login(store *Store, credentials *RabinCredentialStore, startedAt time.Time, generation uint64) {
	username, password, err := credentials.Load()
	if err == nil && (username == "" || password == "") {
		err = context.Canceled
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
		store.setAPIAuthMessage("ورود خودکار رابین پس از دو تلاش ناموفق بود؛ سفارش ارسال نمی‌شود. برای ادامه، از صفحه اتصال به رابین دستی وارد شوید.", now)
		log.Printf("rabin auto-login: stopped after two failed attempts; API orders remain queued")
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
