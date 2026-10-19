package main

import (
	"log"
	"time"

	"github.com/firepager/backend/api"
	"github.com/firepager/backend/db"
)

func main() {
	log.Println("Starting API Service...")

	// Quick wait for dependencies to be up in local env
	time.Sleep(5 * time.Second)

	db.InitDB()

	r := api.SetupRouter()
	r.Run(":8080")
}
