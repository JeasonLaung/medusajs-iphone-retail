package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/internal/server"
	"github.com/eurorefurb/retail/internal/sessionstore"
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
	cancelVerify()
	if err != nil {
		log.Fatal("Complete database migration plan and catalog read schema required; run explicit migrations")
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
	router, err := server.RouterWithAuth(db, identity.Options{Sessions: sessions, Origin: os.Getenv("ADMIN_ORIGIN"), Secure: os.Getenv("SESSION_SECURE") == "true"})
	if err != nil {
		log.Fatal("Explicit ADMIN_ORIGIN and matching secure cookie configuration required")
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
