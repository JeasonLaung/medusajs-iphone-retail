package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string { return "admin_users" }

type Session struct {
	TokenHash string `gorm:"primaryKey"`
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (Session) TableName() string { return "admin_sessions" }

type attempt struct {
	Count int
	Until time.Time
}
type Handler struct {
	DB       *gorm.DB
	mu       sync.Mutex
	attempts map[string]attempt
}

func New(db *gorm.DB) *Handler { return &Handler{DB: db, attempts: map[string]attempt{}} }
func hash(value string) string { return hex.EncodeToString(sum(value)) }
func sum(value string) []byte  { v := sha256.Sum256([]byte(value)); return v[:] }

func (h *Handler) Login(c *gin.Context) {
	var input struct {
		Email    string `json:"email" binding:"required,email,max=254"`
		Password string `json:"password" binding:"required,max=72"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		platform.InvalidJSON(c, err, "invalid_login", "Email and password are required")
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	key := hash(c.ClientIP() + ":" + input.Email)
	h.mu.Lock()
	now := time.Now()
	for k, v := range h.attempts {
		if now.After(v.Until) {
			delete(h.attempts, k)
		}
	}
	a := h.attempts[key]
	if a.Count >= 8 || len(h.attempts) > 10000 {
		h.mu.Unlock()
		platform.Fail(c, 429, "login_rate_limited", "Too many login attempts; try again later")
		return
	}
	a.Count++
	a.Until = now.Add(15 * time.Minute)
	h.attempts[key] = a
	h.mu.Unlock()
	var user User
	err := h.DB.Where("email=? AND enabled=?", input.Email, true).First(&user).Error
	// Keep a bcrypt comparison on missing accounts as well, without exposing whether an email exists.
	encoded := user.PasswordHash
	if err != nil {
		encoded = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	}
	valid := bcrypt.CompareHashAndPassword([]byte(encoded), []byte(input.Password)) == nil
	if err != nil || !valid {
		platform.Fail(c, 401, "invalid_credentials", "Email or password is incorrect")
		return
	}
	bytes := make([]byte, 32)
	if _, err = rand.Read(bytes); err != nil {
		platform.DBError(c, err)
		return
	}
	token := hex.EncodeToString(bytes)
	session := Session{TokenHash: hash(token), UserID: user.ID, CreatedAt: now.UTC(), ExpiresAt: now.Add(12 * time.Hour).UTC()}
	if err = h.DB.Create(&session).Error; err != nil {
		platform.DBError(c, err)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("retail_admin_session", token, 12*3600, "/", "", os.Getenv("SESSION_SECURE") == "true", true)
	c.JSON(200, gin.H{"user": user})
}
func (h *Handler) Require(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	token, err := c.Cookie("retail_admin_session")
	if err != nil || len(token) != 64 {
		platform.Fail(c, 401, "unauthorized", "Sign in to continue")
		return
	}
	var s Session
	if err = h.DB.Where("token_hash=? AND expires_at>?", hash(token), time.Now().UTC()).First(&s).Error; err != nil {
		platform.Fail(c, 401, "unauthorized", "Session expired")
		return
	}
	var user User
	if err = h.DB.Where("id=? AND enabled=?", s.UserID, true).First(&user).Error; err != nil {
		platform.Fail(c, 401, "unauthorized", "Session expired")
		return
	}
	// No supplier/warehouse roles are admitted until their data scopes are ported.
	if user.Role != "admin" {
		platform.Fail(c, 403, "forbidden", "This account is not authorized for this application")
		return
	}
	c.Set("admin_id", user.ID)
	c.Set("admin_user", user)
	c.Next()
}
func (h *Handler) Me(c *gin.Context) { c.JSON(200, gin.H{"user": c.MustGet("admin_user")}) }
func (h *Handler) Logout(c *gin.Context) {
	token, _ := c.Cookie("retail_admin_session")
	if err := h.DB.Where("token_hash=?", hash(token)).Delete(&Session{}).Error; err != nil {
		platform.DBError(c, err)
		return
	}
	c.SetCookie("retail_admin_session", "", -1, "/", "", os.Getenv("SESSION_SECURE") == "true", true)
	c.JSON(200, gin.H{"success": true})
}
