// Standalone maintenance worker; never starts alongside API by implication.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/eurorefurb/retail/internal/buyback"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/schema"
	"github.com/eurorefurb/retail/migrations"
	"github.com/joho/godotenv"
)

func nextDaily(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
func main() {
	apply := flag.Bool("apply", false, "delete eligible MySQL photo bytes and expire metadata")
	daily := flag.Bool("daily", false, "run daily at 03:00 in the process timezone; requires -apply")
	flag.Parse()
	if flag.NArg() != 0 || *daily && !*apply {
		log.Fatal("Use -apply with -daily; no positional arguments supported")
	}
	_ = godotenv.Load()
	db, e := platform.Open()
	if e != nil {
		log.Fatal("Photo expiry database unavailable")
	}
	pool, e := db.DB()
	if e != nil {
		log.Fatal("Photo expiry pool unavailable")
	}
	defer pool.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	plan, e := schema.Load(migrations.Files)
	if e != nil {
		log.Fatal("Photo expiry migration plan invalid")
	}
	check, cancel := context.WithTimeout(ctx, 10*time.Second)
	e = schema.CheckApplied(check, pool, plan)
	cancel()
	if e != nil {
		log.Fatal("Photo expiry requires applied migrations")
	}
	for {
		if *daily {
			timer := time.NewTimer(time.Until(nextDaily(time.Now())))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
		run, cancel := context.WithTimeout(ctx, 10*time.Minute)
		now := time.Now()
		after := ""
		totals := buyback.ExpiryPage{}
		for {
			page, err := buyback.ExpirePhotoPage(run, db, now, after, 100, *apply)
			if err != nil {
				e = err
				break
			}
			totals.Requests += page.Requests
			totals.Photos += page.Photos
			totals.Expired += page.Expired
			totals.LegacyFiles += page.LegacyFiles
			if page.Next == "" {
				e = nil
				break
			}
			after = page.Next
		}
		cancel()
		if e != nil {
			if ctx.Err() != nil {
				return
			}
			if !*daily {
				log.Fatal("Photo expiry failed; committed earlier pages remain retryable")
			}
			log.Print("Photo expiry failed; will retry at the next scheduled run")
		} else {
			raw, _ := json.Marshal(totals)
			fmt.Printf("Photo expiry apply=%t %s\n", *apply, raw)
		}
		if !*daily {
			return
		}
	}
}
