package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	limit := flag.Int("limit", 100, "max messages to replay from DLQ")
	flag.Parse()

	url := env("APP_RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/")
	exchange := env("APP_RABBITMQ_EXCHANGE", "estimate.calc")
	dlq := "estimate.calc.dlq"
	mainKey := "estimate.calc"

	conn, err := amqp.Dial(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		fmt.Fprintf(os.Stderr, "channel failed: %v\n", err)
		os.Exit(1)
	}
	defer ch.Close()

	replayed := 0
	ctx := context.Background()
	for replayed < *limit {
		msg, ok, err := ch.Get(dlq, false)
		if err != nil {
			fmt.Fprintf(os.Stderr, "get failed: %v\n", err)
			os.Exit(1)
		}
		if !ok {
			break
		}
		body := msg.Body
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err == nil {
			payload["attempt"] = 1
			if updated, err := json.Marshal(payload); err == nil {
				body = updated
			}
		}
		err = ch.PublishWithContext(ctx, exchange, mainKey, false, false, amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
			Timestamp:    time.Now().UTC(),
		})
		if err != nil {
			_ = msg.Nack(false, true)
			fmt.Fprintf(os.Stderr, "publish failed: %v\n", err)
			os.Exit(1)
		}
		if err := msg.Ack(false); err != nil {
			fmt.Fprintf(os.Stderr, "ack failed: %v\n", err)
			os.Exit(1)
		}
		replayed++
	}
	fmt.Printf("replayed %d message(s) from %s to %s\n", replayed, dlq, mainKey)
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
