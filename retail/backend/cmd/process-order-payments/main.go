// Explicit native order-payment worker. API startup never runs it or migrates.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eurorefurb/retail/internal/checkoutconfig"
	"github.com/eurorefurb/retail/internal/orderpayments"
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
	payment, e := checkoutconfig.Load(os.Getenv)
	if e != nil {
		log.Fatal("Explicit checkout configuration required")
	}
	service, e := checkoutconfig.OrderPayments(os.Getenv, payment)
	if e != nil || service == nil {
		log.Fatal("Explicit ORDER_PAYMENTS_ENABLED=true and notification/payment configuration required")
	}
	db, e := platform.Open()
	if e != nil {
		log.Fatal("Dedicated order payment database unavailable")
	}
	pool, e := db.DB()
	if e != nil {
		log.Fatal("Dedicated order payment pool unavailable")
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
	service.DB = db
	w := orderpayments.Worker{Service: *service}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		run, cancel := context.WithTimeout(ctx, 10*time.Minute)
		r, e := w.RunOnce(run)
		cancel()
		if e != nil {
			log.Print("Order payment work unavailable; durable instructions retained")
		} else if r.Selected+r.Queried+r.Consumed+r.Failed > 0 {
			log.Printf("Order payment instructions=%d processed=%d queried=%d consumed=%d failed=%d", r.Selected, r.Processed, r.Queried, r.Consumed, r.Failed)
		}
		if once {
			if e != nil || r.Failed > 0 {
				os.Exit(1)
			}
			return
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
