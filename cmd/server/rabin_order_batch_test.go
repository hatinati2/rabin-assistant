package main

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAllocateBatchQuantitiesEachOrder(t *testing.T) {
	got, err := allocateBatchQuantities(10_000, 50, "each")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 50 || got[0] != 10_000 || got[49] != 10_000 {
		t.Fatalf("unexpected per-order allocation: len=%d first=%d last=%d", len(got), got[0], got[len(got)-1])
	}
}

func TestAllocateBatchQuantitiesTotalWithRemainder(t *testing.T) {
	got, err := allocateBatchQuantities(10_003, 50, "total")
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 201 || got[1] != 201 || got[2] != 201 || got[3] != 200 || got[49] != 200 {
		t.Fatalf("unexpected allocation: first=%v last=%d", got[:4], got[49])
	}
	var total int64
	for _, quantity := range got {
		total += quantity
	}
	if total != 10_003 {
		t.Fatalf("allocated total = %d, want 10003", total)
	}
}

func TestAllocateBatchQuantitiesRejectsInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		quantity int64
		count int
		mode string
	}{
		{0, 2, "each"}, {5, 0, "each"}, {1, 2, "total"}, {10, 2, "unknown"},
	} {
		if _, err := allocateBatchQuantities(tc.quantity, tc.count, tc.mode); err == nil {
			t.Fatalf("expected error for %+v", tc)
		}
	}
}

func TestStoreAddManyPersistsAllOrdersTogether(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.json")
	store, err := newStore(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(tehran())
	orders := []Order{
		{ID: "batch-1", Symbol: "TEST", Side: "buy", Price: 100, Quantity: 3, ExecuteAt: now.Add(time.Minute), Status: statusScheduled, CreatedAt: now, UpdatedAt: now},
		{ID: "batch-2", Symbol: "TEST", Side: "buy", Price: 100, Quantity: 2, ExecuteAt: now.Add(time.Minute * 2), Status: statusScheduled, CreatedAt: now, UpdatedAt: now},
	}
	if err := store.addMany(orders); err != nil {
		t.Fatal(err)
	}
	got := store.snapshot()
	if len(got) != 2 || !reflect.DeepEqual(got[0], orders[0]) || !reflect.DeepEqual(got[1], orders[1]) {
		t.Fatalf("stored batch differs: %+v", got)
	}
}

func TestStoreAddManyRejectsDuplicateIDWithoutPartialWrite(t *testing.T) {
	store, err := newStore(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(tehran())
	first := Order{ID: "same", Status: statusScheduled, CreatedAt: now, UpdatedAt: now}
	if err := store.add(first); err != nil {
		t.Fatal(err)
	}
	second := Order{ID: "same", Status: statusScheduled, CreatedAt: now, UpdatedAt: now}
	if err := store.addMany([]Order{second, {ID: "new", Status: statusScheduled, CreatedAt: now, UpdatedAt: now}}); err == nil {
		t.Fatal("expected duplicate ID error")
	}
	if got := store.snapshot(); len(got) != 1 || got[0].ID != "same" {
		t.Fatalf("partial batch was written: %+v", got)
	}
}

func TestStoreCancelAllScheduledPreservesNonScheduledOrders(t *testing.T) {
	store, err := newStore(filepath.Join(t.TempDir(), "orders.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(tehran())
	orders := []Order{
		{ID: "scheduled-manual", ExecutionMode: executionModeManual, Status: statusScheduled, CreatedAt: now, UpdatedAt: now},
		{ID: "scheduled-api", ExecutionMode: executionModeAPI, Status: statusScheduled, CreatedAt: now, UpdatedAt: now},
		{ID: "ready", ExecutionMode: executionModeManual, Status: statusReady, CreatedAt: now, UpdatedAt: now},
		{ID: "submitted", ExecutionMode: executionModeAPI, Status: statusSubmitted, CreatedAt: now, UpdatedAt: now},
	}
	if err := store.addMany(orders); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.cancelAllScheduled(now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if cancelled != 2 {
		t.Fatalf("cancelled count = %d, want 2", cancelled)
	}
	got := store.snapshot()
	statuses := make(map[string]string, len(got))
	for _, order := range got {
		statuses[order.ID] = order.Status
	}
	if statuses["scheduled-manual"] != statusCancelled || statuses["scheduled-api"] != statusCancelled {
		t.Fatalf("scheduled orders were not cancelled: %+v", statuses)
	}
	if statuses["ready"] != statusReady || statuses["submitted"] != statusSubmitted {
		t.Fatalf("non-scheduled orders must be preserved: %+v", statuses)
	}
	if cancelled, err := store.cancelAllScheduled(now.Add(2 * time.Second)); err != nil || cancelled != 0 {
		t.Fatalf("second cancellation should be a no-op, count=%d err=%v", cancelled, err)
	}
}

