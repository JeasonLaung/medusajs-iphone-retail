package server

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/cart"
	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/devices"
	"github.com/eurorefurb/retail/internal/fulfillment"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/inventory"
	"github.com/eurorefurb/retail/internal/orders"
	"github.com/eurorefurb/retail/internal/payments"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/tax"
	"github.com/eurorefurb/retail/internal/warehouse"
	"github.com/eurorefurb/retail/internal/warehouseusers"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func Router(db *gorm.DB) *gin.Engine {
	return router(db, slog.Default())
}

func router(db *gorm.DB, logger *slog.Logger) *gin.Engine {
	return routerWithIdentity(db, logger, identity.New(db))
}

func RouterWithAuth(db *gorm.DB, options identity.Options, cartOptions ...cart.HTTPOptions) (*gin.Engine, error) {
	h, err := identity.NewWithOptions(db, options)
	if err != nil {
		return nil, err
	}
	shop := &cart.HTTPHandler{}
	if len(cartOptions) > 1 {
		return nil, cart.ErrConfiguration
	}
	if len(cartOptions) == 1 {
		shop, err = cart.NewHTTP(db, cartOptions[0])
		if err != nil {
			return nil, err
		}
	}
	return routerWithIdentity(db, slog.Default(), h, shop), nil
}

func routerWithIdentity(db *gorm.DB, logger *slog.Logger, auth *identity.Handler, shops ...*cart.HTTPHandler) *gin.Engine {
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
		ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
		defer cancel()
		if err == nil {
			err = pool.PingContext(ctx)
		}
		if err == nil {
			err = auth.Dependency(ctx)
		}
		if err == nil {
			err = catalog.Ready(ctx, db)
		}
		if err == nil {
			err = inventory.Ready(ctx, db)
		}
		if err == nil {
			err = orders.Ready(ctx, db)
		}
		if err == nil {
			err = payments.Ready(ctx, db)
		}
		if err == nil {
			err = fulfillment.Ready(ctx, db)
		}
		if err == nil {
			err = fulfillment.GeoReady(ctx, db)
		}
		if err == nil {
			err = authorization.Ready(ctx, db)
		}
		if err == nil {
			err = cart.Ready(ctx, db)
		}
		if err == nil {
			err = tax.Ready(ctx, db)
		}
		if err == nil {
			err = promotion.Ready(ctx, db)
		}
		if err != nil {
			platform.Fail(c, 503, "dependency_unavailable", "Application dependency unavailable")
			return
		}
		c.JSON(200, gin.H{"status": "ok", "backend": "gin", "time": time.Now().UTC()})
	})
	h := catalog.Handler{DB: db}
	public := r.Group("/api/store")
	public.GET("/device-photos/:photo_id", (devices.Handler{DB: db}).PhotoHTTP)
	shop := &cart.HTTPHandler{}
	if len(shops) == 1 && shops[0] != nil {
		shop = shops[0]
	}
	shop.Register(public)
	store := catalog.StoreHandler{DB: db, StoreID: os.Getenv("STOREFRONT_STORE_ID")}
	public.POST("/search", store.Search)
	public.GET("/products", store.Products)
	public.GET("/products/:id", store.Product)
	public.GET("/product-categories", store.Categories)
	public.GET("/product-categories/:id", store.Category)
	public.GET("/collections", store.Collections)
	public.GET("/collections/:id", store.Collection)
	public.GET("/regions", h.Regions)
	public.GET("/locales", store.Locales)
	public.GET("/customer-service", store.Contacts)
	public.GET("/buy-policies", store.Policies)
	public.GET("/regions/:id", h.StoreRegion)
	admin := r.Group("/api/admin")
	admin.Use(platform.PrivateResponse, platform.GuardOrigin)
	admin.POST("/auth/challenge", auth.Challenge)
	admin.POST("/auth/login", auth.Login)
	private := admin.Group("")
	private.Use(auth.Require)
	private.GET("/auth/me", auth.Me)
	private.GET("/warehouse/locations/:location_id/device-intake/:category/variants", (devices.Handler{DB: db}).WarehouseIntakeVariantsHTTP)
	private.POST("/warehouse/locations/:location_id/device-intake/:category/photos", (devices.Handler{DB: db}).WarehousePhotoHTTP)
	private.GET("/warehouse/locations/:location_id/device-intake/:category/products/:id/variants/:variant_id", (devices.Handler{DB: db}).WarehouseIntakeHTTP)
	private.POST("/warehouse/locations/:location_id/device-intake/:category/products/:id/variants/:variant_id", (devices.Handler{DB: db}).WarehouseIntakeHTTP)
	private.GET("/warehouse/locations/:location_id/devices/:category", (devices.Handler{DB: db}).WarehouseDevicesHTTP)
	private.POST("/warehouse/locations/:location_id/devices/:category/:device_id", (devices.Handler{DB: db}).WarehouseDeviceCommandHTTP)
	private.POST("/device-photos", authorization.Handler{DB: db}.Require(authorization.DeviceRead, authorization.DeviceIntake, authorization.ProductRead, authorization.InventoryRead, authorization.WarehouseRead), (devices.Handler{DB: db}).PhotoUploadHTTP)
	private.POST("/auth/logout", auth.Logout)
	access := authorization.Handler{DB: db}
	private.GET("/auth/permissions", access.Snapshot)
	private.GET("/warehouse/access", access.WarehouseSnapshot)
	private.GET("/products/:id/variants/:variant_id/device-intake", (devices.Handler{DB: db}).IntakeContextHTTP)
	private.POST("/products/:id/variants/:variant_id/device-intake", (devices.Handler{DB: db}).IntakeHTTP)
	private.GET("/device-sources", (devices.SourceHandler{DB: db, StoreID: os.Getenv("STOREFRONT_STORE_ID")}).DirectoryHTTP)
	private.POST("/device-sources", (devices.SourceHandler{DB: db, StoreID: os.Getenv("STOREFRONT_STORE_ID")}).CreateHTTP)
	private.GET("/products/:id/variants/:variant_id/device-tracking", (devices.Handler{DB: db}).TrackingHTTP)
	private.POST("/products/:id/variants/:variant_id/device-tracking", (devices.Handler{DB: db}).EnrollHTTP)
	private.GET("/warehouse/locations/:location_id/inventory", (inventory.Handler{DB: db}).WarehouseInventory)
	private.POST("/warehouse/locations/:location_id/inventory/:id/stock", (inventory.Handler{DB: db}).UpdateWarehouseStock)
	private.GET("/warehouses", access.Require(authorization.WarehouseRead), (warehouse.Handler{DB: db}).Directory)
	private.POST("/warehouses", access.Require(authorization.WarehouseRead, authorization.WarehouseWrite), (warehouse.Handler{DB: db}).Command)
	private.GET("/warehouse-users", access.Require(authorization.Read, authorization.WarehouseRead), (warehouseusers.Handler{DB: db}).Directory)
	private.POST("/warehouse-users", access.Require(authorization.Read, authorization.Write, authorization.WarehouseRead, authorization.WarehouseWrite), (warehouseusers.Handler{DB: db}).Command)
	private.GET("/authorization", access.Require(authorization.Read), access.Directory)
	private.POST("/authorization/commands", access.Require(authorization.Write), access.Command)
	private.GET("/orders", access.Require(authorization.OrderRead), (orders.Handler{DB: db}).List)
	private.GET("/orders/:id", access.Require(authorization.OrderRead), (orders.Handler{DB: db}).Detail)
	private.GET("/orders/:id/amounts", access.Require(authorization.OrderRead, authorization.OrderAmountRead), (orders.Handler{DB: db}).Amounts)
	private.GET("/orders/:id/payments", access.Require(authorization.OrderRead, authorization.OrderPaymentRead), (orders.Handler{DB: db}).Payments)
	private.GET("/orders/:id/fulfillments", access.Require(authorization.OrderRead, authorization.OrderFulfillmentRead), (orders.Handler{DB: db}).Fulfillments)
	private.GET("/products", access.Require(authorization.ProductRead), h.AdminProducts)
	private.GET("/products/:id", access.Require(authorization.ProductRead), h.AdminProduct)
	private.GET("/products/:id/variants", access.Require(authorization.ProductRead), h.AdminVariants)
 private.GET("/products/:id/variant-creation", access.Require(authorization.ProductRead, authorization.ProductWrite), h.VariantCreateHTTP)
 private.POST("/products/:id/variants", access.Require(authorization.ProductRead, authorization.ProductWrite), h.VariantCreateHTTP)
	private.GET("/products/:id/variants/:variant_id/prices", access.Require(authorization.ProductRead, authorization.PriceRead), h.AdminVariantPrices)
 private.GET("/products/:id/variants/:variant_id/price-creation", access.Require(authorization.ProductRead, authorization.PriceRead), h.PriceCreateHTTP)
 private.POST("/products/:id/variants/:variant_id/prices", access.Require(authorization.ProductRead, authorization.PriceRead, authorization.PriceWrite), h.PriceCreateHTTP)
 private.GET("/products/:id/variants/:variant_id/prices/:price_id", access.Require(authorization.ProductRead, authorization.PriceRead), h.PriceEditHTTP)
 private.POST("/products/:id/variants/:variant_id/prices/:price_id", access.Require(authorization.ProductRead, authorization.PriceRead, authorization.PriceWrite), h.PriceEditHTTP)
	private.GET("/products/:id/variants/:variant_id/inventory", access.Require(authorization.ProductRead, authorization.InventoryRead), (inventory.Handler{DB: db}).AdminVariant)
	private.POST("/inventory-levels/:id/stock", access.Require(authorization.InventoryRead, authorization.StockWrite), (inventory.Handler{DB: db}).UpdateStock)
	private.PATCH("/products/:id", access.Require(authorization.ProductWrite), h.UpdateProduct)
	if root := os.Getenv("PUBLIC_ASSET_ROOT"); root != "" {
		r.Static("/assets", root)
	}
	// Private uploads are never mounted as a static directory.
	if root := os.Getenv("LEGACY_PUBLIC_UPLOAD_ROOT"); root != "" {
		r.Static("/static", root)
	}
	return r
}
