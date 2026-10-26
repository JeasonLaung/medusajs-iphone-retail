package server

import (
	"net/http"
	"os"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Router(db *gorm.DB) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	})
	r.GET("/health", func(c *gin.Context) {
		pool, err := db.DB()
		if err == nil {
			err = pool.PingContext(c.Request.Context())
		}
		if err != nil {
			c.JSON(503, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(200, gin.H{"status": "ok", "backend": "gin", "time": time.Now().UTC()})
	})
	h := catalog.Handler{DB: db}
	public := r.Group("/api/store")
	public.GET("/products", h.Products)
	public.GET("/products/:id", h.Product)
	public.GET("/product-categories", h.Categories)
	public.GET("/collections", h.Collections)
	public.GET("/regions", h.Regions)
	identity := identity.New(db)
	admin := r.Group("/api/admin")
	admin.Use(platform.GuardOrigin)
	admin.POST("/auth/login", identity.Login)
	private := admin.Group("")
	private.Use(identity.Require)
	private.GET("/auth/me", identity.Me)
	private.POST("/auth/logout", identity.Logout)
	private.GET("/products", h.AdminProducts)
	private.GET("/products/:id", h.AdminProduct)
	private.PATCH("/products/:id", h.UpdateProduct)
	if root := os.Getenv("PUBLIC_ASSET_ROOT"); root != "" {
		r.Static("/assets", root)
	}
	// Private uploads are never mounted as a static directory.
	if root := os.Getenv("LEGACY_PUBLIC_UPLOAD_ROOT"); root != "" {
		r.Static("/static", root)
	}
	return r
}
