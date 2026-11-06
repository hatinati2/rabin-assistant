package main

import "errors"

// allocateBatchQuantities returns one positive quantity for each order.
// In total mode, remainder shares are assigned one-by-one to the earliest orders.
func allocateBatchQuantities(quantity int64, count int, mode string) ([]int64, error) {
	if quantity <= 0 || count < 1 || count > orderPlanMaxSlots {
		return nil, errors.New("تعداد سهم یا تعداد سفارش معتبر نیست")
	}
	out := make([]int64, count)
	switch mode {
	case "each":
		for i := range out {
			out[i] = quantity
		}
	case "total":
		if quantity < int64(count) {
			return nil, errors.New("تعداد کل سهم باید حداقل به اندازه تعداد سفارش‌ها باشد تا تعداد هر سفارش مثبت بماند")
		}
		base := quantity / int64(count)
		remainder := quantity % int64(count)
		for i := range out {
			out[i] = base
			if int64(i) < remainder {
				out[i]++
			}
		}
	default:
		return nil, errors.New("روش تعیین تعداد سهم معتبر نیست")
	}
	return out, nil
}
