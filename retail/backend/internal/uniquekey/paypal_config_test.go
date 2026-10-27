package uniquekey

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eurorefurb/retail/internal/legacysecret"
	"github.com/eurorefurb/retail/internal/protected"
	"github.com/eurorefurb/retail/internal/rowimport"
)

func testNestedPayPalConfigSQL(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	text := func(value string) rowimport.Cell { raw, _ := json.Marshal(value); return rowimport.Cell{Raw: raw} }
	sourceKey := sha256.Sum256([]byte("artificial-paypal-source"))
	block, _ := aes.NewCipher(sourceKey[:])
	aead, _ := cipher.NewGCM(block)
	legacy := func(plain string, nonceByte byte) string {
		nonce := bytes.Repeat([]byte{nonceByte}, 12)
		sealed := aead.Seal(nil, nonce, []byte(plain), nil)
		return "enc:v1:" + base64.StdEncoding.EncodeToString(nonce) + ":" + base64.StdEncoding.EncodeToString(sealed[len(sealed)-16:]) + ":" + base64.StdEncoding.EncodeToString(sealed[:len(sealed)-16])
	}
	nested, _ := json.Marshal(map[string]any{"active_environment": "live", "credentials": map[string]any{
		"live":    map[string]string{"client_id": "fixture-live-client", "client_secret": legacy(" live secret 中文 ", 21), "clientSecret": legacy(" live secret 中文 ", 22)},
		"sandbox": map[string]string{"clientId": "fixture-sandbox-client", "clientSecret": legacy(" sandbox secret ", 23)},
	}})
	source := rowimport.SourceRow{"id": text("fixture-nested-paypal"), "environment": text("live"), "status": text("connected"),
		"shared_id": {SQLNull: true}, "auth_code": {SQLNull: true}, "seller_client_id": text("fixture-live-client"),
		"seller_client_secret": text(legacy(" live secret 中文 ", 24)), "seller_merchant_id": {SQLNull: true}, "seller_email": {SQLNull: true},
		"app_access_token": {SQLNull: true}, "app_access_token_expires_at": {SQLNull: true}, "metadata": {Raw: nested},
		"created_at": text("2026-10-27T00:00:00.123456Z"), "updated_at": text("2026-10-27T00:00:00.123456Z"), "deleted_at": {SQLNull: true}}
	reader, _ := legacysecret.NewPayPal("artificial-paypal-source")
	ring, _ := protected.New("fixture-paypal", map[string][]byte{"fixture-paypal": bytes.Repeat([]byte{71}, 32)})
	converter, _ := rowimport.NewConfigSecretsWithPayPal(ring, nil, reader)
	bound, err := converter.Convert("paypal_connection", source)
	if err != nil {
		t.Fatal("nested PayPal fixture conversion failed")
	}
	var columns, markers []string
	for _, column := range bound.Columns {
		columns = append(columns, "`"+column+"`")
		markers = append(markers, "?")
	}
	statement := "INSERT INTO relational_fixture.`" + bound.Table + "` (" + strings.Join(columns, ",") + ") VALUES(" + strings.Join(markers, ",") + ")"
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("nested PayPal transaction unavailable")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, statement, bound.Values...); err != nil {
		t.Fatal("nested PayPal staged insert failed")
	}
	if tx.Rollback() != nil {
		t.Fatal("nested PayPal rollback failed")
	}
	var count int
	if db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relational_fixture.paypal_connections WHERE id='fixture-nested-paypal'").Scan(&count) != nil || count != 0 {
		t.Fatal("nested PayPal rollback left a row")
	}
	if _, err := db.ExecContext(ctx, statement, bound.Values...); err != nil {
		t.Fatal("nested PayPal committed insert failed")
	}
	var secret, metadata []byte
	var auth, token []byte
	if db.QueryRowContext(ctx, "SELECT seller_client_secret,metadata,auth_code,app_access_token FROM relational_fixture.paypal_connections WHERE id='fixture-nested-paypal'").Scan(&secret, &metadata, &auth, &token) != nil || auth != nil || token != nil {
		t.Fatal("nested PayPal SQL NULL/binary readback failed")
	}
	context := protected.Context{Purpose: "merchant-config", Table: "paypal_connections", Column: "seller_client_secret", Row: "fixture-nested-paypal"}
	plain, err := ring.Open(context, secret)
	if err != nil || string(plain) != " live secret 中文 " {
		t.Fatal("PayPal mirror retains old ciphertext")
	}
	context.Column = "metadata"
	plain, err = ring.Open(context, metadata)
	if err != nil || bytes.Contains(plain, []byte("enc:v1:")) {
		t.Fatal("PayPal nested metadata retains old ciphertext")
	}
	var projected struct {
		Credentials map[string]map[string]string `json:"credentials"`
	}
	if json.Unmarshal(plain, &projected) != nil || projected.Credentials["live"]["client_secret"] != " live secret 中文 " || projected.Credentials["live"]["clientSecret"] != " live secret 中文 " || projected.Credentials["sandbox"]["clientSecret"] != " sandbox secret " {
		t.Fatal("PayPal nested SQL projection differs")
	}
	if bytes.Contains(metadata, []byte("secret")) {
		t.Fatal("PayPal nested plaintext stored without encryption")
	}
	context.Row = "other-row"
	if _, err := ring.Open(context, metadata); err == nil {
		t.Fatal("nested PayPal SQL envelope row not bound")
	}
}
