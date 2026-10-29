package customer

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
)

func bridgeKey(raw string) ([]byte, bool) {
	key, err := hex.DecodeString(raw)
	return key, len(raw) == 64 && err == nil && hex.EncodeToString(key) == raw && !bytes.Equal(key, make([]byte, 32))
}

// Proof binds the adapter-observed client address to this exact request. It
// never trusts X-Forwarded-For; the independent bridge key is server-only.
func (h *HTTPHandler) bridge(c *gin.Context) {
	if len(h.bridgeSecret) == 0 {
		c.Next()
		return
	}
	names := []string{"X-Retail-Client-IP", "X-Retail-Bridge-Time", "X-Retail-Bridge-Proof"}
	values := make([]string, 3)
	for i, name := range names {
		v := c.Request.Header.Values(name)
		if len(v) != 1 {
			platform.Fail(c, 403, "customer.bridge_required", "Trusted storefront bridge required")
			return
		}
		values[i] = v[0]
	}
	ip, err := netip.ParseAddr(values[0])
	timestamp, parseErr := strconv.ParseInt(values[1], 10, 64)
	age := time.Now().Unix() - timestamp
	if err != nil || ip.Zone() != "" || parseErr != nil || strconv.FormatInt(timestamp, 10) != values[1] || age < -5 || age > 30 {
		platform.Fail(c, 403, "customer.bridge_required", "Trusted storefront bridge required")
		return
	}
	var body []byte
	if c.Request.Body != nil {
		body, err = io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
		if err != nil {
			platform.InvalidJSON(c, err, "customer.invalid_request", "Valid customer JSON required")
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
	}
	digest := sha256.Sum256(body)
	fields := []string{"retail-customer-bridge-v1", values[1], values[0], c.Request.Method, c.Request.URL.RequestURI(), c.Request.Host, c.GetHeader("Origin"), c.GetHeader("Cookie"), c.GetHeader("X-CSRF-Token"), hex.EncodeToString(digest[:])}
	mac := hmac.New(sha256.New, h.bridgeSecret)
	mac.Write([]byte(strings.Join(fields, "\n")))
	proof, err := hex.DecodeString(values[2])
	if err != nil || len(values[2]) != 64 || hex.EncodeToString(proof) != values[2] || !hmac.Equal(proof, mac.Sum(nil)) {
		platform.Fail(c, 403, "customer.bridge_required", "Trusted storefront bridge required")
		return
	}
	c.Set("customer.client_ip", ip.Unmap().String())
	c.Next()
}

func (h *HTTPHandler) clientIP(c *gin.Context) string {
	if len(h.bridgeSecret) > 0 {
		return c.GetString("customer.client_ip")
	}
	return c.ClientIP()
}
