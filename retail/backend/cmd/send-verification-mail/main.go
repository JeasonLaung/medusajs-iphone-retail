// Explicit new-customer verification worker. Never enabled by merely starting
// the API, importing historical mail, or loading a .env file.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/maildelivery"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	if os.Getenv("CUSTOMER_VERIFICATION_MAIL_ENABLED") != "true" {
		log.Fatal("Explicit CUSTOMER_VERIFICATION_MAIL_ENABLED=true required")
	}
	once := len(os.Args) == 2 && os.Args[1] == "--once"
	if len(os.Args) != 1 && !once {
		log.Fatal("Only --once or no arguments supported")
	}
	source := os.Getenv("REGISTRATION_MAIL_SOURCE")
	var sender *maildelivery.Resend
	var err error
	var merchantRing *protected.Ring
	switch source {
	case "database":
		merchantRing, err = protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON"))
		if err != nil {
			log.Fatal("Explicit merchant encryption keyring required")
		}
	case "", "environment":
		sender, err = maildelivery.NewResend(os.Getenv("REGISTRATION_RESEND_API_KEY"), os.Getenv("REGISTRATION_MAIL_FROM"), os.Getenv("REGISTRATION_MAIL_REPLY_TO"))
		if err != nil {
			log.Fatal("Explicit registration mail sender configuration required")
		}
	default:
		log.Fatal("REGISTRATION_MAIL_SOURCE must be database or environment")
	}
	ring, err := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
	if err != nil {
		log.Fatal("Explicit customer encryption keyring required")
	}
	db, err := platform.Open()
	if err != nil {
		log.Fatal("Dedicated mail database unavailable")
	}
	pool, err := db.DB()
	if err != nil {
		log.Fatal("Dedicated mail database pool unavailable")
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
		log.Fatal("Complete explicit schema migrations required")
	}
	store, err := customer.NewVerificationDelivery(db, ring, os.Getenv("STOREFRONT_ORIGIN"))
	if err != nil {
		log.Fatal("Explicit storefront origin required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for {
		run, cancel := context.WithTimeout(ctx, 15*time.Second)
		var result customer.Delivery
		var err error
		if source == "database" {
			sender, err = (notificationmail.Store{DB: db, Ring: merchantRing}).RegistrationSender(run)
		}
		if err == nil {
			result, err = store.DispatchVerification(run, sender)
		}
		cancel()
		if err != nil {
			log.Print("Verification mail dispatch unavailable; durable intent retained")
		} else if result.ID != "" {
			log.Printf("Verification mail job=%s state=%s attempts=%d", result.ID, result.Status, result.Attempts)
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
