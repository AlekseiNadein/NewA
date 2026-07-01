package queue

import (
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

var defaultQueues = []string{
	"estimate.calc.main",
	"estimate.calc.dlq",
	"estimate.calc.retry.5s",
	"estimate.calc.retry.30s",
	"estimate.calc.retry.120s",
}

// CalcQueueNames returns RabbitMQ queue names used by the estimate calc pipeline.
func CalcQueueNames() []string {
	return append([]string(nil), defaultQueues...)
}

// CalcRetryQueueNames returns retry queue names for the estimate calc pipeline.
func CalcRetryQueueNames() []string {
	return []string{
		"estimate.calc.retry.5s",
		"estimate.calc.retry.30s",
		"estimate.calc.retry.120s",
	}
}

func InspectQueueDepths(rabbitURL string) (map[string]int, error) {
	if strings.TrimSpace(rabbitURL) == "" {
		return map[string]int{}, nil
	}
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	depths := make(map[string]int, len(defaultQueues))
	for _, name := range defaultQueues {
		q, err := ch.QueueDeclarePassive(name, true, false, false, false, nil)
		if err != nil {
			depths[name] = -1
			continue
		}
		depths[name] = q.Messages
	}
	return depths, nil
}
