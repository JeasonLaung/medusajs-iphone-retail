package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/eurorefurb/retail/internal/importer"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/joho/godotenv"
)

func main() {
	bundle := flag.String("bundle", "", "path to a private source snapshot")
	flag.Parse()
	if *bundle == "" {
		log.Fatal("--bundle is required")
	}
	_ = godotenv.Load()
	db, err := platform.Open()
	if err != nil {
		log.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	counts, err := importer.ImportCatalog(db, *bundle)
	if err != nil {
		log.Fatal(err)
	}
	_ = json.NewEncoder(os.Stdout).Encode(counts)
}
