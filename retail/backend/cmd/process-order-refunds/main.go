// Explicit refund worker. API startup only stages durable commands.
package main

import (
	"context"
	"github.com/eurorefurb/retail/internal/checkoutconfig"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/refunds"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/joho/godotenv"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	_ = godotenv.Load()
	once := len(os.Args) == 2 && os.Args[1] == "--once"
	if len(os.Args) != 1 && !once {
		log.Fatal("Only --once or no arguments supported")
	}
	payment, e := checkoutconfig.Load(os.Getenv)
	if e != nil {
		log.Fatal("Explicit checkout configuration required")
	}
	options, e := checkoutconfig.Refunds(os.Getenv, payment)
	if e != nil || !options.Enabled {
		log.Fatal("Explicit ORDER_REFUNDS_ENABLED=true and payment configuration required")
	}
	db, e := platform.Open()
	if e != nil {
		log.Fatal("Dedicated refund database unavailable")
	}
	pool, e := db.DB()
	if e != nil {
		log.Fatal("Dedicated refund pool unavailable")
	}
	defer pool.Close()
	plan, e := schema.Load(migrations.Files)
	if e != nil {
		log.Fatal("Explicit migration plan unavailable")
	}
	check, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	e = schema.CheckApplied(check, pool, plan)
	cancel()
	if e != nil {
		log.Fatal("Complete explicit migrations required")
	}
	service := refunds.Service{DB: db, StoreID: options.StoreID, Ring: options.Ring, Adapters: options.Adapters}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cursor := ""
	failed := false
	for {
		run, cancel := context.WithTimeout(ctx, 7*time.Minute)
		result, next, e := service.RunDue(run, cursor)
		cancel()
		cursor = next
		failed = e != nil || result.Failed > 0
		if e != nil {
			log.Print("Refund work unavailable; durable instructions retained")
		} else if result.Selected > 0 {
			log.Printf("Refund instructions selected=%d processed=%d failed=%d", result.Selected, result.Processed, result.Failed)
		}
		if once {
			break
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
	if failed {
		os.Exit(1)
	}
}
