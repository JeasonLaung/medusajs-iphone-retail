package buyback

import (
	"context"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

func (s Store) Register(group *gin.RouterGroup) {
	group.GET("/buyback/prices", s.PricesHTTP)
	group.POST("/buyback/estimate", s.EstimateHTTP)
}
func unavailable(c *gin.Context) {
	platform.Fail(c, 503, "buyback.unavailable", "Buyback pricing unavailable")
}
func invalid(c *gin.Context) {
	platform.Fail(c, 400, "buyback.invalid_input", "Invalid buyback request")
}
func (s Store) PricesHTTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	query, e := url.ParseQuery(c.Request.URL.RawQuery)
	if e != nil || c.Request.URL.ForceQuery || len(query) > 1 {
		invalid(c)
		return
	}
	product := ""
	if len(query) > 0 {
		values, ok := query["product_id"]
		if !ok || len(values) != 1 || values[0] != "" && !validID(values[0], 100) {
			invalid(c)
			return
		}
		product = values[0]
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	rows, e := s.Prices(ctx, product)
	if e != nil {
		unavailable(c)
		return
	}
	c.JSON(200, gin.H{"prices": rows})
}
func (s Store) EstimateHTTP(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery {
		invalid(c)
		return
	}
	if strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
		invalid(c)
		return
	}
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 16385))
	if e != nil || len(raw) > 16384 {
		invalid(c)
		return
	}
	in, e := ParseEstimate(raw)
	if e != nil {
		invalid(c)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	result, e := s.Estimate(ctx, in)
	if e != nil {
		unavailable(c)
		return
	}
	c.JSON(200, gin.H{"amount": result.Amount, "status": result.Status, "visual": result.Visual, "faults": result.Faults, "provisional": result.Provisional, "currency_code": "eur"})
}
