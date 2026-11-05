package main

import (
	"errors"
	"time"
)

func (s *Store) claimReadyAPIOrders(now time.Time) ([]Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	original := append([]Order(nil), s.orders...)
	claimed := make([]Order, 0)
	changed := false

	for i := range s.orders {
		order := &s.orders[i]
		mode := order.ExecutionMode
		if mode == "" {
			mode = executionModeManual
		}
		if mode != executionModeAPI || order.Status != statusReady {
			continue
		}
		if order.ISIN == "" {
			order.Status = statusSubmissionFailed
			order.BrokerMessage = "شناسه ISIN برای سفارش وارد نشده است؛ سفارش به کارگزاری ارسال نشد."
			order.UpdatedAt = now
			changed = true
			continue
		}
		order.Status = statusSubmitting
		order.AttemptedAt = now
		order.BrokerMessage = "درخواست در حال ارسال به رابین است."
		order.UpdatedAt = now
		claimed = append(claimed, *order)
		changed = true
	}

	if changed {
		if err := s.saveLocked(); err != nil {
			s.orders = original
			return nil, err
		}
	}
	return claimed, nil
}

func (s *Store) completeAPIOrder(id, finalStatus, brokerOrderID, message string, now time.Time) error {
	if finalStatus != statusSubmitted && finalStatus != statusSubmissionFailed && finalStatus != statusSubmissionUnknown {
		return errors.New("وضعیت نهایی ارسال سفارش نامعتبر است")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.orders {
		order := &s.orders[i]
		if order.ID != id {
			continue
		}
		if order.Status != statusSubmitting {
			return errors.New("سفارش در وضعیت ارسال نیست")
		}
		old := *order
		order.Status = finalStatus
		order.BrokerOrderID = brokerOrderID
		order.BrokerMessage = message
		order.UpdatedAt = now
		if err := s.saveLocked(); err != nil {
			*order = old
			return err
		}
		return nil
	}
	return errors.New("سفارش پیدا نشد")
}

func (s *Store) recoverInterruptedAPIOrders(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for i := range s.orders {
		if s.orders[i].Status == statusSubmitting {
			s.orders[i].Status = statusSubmissionUnknown
			s.orders[i].BrokerMessage = "سرویس هنگام ارسال متوقف شده است؛ به دلیل احتمال ثبت در رابین، ارسال خودکار مجدد انجام نمی‌شود. وضعیت را در کارگزاری بررسی کنید."
			s.orders[i].UpdatedAt = now
			changed = true
		}
	}
	if changed {
		return s.saveLocked()
	}
	return nil
}
