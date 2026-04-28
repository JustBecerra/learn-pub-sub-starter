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

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) {
	return func(ps routing.PlayingState) {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
	}
}

func handlerMove(gs *gamelogic.GameState) func(gamelogic.ArmyMove) {
	return func(move gamelogic.ArmyMove) {
		defer fmt.Print("> ")
		gs.HandleMove(move)
	}
}

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

	username, err := gamelogic.ClientWelcome()
	if err != nil {
		log.Fatalf("Failed to get username: %v", err)
	}
	gs := gamelogic.NewGameState(username)

	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilDirect, routing.PauseKey+"."+username, routing.PauseKey, pubsub.SimpleQueueTypeTransient, handlerPause(gs))
	if err != nil {
		log.Fatalf("Failed to subscribe to pause: %v", err)
	}

	err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, routing.ArmyMovesPrefix+"."+username, routing.ArmyMovesPrefix+".*", pubsub.SimpleQueueTypeTransient, handlerMove(gs))
	if err != nil {
		log.Fatalf("Failed to subscribe to army moves: %v", err)
	}
	log.Printf("Subscribed to %s", routing.ArmyMovesPrefix+".*")

loop:
	for {
		words := gamelogic.GetInput()
		if len(words) == 0 {
			continue
		}
		command := words[0]
		switch command {
		case "spawn":
			gs.CommandSpawn(words)
			fmt.Println("spawned a unit")
		case "move":
			move, err := gs.CommandMove(words)
			if err != nil {
				fmt.Println(err)
				continue
			}
			err = pubsub.PublishJSON(ch, routing.ExchangePerilTopic, routing.ArmyMovesPrefix+"."+username, move)
			if err != nil {
				fmt.Printf("error publishing move: %v\n", err)
			}
			fmt.Println("move was published")
		case "status":
			gs.CommandStatus()
		case "spam":
			fmt.Println("Spamming not allowed yet!")
		case "quit":
			gamelogic.PrintQuit()
			break loop
		default:
			fmt.Println("error: unknown command")
		}

	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

}
