package main

import "testing"

func TestBuildOrderPlanTenSecondsEvery200Milliseconds(t *testing.T) {
	plan, err := buildOrderPlan((8*60*60+45*60)*1000, 10_000, 200)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Count != 50 || len(plan.Slots) != 50 {
		t.Fatalf("slot count = %d, want 50", plan.Count)
	}
	if plan.First != "08:44:50.000" {
		t.Fatalf("first slot = %q, want 08:44:50.000", plan.First)
	}
	if plan.Last != "08:44:59.800" {
		t.Fatalf("last slot = %q, want 08:44:59.800", plan.Last)
	}
	if plan.Slots[1].Time != "08:44:50.200" || plan.Slots[49].Number != 50 {
		t.Fatalf("unexpected slot sequence: first few=%+v last=%+v", plan.Slots[:2], plan.Slots[49])
	}
}

func TestBuildOrderPlanRoundsUpAndStopsBeforeTarget(t *testing.T) {
	plan, err := buildOrderPlan(12_000, 1_000, 300)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Count != 4 || plan.First != "00:00:11.000" || plan.Last != "00:00:11.900" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestBuildOrderPlanRejectsInvalidOrUnsafeInputs(t *testing.T) {
	tests := []struct {
		name                       string
		targetMS, leadMS, everyMS int64
	}{
		{name: "target outside day", targetMS: 86_400_000, leadMS: 1000, everyMS: 100},
		{name: "zero lead", targetMS: 10_000, leadMS: 0, everyMS: 100},
		{name: "lead too long", targetMS: 4_000_000, leadMS: 3_600_001, everyMS: 100},
		{name: "zero interval", targetMS: 10_000, leadMS: 1000, everyMS: 0},
		{name: "interval too long", targetMS: 10_000, leadMS: 1000, everyMS: 60_001},
		{name: "crosses midnight", targetMS: 500, leadMS: 1000, everyMS: 100},
		{name: "too many slots", targetMS: 10_000, leadMS: 10_000, everyMS: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := buildOrderPlan(tt.targetMS, tt.leadMS, tt.everyMS); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
