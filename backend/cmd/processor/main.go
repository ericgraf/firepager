package main

import (
	"context"
	"log"
	"time"

	"github.com/firepager/backend/db"
	"github.com/firepager/backend/mq"
)

func main() {
	log.Println("Starting Processor Worker Service...")

	// Quick wait for dependencies to be up in local env
	time.Sleep(5 * time.Second)

	db.InitDB()
	go mq.RunNotificationWorker(context.Background())
	go mq.StartReceiverStatusConsumer()

	// Run consumer
	for {
		mq.StartConsumer()
		time.Sleep(10 * time.Second)
	}
}
