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

func testMappedMerchantConfig(t *testing.T, db *sql.DB, ctx context.Context) {
	t.Helper()
	plain := []byte(" artificial merchant 中文 🚀 ")
	sourceKey := sha256.Sum256([]byte("artificial-source-config"))
	block, _ := aes.NewCipher(sourceKey[:])
	aead, _ := cipher.NewGCM(block)
	nonce := bytes.Repeat([]byte{11}, 12)
	sealed := aead.Seal(nil, nonce, plain, nil)
	legacy := base64.StdEncoding.EncodeToString(nonce) + "." + base64.StdEncoding.EncodeToString(sealed[len(sealed)-16:]) + "." + base64.StdEncoding.EncodeToString(sealed[:len(sealed)-16])
	keys, _ := legacysecret.New(map[string]string{"mollie": "artificial-source-config"})
	ring, _ := protected.New("fixture-target", map[string][]byte{"fixture-target": bytes.Repeat([]byte{21}, 32)})
	converter, _ := rowimport.NewConfigSecrets(ring, keys)
	cell := func(value string) rowimport.Cell { raw, _ := json.Marshal(value); return rowimport.Cell{Raw: raw} }
	source := rowimport.SourceRow{"id": cell("mapped-mollie-config"), "environment": cell("test"), "api_key": cell(legacy), "redirect_url": cell("https://example.invalid/return"), "enabled": {Raw: json.RawMessage(`false`)}, "created_at": cell("2026-10-27T00:00:00.123456Z"), "updated_at": cell("2026-10-27T00:00:00.123456Z"), "deleted_at": {SQLNull: true}}
	bound, err := converter.Convert("mollie_config", source)
	if err != nil {
		t.Fatal("merchant source conversion failed")
	}
	var columns, markers []string
	for _, col := range bound.Columns {
		columns = append(columns, "`"+col+"`")
		markers = append(markers, "?")
	}
	statement := "INSERT INTO relational_fixture.`" + bound.Table + "` (" + strings.Join(columns, ",") + ") VALUES(" + strings.Join(markers, ",") + ")"
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal("artificial transaction unavailable")
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, statement, bound.Values...); err != nil {
		t.Fatal("merchant binary insert failed")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal("merchant rollback failed")
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relational_fixture.mollie_configs WHERE id='mapped-mollie-config'").Scan(&count); err != nil || count != 0 {
		t.Fatal("merchant rollback left a row")
	}
	if _, err := db.ExecContext(ctx, statement, bound.Values...); err != nil {
		t.Fatal("merchant committed insert failed")
	}
	var envelope []byte
	var enabled bool
	if err := db.QueryRowContext(ctx, "SELECT api_key,enabled FROM relational_fixture.mollie_configs WHERE id='mapped-mollie-config'").Scan(&envelope, &enabled); err != nil {
		t.Fatal("merchant binary read failed")
	}
	if enabled || bytes.Contains(envelope, plain) || bytes.Equal(envelope, []byte(legacy)) {
		t.Fatal("merchant secret was not independently sealed")
	}
	context := protected.Context{Purpose: "merchant-config", Table: "mollie_configs", Column: "api_key", Row: "mapped-mollie-config"}
	opened, err := ring.Open(context, envelope)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatal("merchant persisted envelope differs")
	}
	context.Column = "other"
	if _, err := ring.Open(context, envelope); err == nil {
		t.Fatal("persisted context not bound")
	}
	source["api_key"] = cell("invalid-legacy-ciphertext")
	if result, err := converter.Convert("mollie_config", source); err != rowimport.ErrProtection || len(result.Values) != 0 {
		t.Fatal("invalid source produced SQL arguments")
	}
}
