package customer

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
)

// Flags use canonical "true"/"false" strings at this strict string-only JSON
// boundary. They become booleans in the public Address projection and SQL.
var addressInputFields = []string{"first_name", "last_name", "company", "address_1", "address_2", "city", "postal_code", "province", "country_code", "phone", "is_default_billing", "is_default_shipping"}

func (h *HTTPHandler) ListAddresses(c *gin.Context) {
	if !h.originOK(c, false) {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	addresses, err := h.store.Addresses(ctx, token)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"addresses": addresses})
}
func (h *HTTPHandler) CreateAddress(c *gin.Context) { h.saveAddress(c, "") }
func (h *HTTPHandler) UpdateAddress(c *gin.Context) { h.saveAddress(c, c.Param("id")) }
func (h *HTTPHandler) saveAddress(c *gin.Context, id string) {
	fields, ok := h.input(c, addressInputFields...)
	if !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	address, err := h.store.SaveAddress(ctx, token, id, fields)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"address": address})
}
func (h *HTTPHandler) RemoveAddress(c *gin.Context) {
	if _, ok := h.input(c); !ok || !h.originOK(c, true) {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	token, ok := h.addressWriteToken(c, ctx)
	if !ok {
		return
	}
	if err := h.store.DeleteAddress(ctx, token, c.Param("id")); err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true})
}
func (h *HTTPHandler) addressWriteToken(c *gin.Context, ctx context.Context) (string, bool) {
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return "", false
	}
	if err := h.sessions.VerifyCSRF(ctx, sessionstore.Customer, token, customerCSRF(c)); err != nil {
		h.fail(c, err)
		return "", false
	}
	return token, true
}
