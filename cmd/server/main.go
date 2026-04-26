package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to create channel: %v", err)
	}
	defer ch.Close()
	defer conn.Close()
	gamelogic.PrintServerHelp()

	ch, queue, err := pubsub.DeclareAndBind(conn, routing.ExchangePerilTopic, routing.GameLogSlug, routing.GameLogSlug, pubsub.SimpleQueueTypeDurable)
	if err != nil {
		log.Fatalf("Failed to declare and bind queue: %v", err)
	}
	fmt.Printf("Declared and bound queue %s to exchange %s with key %s\n", queue.Name, routing.ExchangePerilTopic, routing.GameLogSlug)
loop:
	for {
		words := gamelogic.GetInput()
		if len(words) == 0 {
			continue
		}
		command := words[0]
		switch command {
		case "pause":
			err = pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: true,
			})
			if err != nil {
				log.Fatalf("Failed to publish pause message: %v", err)
			}

			fmt.Println("Published pause message")
		case "resume":
			err = pubsub.PublishJSON(ch, routing.ExchangePerilDirect, routing.PauseKey, routing.PlayingState{
				IsPaused: false,
			})
			if err != nil {
				log.Fatalf("Failed to publish resume message: %v", err)
			}
			fmt.Println("Published resume message")
		case "quit":
			fmt.Println("exiting")
			break loop
		default:
			fmt.Println("I don't understand the command")
		}
	}

	fmt.Println("Connected to RabbitMQ")
	// create a channel to listen for signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	received := <-sigCh
	fmt.Println("Received signal:", received.String())
}
