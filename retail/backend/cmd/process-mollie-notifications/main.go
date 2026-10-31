// Explicit worker only. API startup never queries payment providers or creates orders.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eurorefurb/retail/internal/checkoutconfig"
	"github.com/eurorefurb/retail/internal/mollieworker"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	once := len(os.Args) == 2 && os.Args[1] == "--once"
	if len(os.Args) != 1 && !once {
		log.Fatal("Only --once or no arguments supported")
	}
	payment, err := checkoutconfig.Load(os.Getenv)
	if err != nil {
		log.Fatal("Explicit checkout origins, store, providers and independent payment keyring required")
	}
	webhooks, err := checkoutconfig.Webhooks(os.Getenv, payment)
	if err != nil || payment == nil || len(webhooks.ProviderIDs) != 1 {
		log.Fatal("Explicit CHECKOUT_PAYMENT_ENABLED=true and MOLLIE_NOTIFICATIONS_ENABLED=true required")
	}
	db, err := platform.Open()
	if err != nil {
		log.Fatal("Dedicated payment worker database unavailable")
	}
	pool, err := db.DB()
	if err != nil {
		log.Fatal("Dedicated payment worker pool unavailable")
	}
	defer pool.Close()
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		log.Fatal("Migration plan invalid")
	}
	check, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = schema.CheckApplied(check, pool, plan)
	cancel()
	if err != nil {
		log.Fatal("Complete explicit migrations required")
	}
	worker := mollieworker.Worker{DB: db, StoreID: os.Getenv("STOREFRONT_STORE_ID"), Payment: *payment}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failed := false
	for {
		run, cancel := context.WithTimeout(ctx, 3*time.Minute)
		result, err := worker.RunOnce(run)
		cancel()
		if err != nil {
			log.Print("Payment notification work unavailable; durable work retained")
			failed = true
		} else {
			failed = result.Failed > 0
			if result.Queried+result.Consumed+result.Failed > 0 {
				log.Printf("Payment notifications queried=%d consumed=%d review=%d failed=%d", result.Queried, result.Consumed, result.Review, result.Failed)
			}
		}
		if once {
			break
		}
		timer := time.NewTimer(time.Second)
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
