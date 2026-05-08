package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/bootdotdev/learn-pub-sub-starter/internal/gamelogic"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/pubsub"
	"github.com/bootdotdev/learn-pub-sub-starter/internal/routing"
	amqp "github.com/rabbitmq/amqp091-go"
)

func handlerPause(gs *gamelogic.GameState) func(routing.PlayingState) pubsub.AckType {
	return func(ps routing.PlayingState) pubsub.AckType {
		defer fmt.Print("> ")
		gs.HandlePause(ps)
		return pubsub.Ack
	}
}

func handlerMove(gs *gamelogic.GameState, channel *amqp.Channel, username string) func(gamelogic.ArmyMove) pubsub.AckType {
	return func(move gamelogic.ArmyMove) pubsub.AckType {
		defer fmt.Print("> ")
		outcome := gs.HandleMove(move)
		switch outcome {
		case gamelogic.MoveOutComeSafe, gamelogic.MoveOutcomeMakeWar:
			err := pubsub.PublishJSON(channel, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix+"."+username, gamelogic.RecognitionOfWar{
				Attacker: move.Player,
				Defender: gs.GetPlayerSnap(),
			})
			if err != nil {
				fmt.Printf("error publishing war recognition: %v\n", err)
				return pubsub.NackRequeue
			}
			fmt.Println("war recognition published")
			return pubsub.Ack
		default:
			return pubsub.NackDiscard
		}
	}
}

func publishGameLog(ch *amqp.Channel, username, message string) error {
	return pubsub.PublishGob(ch, routing.ExchangePerilTopic, routing.GameLogSlug+"."+username, routing.GameLog{
		CurrentTime: time.Now(),
		Message:     message,
		Username:    username,
	})
}

func handlerWar(gs *gamelogic.GameState, ch *amqp.Channel, username string) func(gamelogic.RecognitionOfWar) pubsub.AckType {
	return func(war gamelogic.RecognitionOfWar) pubsub.AckType {
		defer fmt.Print("> ")
		outcome, winner, loser := gs.HandleWar(war)
		switch outcome {
		case gamelogic.WarOutcomeNotInvolved:
			return pubsub.NackRequeue
		case gamelogic.WarOutcomeNoUnits:
			return pubsub.NackDiscard
		case gamelogic.WarOutcomeOpponentWon:
			message := fmt.Sprintf("%s won a war against %s", winner, loser)
			err := publishGameLog(ch, username, message)
			if err != nil {
				fmt.Printf("error publishing game log: %v\n", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.WarOutcomeYouWon:
			message := fmt.Sprintf("%s won a war against %s", winner, loser)
			err := publishGameLog(ch, username, message)
			if err != nil {
				fmt.Printf("error publishing game log: %v\n", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		case gamelogic.WarOutcomeDraw:
			message := fmt.Sprintf("The war between %s and %s ended in a draw", winner, loser)
			err := publishGameLog(ch, username, message)
			if err != nil {
				fmt.Printf("error publishing game log: %v\n", err)
				return pubsub.NackRequeue
			}
			return pubsub.Ack
		default:
			fmt.Printf("error: unknown war outcome: %v\n", outcome)
			return pubsub.NackDiscard
		}
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

	_, err = pubsub.SubscribeJSON(conn, routing.ExchangePerilDirect, routing.PauseKey+"."+username, routing.PauseKey, pubsub.SimpleQueueTypeTransient, handlerPause(gs))
	if err != nil {
		log.Fatalf("Failed to subscribe to pause: %v", err)
	}

	_, err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, routing.ArmyMovesPrefix+"."+username, routing.ArmyMovesPrefix+".*", pubsub.SimpleQueueTypeTransient, handlerMove(gs, ch, username))
	if err != nil {
		log.Fatalf("Failed to subscribe to army moves: %v", err)
	}
	log.Printf("Subscribed to %s", routing.ArmyMovesPrefix+".*")

	_, err = pubsub.SubscribeJSON(conn, routing.ExchangePerilTopic, routing.WarRecognitionsPrefix, routing.WarRecognitionsPrefix+".*", pubsub.SimpleQueueTypeDurable, handlerWar(gs, ch, username))
	if err != nil {
		log.Fatalf("Failed to subscribe to war recognitions: %v", err)
	}

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
