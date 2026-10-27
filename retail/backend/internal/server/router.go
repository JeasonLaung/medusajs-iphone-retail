package server

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Router(db *gorm.DB) *gin.Engine {
	return router(db, slog.Default())
}

func router(db *gorm.DB, logger *slog.Logger) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(platform.RequestBoundary(logger))
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) { platform.Fail(c, 404, "not_found", "Resource not found") })
	r.NoMethod(func(c *gin.Context) { platform.Fail(c, 405, "method_not_allowed", "Method not allowed") })
	r.GET("/health", func(c *gin.Context) {
		if db == nil {
			platform.Fail(c, 503, "dependency_unavailable", "Database unavailable")
			return
		}
		pool, err := db.DB()
		if err == nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
			defer cancel()
			err = pool.PingContext(ctx)
		}
		if err != nil {
			platform.Fail(c, 503, "dependency_unavailable", "Database unavailable")
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
	admin.Use(platform.PrivateResponse, platform.GuardOrigin)
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
