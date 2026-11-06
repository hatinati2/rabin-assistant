package main

import (
	"fmt"
)

const (
	orderPlanDayMilliseconds int64 = 24 * 60 * 60 * 1000
	orderPlanMaxLeadMS       int64 = 60 * 60 * 1000
	orderPlanMaxIntervalMS   int64 = 60 * 1000
	orderPlanMaxSlots              = 5000
)

type orderPlanSlot struct {
	Number int    `json:"number"`
	Time   string `json:"time"`
}

type orderPlanResult struct {
	Target     string          `json:"target"`
	LeadMS     int64           `json:"lead_ms"`
	IntervalMS int64           `json:"interval_ms"`
	Count      int             `json:"count"`
	First      string          `json:"first"`
	Last       string          `json:"last"`
	Slots      []orderPlanSlot `json:"slots"`
}

// buildOrderPlan calculates display-only time slots before a target clock time.
// The target instant itself is excluded: a 10-second lead with a 200ms interval
// produces 50 slots, from target-10s through target-200ms.
func buildOrderPlan(targetMS, leadMS, intervalMS int64) (orderPlanResult, error) {
	if targetMS < 0 || targetMS >= orderPlanDayMilliseconds {
		return orderPlanResult{}, fmt.Errorf("target time is outside one day")
	}
	if leadMS < 1 || leadMS > orderPlanMaxLeadMS {
		return orderPlanResult{}, fmt.Errorf("lead time must be between 1ms and 1 hour")
	}
	if intervalMS < 1 || intervalMS > orderPlanMaxIntervalMS {
		return orderPlanResult{}, fmt.Errorf("interval must be between 1ms and 1 minute")
	}
	if targetMS < leadMS {
		return orderPlanResult{}, fmt.Errorf("plan start crosses midnight; select the corresponding target date")
	}

	count := int((leadMS + intervalMS - 1) / intervalMS)
	if count < 1 || count > orderPlanMaxSlots {
		return orderPlanResult{}, fmt.Errorf("plan exceeds the %d-slot safety limit", orderPlanMaxSlots)
	}

	result := orderPlanResult{
		Target:     formatOrderPlanClock(targetMS),
		LeadMS:     leadMS,
		IntervalMS: intervalMS,
		Count:      count,
		Slots:      make([]orderPlanSlot, 0, count),
	}
	startMS := targetMS - leadMS
	for i := 0; i < count; i++ {
		slotMS := startMS + int64(i)*intervalMS
		if slotMS >= targetMS {
			break
		}
		slot := orderPlanSlot{Number: i + 1, Time: formatOrderPlanClock(slotMS)}
		result.Slots = append(result.Slots, slot)
	}
	result.Count = len(result.Slots)
	if result.Count == 0 {
		return orderPlanResult{}, fmt.Errorf("plan contains no slots")
	}
	result.First = result.Slots[0].Time
	result.Last = result.Slots[result.Count-1].Time
	return result, nil
}

func formatOrderPlanClock(milliseconds int64) string {
	hours := milliseconds / 3_600_000
	minutes := (milliseconds / 60_000) % 60
	seconds := (milliseconds / 1_000) % 60
	millis := milliseconds % 1_000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, millis)
}
