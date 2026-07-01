package main

import (
	"fmt"
	"os"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	url := env("APP_RABBITMQ_URL", "amqp://guest:guest@127.0.0.1:5672/")
	queue := env("APP_RABBITMQ_DLQ", "estimate.calc.dlq")

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

	count, err := ch.QueuePurge(queue, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "purge failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("purged %d message(s) from %s\n", count, queue)
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
