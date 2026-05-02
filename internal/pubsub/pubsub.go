package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

type SimpleQueueType string

type AckType int

const (
	Ack AckType = iota
	NackRequeue
	NackDiscard
)

const (
	SimpleQueueTypeDurable   SimpleQueueType = "durable"
	SimpleQueueTypeTransient SimpleQueueType = "transient"
)

func PublishJSON[T any](ch *amqp.Channel, exchange, key string, val T) error {
	marshalledValue, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("could not marshal value: %v", err)
	}

	err = ch.PublishWithContext(context.Background(), exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        marshalledValue,
	})
	if err != nil {
		return fmt.Errorf("failed to publish message: %v", err)
	}

	return nil
}

func SubscribeJSON[T any](
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType, // an enum to represent "durable" or "transient"
	handler func(T) AckType,
) (ackType AckType, err error) {
	channel, _, err := DeclareAndBind(conn, exchange, queueName, key, queueType)
	if err != nil {
		return Ack, fmt.Errorf("failed to declare and bind: %v", err)
	}
	newChan, err := channel.Consume("", "", false, false, false, false, nil)
	if err != nil {
		return Ack, fmt.Errorf("failed to consume: %v", err)
	}

	go func() {
		for d := range newChan {
			var val T
			err := json.Unmarshal(d.Body, &val)
			if err != nil {
				fmt.Printf("failed to unmarshal value: %v", err)
				continue
			}
			ackType := handler(val)
			switch ackType {
			case Ack:
				d.Ack(false)
				fmt.Println("acked message")
			case NackRequeue:
				d.Nack(false, true)
				fmt.Println("nacked message and requeued")
			case NackDiscard:
				fmt.Println("nacked message and discarded")
				d.Nack(false, false)
			}
		}
	}()

	return Ack, nil
}

func DeclareAndBind(
	conn *amqp.Connection,
	exchange,
	queueName,
	key string,
	queueType SimpleQueueType,
) (*amqp.Channel, amqp.Queue, error) {
	ch, err := conn.Channel()
	if err != nil {
		return nil, amqp.Queue{}, fmt.Errorf("failed to create channel: %v", err)
	}

	err = ch.ExchangeDeclare(routing.ExchangePerilDlx, "fanout", true, false, false, false, nil)
	if err != nil {
		return nil, amqp.Queue{}, fmt.Errorf("failed to declare exchange: %v", err)
	}

	queue, err := ch.QueueDeclare(queueName, queueType == SimpleQueueTypeDurable, queueType == SimpleQueueTypeTransient, queueType == SimpleQueueTypeTransient, false, amqp.Table{"x-dead-letter-exchange": routing.ExchangePerilDlx})
	if err != nil {
		return nil, amqp.Queue{}, fmt.Errorf("failed to declare queue: %v", err)
	}

	err = ch.QueueBind(queue.Name, key, exchange, false, nil)
	if err != nil {
		return nil, amqp.Queue{}, fmt.Errorf("failed to bind queue: %v", err)
	}

	return ch, queue, nil
}
