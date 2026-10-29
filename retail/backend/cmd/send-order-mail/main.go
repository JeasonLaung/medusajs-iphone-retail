// Explicit worker. Starting the API never sends order emails.
package main

import (
	"context"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/ordermail"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
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
	if os.Getenv("ORDER_MAIL_ENABLED") != "true" {
		log.Fatal("Explicit ORDER_MAIL_ENABLED=true required")
	}
	once := len(os.Args) == 2 && os.Args[1] == "--once"
	if len(os.Args) != 1 && !once {
		log.Fatal("Only --once or no arguments supported")
	}
	paymentRing, e := protected.ParseKeyring(os.Getenv("PAYMENT_KEY_ACTIVE"), os.Getenv("PAYMENT_KEYRING_JSON"))
	if e != nil {
		log.Fatal("Persistent payment encryption keyring required")
	}
	merchantRing, e := protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON"))
	if e != nil {
		log.Fatal("Explicit merchant encryption keyring required")
	}
	db, e := platform.Open()
	if e != nil {
		log.Fatal("Dedicated order mail database unavailable")
	}
	pool, e := db.DB()
	if e != nil {
		log.Fatal("Dedicated order mail pool unavailable")
	}
	defer pool.Close()
	plan, e := schema.Load(migrations.Files)
	if e != nil {
		log.Fatal("Migration plan invalid")
	}
	check, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	e = schema.CheckApplied(check, pool, plan)
	cancel()
	if e != nil {
		log.Fatal("Complete explicit migrations required")
	}
	store := ordermail.Store{DB: db, Ring: paymentRing}
	mailbox := notificationmail.Store{DB: db, Ring: merchantRing}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		run, cancel := context.WithTimeout(ctx, 15*time.Second)
		sender, err := mailbox.OrderSender(run)
		var result ordermail.Result
		if err == nil {
			result, err = store.Dispatch(run, sender)
		}
		cancel()
		if err != nil {
			log.Print("Order mail dispatch unavailable; intent retained")
		} else if result.ID != "" {
			log.Printf("Order mail job=%s state=%s attempts=%d", result.ID, result.Status, result.Attempts)
		}
		if once {
			if err != nil {
				os.Exit(1)
			}
			return
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
