package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PolicyCommand struct {
	OperationID      string      `json:"operation_id"`
	ExpectedRevision string      `json:"expected_revision"`
	Policies         BuyPolicies `json:"policies"`
}
type PolicyContext struct {
	Policies BuyPolicies `json:"policies"`
	Revision string      `json:"revision"`
	Enabled  bool        `json:"enabled"`
}
type PolicyResult struct {
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

func decodePolicyCommand(r io.Reader) (PolicyCommand, error) {
	var in PolicyCommand
	raw, e := io.ReadAll(io.LimitReader(r, 16385))
	if e != nil {
		return in, e
	}
	if len(raw) > 16384 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	fields, e := definitionObject(raw, []string{"operation_id", "expected_revision", "policies"})
	if e != nil {
		return in, e
	}
	if json.Unmarshal(fields["operation_id"], &in.OperationID) != nil || json.Unmarshal(fields["expected_revision"], &in.ExpectedRevision) != nil {
		return in, errEditInput
	}
	p, e := definitionObject(fields["policies"], []string{"warranty_months", "return_days", "warranty_note", "return_note"})
	if e != nil {
		return in, e
	}
	number := func(key string, max int) (int, error) {
		var n json.Number
		if json.Unmarshal(p[key], &n) != nil || len(p[key]) == 0 || p[key][0] == '"' {
			return 0, errEditInput
		}
		s, e := totals.Resolve(totals.Number{Decimal: string(n)})
		if e != nil {
			return 0, errEditInput
		}
		v, e := strconv.ParseInt(s, 10, 64)
		if e != nil || v < 0 || v > int64(max) {
			return 0, errEditInput
		}
		return int(v), nil
	}
	if in.Policies.WarrantyMonths, e = number("warranty_months", 120); e != nil {
		return in, e
	}
	if in.Policies.ReturnDays, e = number("return_days", 365); e != nil {
		return in, e
	}
	note := func(key string) (PolicyNote, error) {
		// The original nested Zod object strips extra note keys; root keys are strict.
		var values map[string]json.RawMessage
		if json.Unmarshal(p[key], &values) != nil || values == nil {
			return PolicyNote{}, errEditInput
		}
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		if _, e := imageObject(p[key], keys...); e != nil {
			return PolicyNote{}, e
		}
		var n PolicyNote
		if string(values["en"]) == "null" || string(values["zh"]) == "null" || json.Unmarshal(values["en"], &n.EN) != nil || json.Unmarshal(values["zh"], &n.ZH) != nil {
			return n, errEditInput
		}
		const whitespace = "\t\n\v\f\r \u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"
		n.EN = strings.Trim(n.EN, whitespace)
		n.ZH = strings.Trim(n.ZH, whitespace)
		if len(utf16.Encode([]rune(n.EN))) > 200 || len(utf16.Encode([]rune(n.ZH))) > 200 {
			return n, errEditInput
		}
		return n, nil
	}
	if in.Policies.WarrantyNote, e = note("warranty_note"); e != nil {
		return in, e
	}
	if in.Policies.ReturnNote, e = note("return_note"); e != nil {
		return in, e
	}
	if !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) {
		return in, errEditInput
	}
	return in, nil
}

type policyStore struct {
	ID       string
	Metadata *string
}

func policyState(tx *gorm.DB, storeID string, write bool) (PolicyContext, policyStore, error) {
	var out PolicyContext
	var store policyStore
	var gate struct {
		Version int
		Enabled bool
	}
	strength := "SHARE"
	if write {
		strength = "UPDATE"
	}
	if tx.Table("catalog_command_capabilities").Where("command='store.buy-policies'").Clauses(clause.Locking{Strength: strength}).Take(&gate).Error != nil || gate.Version != 1 {
		return out, store, ErrSchema
	}
	// Full metadata participates in the version; unrelated store edits must not be lost.
	var row struct {
		ID       string
		Metadata *string
		Bytes    int
	}
	q := tx.Raw("SELECT id,LEFT(metadata,65537) AS metadata,COALESCE(OCTET_LENGTH(metadata),0) AS bytes FROM commerce_stores WHERE id=? AND deleted_at IS NULL FOR "+strength, storeID).Scan(&row)
	if q.Error != nil {
		return out, store, q.Error
	}
	if q.RowsAffected != 1 {
		return out, store, gorm.ErrRecordNotFound
	}
	if row.Bytes > 65536 {
		return out, store, ErrSchema
	}
	store = policyStore{row.ID, row.Metadata}
	var e error
	out.Policies, e = buyPolicies(row.Metadata)
	if e != nil {
		return out, store, e
	}
	out.Revision = imageDigest(store)
	out.Enabled = gate.Enabled
	return out, store, nil
}

func policyAccess(ctx context.Context, tx *gorm.DB, actor identity.User, write bool) error {
	actions := []authorization.Action{authorization.StorePoliciesRead}
	if write {
		actions = append(actions, authorization.StorePoliciesWrite)
	}
	if e := authorization.CheckTx(ctx, tx, actor, actions...); e != nil {
		return e
	}
	return Ready(ctx, tx)
}
func (h StoreHandler) ReadPolicySettings(ctx context.Context, actor identity.User) (PolicyContext, error) {
	var out PolicyContext
	if h.DB == nil || !validProductID(h.StoreID) {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := policyAccess(ctx, tx, actor, false); e != nil {
			return e
		}
		var e error
		out, _, e = policyState(tx, h.StoreID, false)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
func (h StoreHandler) SavePolicySettings(ctx context.Context, actor identity.User, in PolicyCommand) (PolicyResult, error) {
	var out PolicyResult
	// Direct callers receive the same validation and source normalization as HTTP.
	raw, e := json.Marshal(in)
	if e != nil {
		return out, errEditInput
	}
	in, e = decodePolicyCommand(strings.NewReader(string(raw)))
	if e != nil {
		return out, e
	}
	if h.DB == nil || !validProductID(h.StoreID) {
		return out, ErrSchema
	}
	requestSHA := imageDigest(struct {
		Store string
		Input PolicyCommand
	}{h.StoreID, in})
	e = h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := policyAccess(ctx, tx, actor, true); e != nil {
			return e
		}
		before, store, e := policyState(tx, h.StoreID, true)
		if e != nil {
			return e
		}
		var receipt struct{ StoreID, ActorID, RequestSHA256, ResultRevision string }
		e = tx.Table("store_policy_edits").Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.StoreID != h.StoreID || receipt.ActorID != actor.ID || receipt.RequestSHA256 != requestSHA {
				return ErrEditConflict
			}
			out = PolicyResult{receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !before.Enabled {
			return ErrSchema
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		metadata := map[string]json.RawMessage{}
		if store.Metadata != nil && json.Unmarshal([]byte(*store.Metadata), &metadata) != nil {
			return ErrSchema
		}
		if metadata == nil {
			metadata = map[string]json.RawMessage{}
		}
		metadata["buy_policies"], e = json.Marshal(in.Policies)
		if e != nil {
			return e
		}
		updated, e := json.Marshal(metadata)
		if e != nil || len(updated) > 65536 {
			return ErrSchema
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if e = tx.Table("commerce_stores").Where("id=? AND deleted_at IS NULL", h.StoreID).Updates(map[string]any{"metadata": string(updated), "updated_at": now}).Error; e != nil {
			return e
		}
		after, _, e := policyState(tx, h.StoreID, true)
		if e != nil {
			return e
		}
		beforeJSON, _ := json.Marshal(before.Policies)
		afterJSON, _ := json.Marshal(after.Policies)
		if e = tx.Table("store_policy_edits").Create(map[string]any{"operation_id": in.OperationID, "store_id": h.StoreID, "actor_id": actor.ID, "request_sha256": requestSHA, "before_json": string(beforeJSON), "after_json": string(afterJSON), "result_revision": after.Revision, "created_at": now}).Error; e != nil {
			return e
		}
		out = PolicyResult{after.Revision, false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
