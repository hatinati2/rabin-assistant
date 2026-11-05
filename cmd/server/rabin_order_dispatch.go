package main

import (
	"context"
	"log"
	"strings"
	"time"
)

func dispatchReadyRabinOrders(store *Store, credentials *RabinCredentialStore, now time.Time) {
	token, _, err := credentials.LoadAccessToken()
	if err != nil {
		log.Printf("rabin order sender: could not load saved session")
		return
	}
	if token == "" {
		// Leave orders in "ready" until the user reconnects Rabin.
		return
	}

	claimed, err := store.claimReadyAPIOrders(now)
	if err != nil {
		log.Printf("rabin order sender: could not claim ready orders: %v", err)
		return
	}
	for _, order := range claimed {
		sideJSON := strings.TrimSpace(env(
			map[bool]string{true: "RABIN_ORDER_SIDE_BUY_JSON", false: "RABIN_ORDER_SIDE_SELL_JSON"}[order.Side == "buy"],
			"",
		))
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		result, submitErr := (RabinOrderClient{}).Submit(ctx, token, order, sideJSON)
		cancel()

		finalStatus := statusSubmitted
		message := result.Message
		if submitErr != nil {
			if result.Unknown {
				finalStatus = statusSubmissionUnknown
			} else {
				finalStatus = statusSubmissionFailed
			}
			message = submitErr.Error()
		}
		if err := store.completeAPIOrder(order.ID, finalStatus, result.OrderID, message, time.Now().In(tehran())); err != nil {
			log.Printf("rabin order sender: failed to persist final state for order %s", order.ID)
			continue
		}
		if submitErr != nil {
			log.Printf("rabin order sender: order %s final_state=%s", order.ID, finalStatus)
			continue
		}
		log.Printf("rabin order sender: order %s accepted by Rabin; broker_order_id=%s", order.ID, result.OrderID)
	}
}
