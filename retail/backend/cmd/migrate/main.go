package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/joho/godotenv"
)

func main() {
	// Parse and validate ALL files before opening the explicit target database.
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		log.Fatal(err)
	}
	_ = godotenv.Load()
	db, err := platform.Open()
	if err != nil {
		log.Fatal("migration target connection failed")
	}
	pool, err := db.DB()
	if err != nil {
		log.Fatal("migration target connection unavailable")
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	result, err := schema.Apply(ctx, pool, plan)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Applied %d, skipped %d migrations\n", result.Applied, result.Skipped)
}
