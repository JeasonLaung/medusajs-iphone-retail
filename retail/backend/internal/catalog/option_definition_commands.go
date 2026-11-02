package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/eurorefurb/retail/internal/identity"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OptionDefinitionContext struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	MetadataJSON   *string           `json:"metadata_json"`
	Values         []definitionValue `json:"values"`
	ActiveProducts int               `json:"active_products"`
	Exclusive      bool              `json:"is_exclusive"`
	Deleted        bool              `json:"deleted"`
	Enabled        bool              `json:"enabled"`
	Revision       string            `json:"revision"`
}

type OptionDefinitionRemoveInput struct {
	OperationID      string `json:"operation_id"`
	ExpectedRevision string `json:"expected_revision"`
}

type OptionDefinitionResult struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

type OptionDefinitionsPage struct {
	Options []OptionDefinitionContext `json:"options"`
	Count   int64                     `json:"count"`
	Offset  int                       `json:"offset"`
	Limit   int                       `json:"limit"`
	Enabled bool                      `json:"enabled"`
}

// This row is also the serialization point for product-option membership
// writes. Keep this lock before any product lock in every participating path.
func lockOptionDefinitions(tx *gorm.DB, write bool) (bool, error) {
	var capability struct {
		Version int
		Enabled bool
	}
	strength := "SHARE"
	if write {
		strength = "UPDATE"
	}
	if tx.Table("catalog_command_capabilities").Where("command='product.option-definitions'").Clauses(clause.Locking{Strength: strength}).Take(&capability).Error != nil || capability.Version != 1 {
		return false, ErrSchema
	}
	return capability.Enabled, nil
}

func loadOptionDefinition(tx *gorm.DB, id string, enabled bool) (OptionDefinitionContext, error) {
	return loadDefinitionState(tx, id, enabled, false)
}
func loadDefinitionState(tx *gorm.DB, id string, enabled, includeOther bool) (OptionDefinitionContext, error) {
	out := OptionDefinitionContext{ID: id, Enabled: enabled, Values: []definitionValue{}}
	var option struct {
		ID, Title   string
		Metadata    *string
		IsExclusive bool
		DeletedAt   *time.Time
	}
	query := tx.Table("catalog_options").Select("id,title,metadata,is_exclusive,deleted_at").Where("id=?", id)
	if !includeOther {
		query = query.Where("is_exclusive=FALSE AND deleted_at IS NULL")
	}
	if e := query.Take(&option).Error; e != nil {
		return out, e
	}
	out.Title = option.Title
	out.Exclusive = option.IsExclusive
	out.Deleted = option.DeletedAt != nil
	if option.Metadata != nil {
		var object map[string]json.RawMessage
		if json.Unmarshal([]byte(*option.Metadata), &object) != nil {
			return out, ErrSchema
		}
		if object != nil {
			raw, err := json.Marshal(object)
			if err != nil {
				return out, ErrSchema
			}
			text := string(raw)
			out.MetadataJSON = &text
		}
	}
	if tx.Table("catalog_option_values").Select("id,value,`rank`").Where("option_id=? AND deleted_at IS NULL", id).Order("`rank` IS NULL,`rank`,id").Limit(101).Scan(&out.Values).Error != nil || len(out.Values) > 100 {
		return out, ErrSchema
	}
	var members []struct{ ID, ProductProductOptionID, ProductOptionValueID string }
	// Match the source guard: non-deleted memberships also block value removal
	// when the associated product itself is soft deleted.
	if tx.Table("catalog_product_option_values m").Select("m.id,m.product_product_option_id,m.product_option_value_id").Joins("JOIN catalog_option_values v ON v.id=m.product_option_value_id").Where("v.option_id=? AND v.deleted_at IS NULL AND m.deleted_at IS NULL", id).Order("m.id").Limit(10001).Scan(&members).Error != nil || len(members) > 10000 {
		return out, ErrSchema
	}
	counts := map[string]int64{}
	for _, member := range members {
		counts[member.ProductOptionValueID]++
	}
	for i := range out.Values {
		out.Values[i].Memberships = counts[out.Values[i].ID]
	}
	var links []struct{ ID, ProductID string }
	// Whole-definition deletion has a different source predicate: only links
	// to non-deleted products block it. Keep identities in the revision too.
	if tx.Table("catalog_product_options l").Select("l.id,l.product_id").Joins("JOIN catalog_products p ON p.id=l.product_id AND p.deleted_at IS NULL").Where("l.product_option_id=? AND l.deleted_at IS NULL", id).Order("l.id").Limit(10001).Scan(&links).Error != nil || len(links) > 10000 {
		return out, ErrSchema
	}
	out.ActiveProducts = len(links)
	out.Revision = imageDigest(struct {
		Definition OptionDefinitionContext
		Members    any
		Links      any
	}{out, members, links})
	return out, nil
}

func (h Handler) ReadOptionDefinition(ctx context.Context, id string, actor identity.User) (OptionDefinitionContext, error) {
	var out OptionDefinitionContext
	if !validProductID(id) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockOptionDefinitions(tx, false)
		if e != nil {
			return e
		}
		out, e = loadOptionDefinition(tx, id, enabled)
		return e
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}

func (h Handler) ListOptionDefinitions(ctx context.Context, actor identity.User, query optionDefinitionQuery) (OptionDefinitionsPage, error) {
	out := OptionDefinitionsPage{Options: []OptionDefinitionContext{}, Offset: query.Offset, Limit: 20}
	if query.Offset < 0 || query.Offset > 100000 {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	e := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := imageAccess(ctx, tx, actor); e != nil {
			return e
		}
		enabled, e := lockOptionDefinitions(tx, false)
		if e != nil {
			return e
		}
		out.Enabled = enabled
		filtered := func() *gorm.DB {
			base := tx.Table("catalog_options")
			// Preserve the established shared-list default; explicit scope can select
			// product-specific rows without exposing those rows to shared commands.
			if query.Exclusive == nil {
				base = base.Where("is_exclusive=FALSE")
			}
			if !query.WithDeleted {
				base = base.Where("deleted_at IS NULL")
			}
			return applyDefinitionPredicates(base, query)
		}
		orders := map[string]string{"": "title ASC,id ASC", "title_asc": "title ASC,id ASC", "title_desc": "title DESC,id ASC", "created_at_asc": "created_at ASC,id ASC", "created_at_desc": "created_at DESC,id ASC", "updated_at_asc": "updated_at ASC,id ASC", "updated_at_desc": "updated_at DESC,id ASC"}
		order, ok := orders[query.Sort]
		if !ok {
			return errEditInput
		}
		if e = filtered().Count(&out.Count).Error; e != nil {
			return e
		}
		var ids []string
		if e = filtered().Order(order).Offset(query.Offset).Limit(out.Limit).Pluck("id", &ids).Error; e != nil {
			return e
		}
		for _, id := range ids {
			option, e := loadDefinitionState(tx, id, enabled, true)
			if e != nil {
				return e
			}
			out.Options = append(out.Options, option)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}

func (h Handler) CreateOptionDefinition(ctx context.Context, actor identity.User, in OptionDefinitionInput) (OptionDefinitionResult, error) {
	return h.saveOptionDefinition(ctx, "", actor, in)
}

func (h Handler) EditOptionDefinition(ctx context.Context, id string, actor identity.User, in OptionDefinitionInput) (OptionDefinitionResult, error) {
	if !validProductID(id) {
		return OptionDefinitionResult{}, errEditInput
	}
	return h.saveOptionDefinition(ctx, id, actor, in)
}

func (h Handler) saveOptionDefinition(ctx context.Context, target string, actor identity.User, in OptionDefinitionInput) (OptionDefinitionResult, error) {
	var out OptionDefinitionResult
	if !validOptionDefinition(in) || target == "" && in.ExpectedRevision != strings.Repeat("0", 64) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	action := "edit"
	if target == "" {
		action = "create"
	}
	requestSHA := imageDigest(struct {
		Target string
		Action string
		Input  OptionDefinitionInput
	}{target, action, in})
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
			if receipt.ActorID != actor.ID || receipt.Action != action || receipt.RequestSHA256 != requestSHA || target != "" && receipt.OptionID != target {
				return ErrEditConflict
			}
			out = OptionDefinitionResult{receipt.OptionID, receipt.ResultRevision, true}
			return nil
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if !enabled {
			return ErrSchema
		}
		before := OptionDefinitionContext{Enabled: enabled, Values: []definitionValue{}}
		id := target
		if target != "" {
			before, e = loadOptionDefinition(tx, target, enabled)
			if e != nil {
				return e
			}
			if before.Revision != in.ExpectedRevision {
				return ErrEditConflict
			}
		} else {
			id = "opt_" + uuid.NewString()
		}
		plan, e := planDefinitionValues(before.Values, in)
		if e != nil {
			return e
		}
		// The source model has a partial unique index on the title of active
		// global definitions. Exclusive groups may share that title. All global
		// definition commands hold the same mutex before checking or writing.
		var sameTitle int64
		if e = tx.Table("catalog_options").Where("title=? AND is_exclusive=FALSE AND deleted_at IS NULL AND id<>?", in.Title, id).Count(&sameTitle).Error; e != nil {
			return e
		}
		if sameTitle != 0 {
			return ErrOptionDefinitionTitle
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if target == "" {
			e = tx.Table("catalog_options").Create(map[string]any{"id": id, "title": in.Title, "is_exclusive": false, "created_at": now, "updated_at": now}).Error
		} else {
			e = tx.Table("catalog_options").Where("id=?", id).Updates(map[string]any{"title": in.Title, "updated_at": now}).Error
		}
		if e != nil {
			return e
		}
		for _, value := range plan.Keep {
			if value.ID == "" {
				e = tx.Table("catalog_option_values").Create(map[string]any{"id": "optval_" + uuid.NewString(), "option_id": id, "value": value.Value, "rank": value.Rank, "created_at": now, "updated_at": now}).Error
			} else {
				e = tx.Table("catalog_option_values").Where("id=? AND option_id=? AND deleted_at IS NULL", value.ID, id).Updates(map[string]any{"rank": value.Rank, "updated_at": now}).Error
			}
			if e != nil {
				return e
			}
		}
		if len(plan.Remove) != 0 {
			if e = tx.Table("catalog_option_values").Where("id IN ? AND option_id=? AND deleted_at IS NULL", plan.Remove, id).Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; e != nil {
				return e
			}
		}
		after, e := loadOptionDefinition(tx, id, enabled)
		if e != nil {
			return e
		}
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		if e = tx.Table("catalog_option_definition_edits").Create(map[string]any{"operation_id": in.OperationID, "option_id": id, "actor_id": actor.ID, "action": action, "request_sha256": requestSHA, "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": after.Revision, "created_at": now}).Error; e != nil {
			return e
		}
		out = OptionDefinitionResult{id, after.Revision, false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}

// An empty valueID removes the whole definition. A value-specific deletion
// permits removing its last value, as the original standalone value action
// does; the create/edit form's requirement for at least one value is separate.
func (h Handler) RemoveOptionDefinition(ctx context.Context, id, valueID string, actor identity.User, in OptionDefinitionRemoveInput) (OptionDefinitionResult, error) {
	var out OptionDefinitionResult
	if !validProductID(id) || valueID != "" && !validProductID(valueID) || !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) {
		return out, errEditInput
	}
	if h.DB == nil {
		return out, ErrSchema
	}
	action := "delete"
	if valueID != "" {
		action = "delete-value"
	}
	requestSHA := imageDigest(struct {
		ID, ValueID string
		Input       OptionDefinitionRemoveInput
	}{id, valueID, in})
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
			if receipt.OptionID != id || receipt.ActorID != actor.ID || receipt.Action != action || receipt.RequestSHA256 != requestSHA {
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
		if valueID == "" && before.ActiveProducts != 0 {
			return ErrOptionDefinitionAssociated
		}
		if valueID != "" {
			found := false
			for _, value := range before.Values {
				if value.ID == valueID {
					found = true
					if value.Memberships != 0 {
						return ErrOptionDefinitionAssociated
					}
				}
			}
			if !found {
				return gorm.ErrRecordNotFound
			}
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		deleted := map[string]any{"deleted_at": now, "updated_at": now}
		if valueID == "" {
			// Mirror source soft-delete cascades, including links to products
			// already soft deleted. Durable source identities remain in place.
			if e = tx.Table("catalog_product_option_values").Where("product_product_option_id IN (SELECT id FROM catalog_product_options WHERE product_option_id=?) AND deleted_at IS NULL", id).Updates(deleted).Error; e != nil {
				return e
			}
			if e = tx.Table("catalog_product_options").Where("product_option_id=? AND deleted_at IS NULL", id).Updates(deleted).Error; e != nil {
				return e
			}
			if e = tx.Table("catalog_option_values").Where("option_id=? AND deleted_at IS NULL", id).Updates(deleted).Error; e != nil {
				return e
			}
			if e = tx.Table("catalog_options").Where("id=? AND deleted_at IS NULL", id).Updates(deleted).Error; e != nil {
				return e
			}
		} else {
			if e = tx.Table("catalog_option_values").Where("id=? AND option_id=? AND deleted_at IS NULL", valueID, id).Updates(deleted).Error; e != nil {
				return e
			}
		}
		after := OptionDefinitionContext{ID: id, Title: before.Title, Enabled: enabled, Deleted: true, Values: []definitionValue{}}
		if valueID != "" {
			after, e = loadOptionDefinition(tx, id, enabled)
			if e != nil {
				return e
			}
		} else {
			after.Revision = imageDigest(after)
		}
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		if e = tx.Table("catalog_option_definition_edits").Create(map[string]any{"operation_id": in.OperationID, "option_id": id, "actor_id": actor.ID, "action": action, "request_sha256": requestSHA, "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": after.Revision, "created_at": now}).Error; e != nil {
			return e
		}
		out = OptionDefinitionResult{id, after.Revision, false}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	return out, e
}

func applyDefinitionPredicates(base *gorm.DB, query optionDefinitionQuery) *gorm.DB {
	if query.Search != "" {
		base = base.Where("title LIKE ? ESCAPE '!'", literalLike(query.Search))
	}
	if query.Title != "" {
		base = base.Where("title = ?", query.Title)
	}
	if query.ID != "" {
		base = base.Where("id = ?", query.ID)
	}
	if len(query.IDs) > 0 {
		base = base.Where("id IN ?", query.IDs)
	}
	if len(query.Titles) > 0 {
		base = base.Where("title IN ?", query.Titles)
	}
	if query.Exclusive != nil {
		base = base.Where("is_exclusive = ?", *query.Exclusive)
	}
	operators := map[string]string{"gt": ">", "gte": ">=", "lt": "<", "lte": "<=", "eq": "=", "ne": "<>"}
	for _, date := range query.Dates {
		if (date.Column != "created_at" && date.Column != "updated_at" && date.Column != "deleted_at") || operators[date.Operator] == "" {
			base = base.Where("1=0")
			continue
		}
		base = base.Where(date.Column+" "+operators[date.Operator]+" ?", date.Value)
	}
	for _, child := range query.And {
		base = base.Where(applyDefinitionPredicates(base.Session(&gorm.Session{NewDB: true}), child))
	}
	if len(query.Or) > 0 {
		alternatives := base.Session(&gorm.Session{NewDB: true})
		for i, child := range query.Or {
			predicate := applyDefinitionPredicates(base.Session(&gorm.Session{NewDB: true}), child)
			if i == 0 {
				alternatives = alternatives.Where(predicate)
			} else {
				alternatives = alternatives.Or(predicate)
			}
		}
		base = base.Where(alternatives)
	}
	return base
}
