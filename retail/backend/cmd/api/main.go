package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/customer"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/notificationmail"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/eurorefurb/retail/internal/tax"
	"github.com/eurorefurb/retail/migrations"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	if os.Getenv("APP_ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
		if os.Getenv("SESSION_SECURE") != "true" {
			log.Fatal("Production authentication requires secure cookies")
		}
	}
	db, err := platform.Open()
	if err != nil {
		log.Fatal("Database connection unavailable; verify the dedicated MySQL configuration")
	}
	pool, _ := db.DB()
	defer pool.Close()
	// Readiness requires an explicitly migrated schema; never mutate it here.
	plan, err := schema.Load(migrations.Files)
	if err != nil {
		log.Fatal("Compiled database migration plan invalid")
	}
	verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 5*time.Second)
	err = schema.CheckApplied(verifyCtx, pool, plan)
	if err == nil {
		err = catalog.Ready(verifyCtx, db)
	}
	if err == nil {
		err = inventory.Ready(verifyCtx, db)
	}
	if err == nil {
		err = orders.Ready(verifyCtx, db)
	}
	if err == nil {
		err = payments.Ready(verifyCtx, db)
	}
	if err == nil {
		err = fulfillment.Ready(verifyCtx, db)
	}
	if err == nil {
		err = fulfillment.GeoReady(verifyCtx, db)
	}
	if err == nil {
		err = authorization.Ready(verifyCtx, db)
	}
	if err == nil {
		err = cart.Ready(verifyCtx, db)
	}
	if err == nil {
		err = tax.Ready(verifyCtx, db)
	}
	if err == nil {
		err = promotion.Ready(verifyCtx, db)
	}
	cancelVerify()
	if err != nil {
		log.Fatal("Complete database migration plan and domain read schemas required; run explicit migrations")
	}
	bind := os.Getenv("APP_BIND")
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "9100"
	}
	setup, cancelSetup := context.WithTimeout(context.Background(), 3*time.Second)
	sessions, redisClient, err := sessionstore.Open(setup, os.Getenv("SESSION_REDIS_URL"), os.Getenv("SESSION_REDIS_PREFIX"))
	cancelSetup()
	if err != nil {
		log.Fatal("Dedicated session Redis unavailable or configuration invalid")
	}
	defer redisClient.Close()
	initialPayment, err := checkoutOptions(os.Getenv)
	if err != nil {
		log.Fatal("Explicit checkout payment origins, independent keyring and Mollie credentials required")
	}
	var cartOptions []cart.HTTPOptions
	if origin := os.Getenv("STOREFRONT_ORIGIN"); origin != "" {
		manualProviders := []string{}
		if raw := os.Getenv("MANUAL_SHIPPING_PROVIDER_IDS"); raw != "" {
			manualProviders = strings.Split(raw, ",")
		}
		cartOptions = append(cartOptions, cart.HTTPOptions{InitialPayment: initialPayment, ManualProviderIDs: manualProviders, Sessions: sessions, Origin: origin, StoreID: os.Getenv("STOREFRONT_STORE_ID"), Secure: os.Getenv("SESSION_SECURE") == "true"})
	}
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: os.Getenv("ADMIN_ORIGIN"), Secure: os.Getenv("SESSION_SECURE") == "true"}, cartOptions...)
	if err != nil {
		log.Fatal("Explicit application origins, shop binding and matching secure cookie configuration required")
	}
	customers := &customer.HTTPHandler{}
	switch os.Getenv("CUSTOMER_AUTH_ENABLED") {
	case "", "false":
	case "true":
		if os.Getenv("CUSTOMER_BRIDGE_KEY") == "" {
			log.Fatal("Customer authentication requires an independent storefront bridge key")
		}
		ring, keyErr := protected.ParseKeyring(os.Getenv("CUSTOMER_KEY_ACTIVE"), os.Getenv("CUSTOMER_KEYRING_JSON"))
		if keyErr != nil {
			log.Fatal("Explicit customer encryption keyring required")
		}
		customers, err = customer.NewHTTP(db, customer.HTTPOptions{Sessions: sessions, Ring: ring, Origin: os.Getenv("STOREFRONT_ORIGIN"), Secure: os.Getenv("SESSION_SECURE") == "true", BridgeKey: os.Getenv("CUSTOMER_BRIDGE_KEY"), StoreID: os.Getenv("STOREFRONT_STORE_ID")})
		if err != nil {
			log.Fatal("Customer origin and secure cookie configuration invalid")
		}
	default:
		log.Fatal("CUSTOMER_AUTH_ENABLED must be true or false")
	}
	customers.Register(router.Group("/api/store/customer"))
	switch os.Getenv("NOTIFICATION_MAIL_ENABLED") {
	case "", "false":
	case "true":
		ring, err := protected.ParseKeyring(os.Getenv("MERCHANT_KEY_ACTIVE"), os.Getenv("MERCHANT_KEYRING_JSON"))
		if err != nil {
			log.Fatal("Explicit merchant encryption keyring required for notification mail configuration")
		}
		adminAuth, err := identity.NewWithOptions(db, identity.Options{Sessions: sessions, Origin: os.Getenv("ADMIN_ORIGIN"), Secure: os.Getenv("SESSION_SECURE") == "true"})
		if err != nil {
			log.Fatal("Notification mailbox authentication unavailable")
		}
		private := router.Group("/api/admin")
		private.Use(adminAuth.Require)
		notificationmail.Store{DB: db, Ring: ring}.Register(private)
	default:
		log.Fatal("NOTIFICATION_MAIL_ENABLED must be true or false")
	}

	srv := &http.Server{Addr: net.JoinHostPort(bind, port), Handler: router, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Printf("Gin API listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Print(err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = srv.Shutdown(shutdown); err != nil {
		log.Print(err)
	}
}
