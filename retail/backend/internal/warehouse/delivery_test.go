package warehouse

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/gin-gonic/gin"
)

func TestDeliveryWireRejectsAmbiguousCommands(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `{"operation_id":"2ca432d3-cea0-4a4e-9433-cbc0d047432c","expected_revision":"1","location_id":"location","set_id":"set"}`
	for _, raw := range []string{`null`, `{}`, valid + `{}`, strings.Replace(valid, `"set_id":"set"`, `"set_id":"set","set_id":"other"`, 1), strings.Replace(valid, `"set_id":"set"`, `"set_id":null`, 1), strings.Replace(valid, `"set_id":"set"`, `"set_id":"set","unknown":"x"`, 1), strings.Replace(valid, `"expected_revision":"1"`, `"expected_revision":"01"`, 1), strings.Replace(valid, `"location_id":"location"`, `"location_id":"other"`, 1), strings.Replace(valid, `"set_id":"set"`, `"set_id":"`+string([]byte{0xff})+`"`, 1), strings.Repeat(" ", 16385)} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("admin_user", identity.User{})
		c.Params = gin.Params{{Key: "id", Value: "location"}}
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")
		(Handler{}).DeliveryHTTP(c)
		if w.Code != 400 {
			t.Fatalf("bad command reached storage: %d", w.Code)
		}
	}
}
