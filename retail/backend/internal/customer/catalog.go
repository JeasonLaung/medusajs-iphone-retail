package customer

import (
	"crypto/subtle"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/eurorefurb/retail/internal/customergroups"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Catalog prices are resolved inside the same transaction as the product and
// price reads. Neither customer IDs nor group IDs are accepted from the browser.
func (h *HTTPHandler) Catalog(c *gin.Context) {
	if !h.originOK(c, c.Request.Method == "POST") {
		return
	}
	token, ok, _ := h.cookie(c, "session")
	if !ok {
		h.fail(c, ErrUnauthorized)
		return
	}
	principal, err := h.sessions.Resolve(c.Request.Context(), sessionstore.Customer, token)
	if err != nil {
		h.fail(c, customerSessionError(err))
		return
	}
	// Resolve current identity before entering the catalog, then recheck after
	// waiting for its account lock so revocation cannot authorize a stale read.
	if _, err := h.store.Current(c.Request.Context(), token); err != nil {
		h.fail(c, err)
		return
	}
	projection := catalog.StoreHandler{DB: h.store.db, StoreID: h.storeID, CustomerGroups: func(tx *gorm.DB) ([]string, error) {
		var current record
		if tx.Clauses(clause.Locking{Strength: "SHARE"}).Where("id=?", principal.UserID).Take(&current).Error != nil {
			return nil, ErrUnauthorized
		}
		fresh, err := h.sessions.Resolve(c.Request.Context(), sessionstore.Customer, token)
		if err != nil {
			return nil, customerSessionError(err)
		}
		if fresh.UserID != current.ID || !current.Enabled || current.VerifiedAt == nil || current.AuthVersion == 0 || subtle.ConstantTimeCompare([]byte(fresh.Version), []byte(version(current))) != 1 {
			return nil, ErrUnauthorized
		}
		return customergroups.ForAccountTx(tx, current.ID)
	}}
	if c.Request.Method == "POST" {
		projection.Search(c)
	} else if c.Param("id") != "" {
		projection.Product(c)
	} else {
		projection.Products(c)
	}
}
