package catalog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/identity"
	"gorm.io/gorm"
)

// Raw JSON keeps imported numbers and nested values exact through revisions,
// receipts and shallow merges. The source deletes keys using an empty string;
// JSON null clears the entire metadata column, while {} preserves existing keys.
type OptionDefinitionMetadataInput struct {
	OperationID      string          `json:"operation_id"`
	ExpectedRevision string          `json:"expected_revision"`
	Metadata         json.RawMessage `json:"metadata"`
}

func decodeDefinitionMetadata(r io.Reader) (OptionDefinitionMetadataInput, error) {
	var in OptionDefinitionMetadataInput
	raw, e := io.ReadAll(io.LimitReader(r, 262145))
	if e != nil {
		return in, e
	}
	if len(raw) > 262144 || !utf8.Valid(raw) {
		return in, errEditInput
	}
	if _, e = definitionObject(raw, []string{"operation_id", "expected_revision", "metadata"}); e != nil {
		return in, e
	}
	if json.Unmarshal(raw, &in) != nil || !validDefinitionMetadata(in) {
		return in, errEditInput
	}
	return in, nil
}

func validDefinitionMetadata(in OptionDefinitionMetadataInput) bool {
	if !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) || len(in.Metadata) > 262144 || !utf8.Valid(in.Metadata) {
		return false
	}
	if bytes.Equal(bytes.TrimSpace(in.Metadata), []byte("null")) {
		return true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(in.Metadata, &fields) != nil || fields == nil {
		return false
	}
	// Reject duplicate top-level patch keys rather than silently choosing one.
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	_, e := imageObject(in.Metadata, keys...)
	return e == nil
}

func mergeDefinitionMetadata(before *string, patch json.RawMessage) (*string, error) {
	if bytes.Equal(bytes.TrimSpace(patch), []byte("null")) {
		return nil, nil
	}
	merged := map[string]json.RawMessage{}
	if before != nil && json.Unmarshal([]byte(*before), &merged) != nil {
		return nil, ErrSchema
	}
	if merged == nil {
		merged = map[string]json.RawMessage{}
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(patch, &values) != nil || values == nil {
		return nil, errEditInput
	}
	for key, value := range values {
		if bytes.Equal(bytes.TrimSpace(value), []byte(`""`)) {
			delete(merged, key)
		} else {
			merged[key] = value
		}
	}
	raw, e := json.Marshal(merged)
	if e != nil {
		return nil, e
	}
	if len(raw) > 262144 {
		return nil, errEditInput
	}
	text := string(raw)
	return &text, nil
}

func (h Handler) EditOptionDefinitionMetadata(ctx context.Context, id string, actor identity.User, in OptionDefinitionMetadataInput) (OptionDefinitionResult, error) {
	var out OptionDefinitionResult
	if !validProductID(id) || !validDefinitionMetadata(in) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	// Reuse the existing edit receipt action; the command kind is part of the
	// request digest, so its UUID cannot be replayed as a title/value edit.
	requestSHA := imageDigest(struct {
		ID, Kind string
		Input    OptionDefinitionMetadataInput
	}{id, "metadata", in})
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockOptionDefinitions(tx, true)
		if e != nil {
			return e
		}
		var receipt struct{ OptionID, ActorID, Action, RequestSHA256, ResultRevision string }
		e = tx.Table("catalog_option_definition_edits").Where("operation_id=?", in.OperationID).Take(&receipt).Error
		if e == nil {
			if receipt.OptionID != id || receipt.ActorID != actor.ID || receipt.Action != "edit" || receipt.RequestSHA256 != requestSHA {
				return ErrEditConflict
			}
			out = OptionDefinitionResult{id, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !enabled {
			return ErrSchema
		}
		before, e := loadOptionDefinition(tx, id, enabled)
		if e != nil {
			return e
		}
		if before.Revision != in.ExpectedRevision {
			return ErrEditConflict
		}
		metadata, e := mergeDefinitionMetadata(before.MetadataJSON, in.Metadata)
		if e != nil {
			return e
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if e = tx.Table("catalog_options").Where("id=?", id).Updates(map[string]any{"metadata": metadata, "updated_at": now}).Error; e != nil {
			return e
		}
		after, e := loadOptionDefinition(tx, id, enabled)
		if e != nil {
			return e
		}
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		if e = tx.Table("catalog_option_definition_edits").Create(map[string]any{"operation_id": in.OperationID, "option_id": id, "actor_id": actor.ID, "action": "edit", "request_sha256": requestSHA, "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": after.Revision, "created_at": now}).Error; e != nil {
			return e
		}
		out = OptionDefinitionResult{id, after.Revision, false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}
