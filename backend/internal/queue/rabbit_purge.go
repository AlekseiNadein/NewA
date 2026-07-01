package queue

import (
	"fmt"
	"strings"

	amqp "github.com/rabbitmq/amqp091-go"
)

// PurgeQueue removes all ready messages from a RabbitMQ queue.
func PurgeQueue(rabbitURL, queueName string) (int, error) {
	queueName = strings.TrimSpace(queueName)
	if queueName == "" {
		return 0, fmt.Errorf("queue name is empty")
	}
	if strings.TrimSpace(rabbitURL) == "" {
		return 0, fmt.Errorf("rabbit url is empty")
	}

	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return 0, err
	}
	defer ch.Close()

	count, err := ch.QueuePurge(queueName, false)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// PurgeQueues removes all ready messages from the given RabbitMQ queues.
func PurgeQueues(rabbitURL string, queueNames []string) (map[string]int, error) {
	if strings.TrimSpace(rabbitURL) == "" {
		return nil, fmt.Errorf("rabbit url is empty")
	}
	if len(queueNames) == 0 {
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

	purged := make(map[string]int, len(queueNames))
	for _, queueName := range queueNames {
		queueName = strings.TrimSpace(queueName)
		if queueName == "" {
			continue
		}
		count, err := ch.QueuePurge(queueName, false)
		if err != nil {
			return purged, fmt.Errorf("purge %s: %w", queueName, err)
		}
		purged[queueName] = int(count)
	}
	return purged, nil
}
