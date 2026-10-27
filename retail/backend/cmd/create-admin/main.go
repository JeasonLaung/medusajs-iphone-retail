package main

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	_ = godotenv.Load()
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	if !strings.Contains(email, "@") || len(email) > 254 || len(password) < 12 || len(password) > 72 {
		log.Fatal("Set ADMIN_EMAIL and ADMIN_PASSWORD (12–72 bytes) in the environment; do not pass credentials on the command line")
	}
	db, err := platform.Open()
	if err != nil {
		log.Fatal("Database connection unavailable; verify the dedicated MySQL configuration")
	}
	pool, _ := db.DB()
	defer pool.Close()
	encoded, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	user := identity.User{ID: "usr_" + uuid.NewString(), Email: email, PasswordHash: string(encoded), Name: "Administrator", Role: "admin", Enabled: true, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = db.Create(&user).Error; err != nil {
		log.Fatal("Unable to create account (it may already exist)")
	}
	log.Print("Created independent migration Admin account")
}
