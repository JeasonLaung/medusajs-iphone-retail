package payments

import (
	"mime"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type MollieWebhookOptions struct{ ProviderIDs []string }
type MollieWebhookHandler struct {
	db        *gorm.DB
	providers map[string]bool
}

// No API keys are needed to accept a known-reference notification. Configuring
// this receiver does not enable payment commands or a worker. Empty options keep
// every original Mollie callback route closed with 503.
func NewMollieWebhook(db *gorm.DB, options MollieWebhookOptions) (*MollieWebhookHandler, error) {
	if len(options.ProviderIDs) == 0 {
		return &MollieWebhookHandler{}, nil
	}
	if db == nil || len(options.ProviderIDs) > len(mollieMethods) {
		return nil, ErrEffectInput
	}
	h := &MollieWebhookHandler{db: db, providers: map[string]bool{}}
	for _, id := range options.ProviderIDs {
		if _, ok := mollieMethods[id]; !ok || h.providers[id] {
			return nil, ErrEffectInput
		}
		h.providers[id] = true
	}
	return h, nil
}

func (h *MollieWebhookHandler) Register(router *gin.Engine) {
	for provider := range mollieMethods {
		provider := provider
		router.POST("/hooks/payment/"+strings.TrimPrefix(provider, "pp_"), func(c *gin.Context) { h.receive(c, provider) })
	}
}

func (h *MollieWebhookHandler) receive(c *gin.Context, provider string) {
	c.Header("Cache-Control", "no-store")
	if h == nil || h.db == nil || !h.providers[provider] {
		c.String(http.StatusServiceUnavailable, "Unavailable")
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/x-www-form-urlencoded" {
		c.String(http.StatusUnsupportedMediaType, "Invalid notification")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if c.Request.ParseForm() != nil || len(c.Request.PostForm) != 1 || len(c.Request.PostForm["id"]) != 1 || !molliePaymentID.MatchString(c.Request.PostForm.Get("id")) {
		c.String(http.StatusBadRequest, "Invalid notification")
		return
	}
	remote := c.Request.PostForm.Get("id")
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		_, err := EnqueueMollieNotification(c.Request.Context(), tx, provider, remote)
		return err
	})
	if err != nil {
		c.String(http.StatusServiceUnavailable, "Unavailable")
		return
	}
	// Known and unknown IDs get the same response only after the transaction
	// commits. Do not leak local ownership, queue state or payment details.
	c.String(http.StatusOK, "OK")
}
