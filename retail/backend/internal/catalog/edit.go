package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/importvalue"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EditableProduct struct {
	Title       string    `json:"title"`
	Subtitle    *string   `json:"subtitle"`
	Description *string   `json:"description"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type EditInput struct {
	OperationID      string  `json:"operation_id"`
	ExpectedRevision string  `json:"expected_revision"`
	Title            string  `json:"title"`
	Subtitle         *string `json:"subtitle"`
	Description      *string `json:"description"`
	Status           string  `json:"status"`
}
type EditResult struct {
	Revision string `json:"revision"`
	Replayed bool   `json:"replayed"`
}

var ErrEditConflict = errors.New("catalog edit conflict")
var ErrEditDenied = errors.New("catalog edit authority changed")
var errEditInput = errors.New("invalid product edit")

func editRevision(p EditableProduct) string {
	body, _ := json.Marshal(p)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func decodeEdit(reader io.Reader) (EditInput, error) {
	var in EditInput
	d := json.NewDecoder(reader)
	start, err := d.Token()
	if err != nil {
		return in, err
	}
	if start != json.Delim('{') {
		return in, errEditInput
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return in, err
		}
		key, ok := token.(string)
		if !ok || fields[key] != nil {
			return in, errEditInput
		}
		switch key {
		case "operation_id", "expected_revision", "title", "subtitle", "description", "status":
		default:
			return in, errEditInput
		}
		var raw json.RawMessage
		if err = d.Decode(&raw); err != nil {
			return in, err
		}
		if (key != "subtitle" && key != "description") || !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			if _, err := importvalue.Text(raw, 0); err != nil {
				return in, errEditInput
			}
		}
		fields[key] = raw
	}
	if _, err = d.Token(); err != nil {
		return in, err
	}
	if len(fields) != 6 {
		return in, errEditInput
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return in, errEditInput
	}
	body, _ := json.Marshal(fields)
	if json.Unmarshal(body, &in) != nil {
		return in, errEditInput
	}
	return in, validateEdit(in)
}

func validateEdit(in EditInput) error {
	id, err := uuid.Parse(in.OperationID)
	if err != nil || id.String() != in.OperationID || id.Version() != 4 {
		return errEditInput
	}
	if len(in.ExpectedRevision) != 64 {
		return errEditInput
	}
	decoded, err := hex.DecodeString(in.ExpectedRevision)
	if err != nil || hex.EncodeToString(decoded) != in.ExpectedRevision {
		return errEditInput
	}
	if !utf8.ValidString(in.Title) || strings.TrimSpace(in.Title) == "" || utf8.RuneCountInString(in.Title) > 500 || strings.ContainsRune(in.Title, 0) {
		return errEditInput
	}
	for _, field := range []struct {
		value *string
		max   int
	}{{in.Subtitle, 1000}, {in.Description, 100000}} {
		if field.value != nil && (!utf8.ValidString(*field.value) || utf8.RuneCountInString(*field.value) > field.max || strings.ContainsRune(*field.value, 0)) {
			return errEditInput
		}
	}
	switch in.Status {
	case "draft", "proposed", "published", "rejected":
	default:
		return errEditInput
	}
	return nil
}

// EditBasics serializes edits on the product row. Authority is rechecked under
// an account SHARE lock, held until the update and immutable audit commit.
func (h Handler) EditBasics(ctx context.Context, id string, actor identity.User, in EditInput) (EditResult, error) {
	var result EditResult
	if !validProductID(id) || validateEdit(in) != nil {
		return result, errEditInput
	}
	if h.DB == nil {
		return result, ErrSchema
	}
	request, _ := json.Marshal(in)
	err := h.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := authorization.CheckTx(ctx, tx, actor, authorization.ProductWrite); err != nil {
			if errors.Is(err, authorization.ErrDenied) {
				return ErrEditDenied
			}
			return err
		}

		var capability struct {
			Version int
			Enabled bool
		}
		if tx.Table("catalog_command_capabilities").Clauses(clause.Locking{Strength: "SHARE"}).Where("command=?", "product.edit_basics").Take(&capability).Error != nil || capability.Version != 1 || !capability.Enabled {
			return ErrSchema
		}
		var before EditableProduct
		if err := tx.Table("catalog_products").Select("title,subtitle,description,status,updated_at").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND deleted_at IS NULL", id).Take(&before).Error; err != nil {
			return err
		}
		var previous struct{ ActorID, ProductID, RequestJSON, ResultRevision string }
		// Current read: a retry may have waited for the first commit on the product
		// lock. A repeatable-read snapshot would miss that just-committed audit.
		err := tx.Table("catalog_product_edits").Clauses(clause.Locking{Strength: "SHARE"}).Where("operation_id=?", in.OperationID).Take(&previous).Error
		if err == nil {
			if previous.ActorID != actor.ID || previous.ProductID != id || !bytes.Equal([]byte(previous.RequestJSON), request) {
				return ErrEditConflict
			}
			result = EditResult{Revision: previous.ResultRevision, Replayed: true}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if editRevision(before) != in.ExpectedRevision {
			return ErrEditConflict
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		if !now.After(before.UpdatedAt) {
			now = before.UpdatedAt.Add(time.Microsecond)
		}
		after := EditableProduct{Title: in.Title, Subtitle: in.Subtitle, Description: in.Description, Status: in.Status, UpdatedAt: now}
		result.Revision = editRevision(after)
		oldJSON, _ := json.Marshal(before)
		newJSON, _ := json.Marshal(after)
		if err := tx.Table("catalog_products").Where("id=?", id).Updates(map[string]any{"title": after.Title, "subtitle": after.Subtitle, "description": after.Description, "status": after.Status, "updated_at": after.UpdatedAt}).Error; err != nil {
			return err
		}
		return tx.Table("catalog_product_edits").Create(map[string]any{"operation_id": in.OperationID, "actor_id": actor.ID, "product_id": id, "request_json": string(request), "before_json": string(oldJSON), "after_json": string(newJSON), "result_revision": result.Revision, "created_at": now}).Error
	})
	return result, err
}

func (h Handler) UpdateProduct(c *gin.Context) {
	if !validProductID(c.Param("id")) || c.Request.URL.RawQuery != "" {
		platform.Fail(c, 400, "catalog.invalid_edit", "Invalid product edit")
		return
	}
	media, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || media != "application/json" {
		platform.Fail(c, 400, "catalog.invalid_edit", "JSON product edit required")
		return
	}
	in, err := decodeEdit(c.Request.Body)
	if err != nil {
		platform.InvalidJSON(c, err, "catalog.invalid_edit", "Invalid product edit")
		return
	}
	value, ok := c.Get("admin_user")
	actor, typed := value.(identity.User)
	if !ok || !typed {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	result, err := h.EditBasics(c.Request.Context(), c.Param("id"), actor, in)
	switch {
	case errors.Is(err, ErrEditDenied):
		platform.Fail(c, 401, "auth.unauthorized", "Account changed; sign in again")
	case errors.Is(err, ErrEditConflict):
		platform.Fail(c, 409, "catalog.edit_conflict", "Product or operation changed; reload before editing")
	case errors.Is(err, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "catalog.not_found", "Product not found")
	case err != nil:
		platform.Fail(c, 503, "catalog.unavailable", "Product edit unavailable; retry the same operation")
	default:
		c.JSON(200, result)
	}
}
