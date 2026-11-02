package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/eurorefurb/retail/internal/authorization"
	"github.com/eurorefurb/retail/internal/identity"
	"github.com/eurorefurb/retail/internal/platform"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func optionDefinitionFailure(c *gin.Context, e error) {
	switch {
	case errors.Is(e, errEditInput):
		platform.Fail(c, 400, "catalog.invalid_option_definition", "Invalid option definition")
	case errors.Is(e, authorization.ErrDenied):
		platform.Fail(c, 403, "authorization.denied", "Current product permission required")
	case errors.Is(e, ErrOptionDefinitionAssociated):
		platform.Fail(c, 409, "catalog.option_associated", "Remove product associations before deleting this option or value")
	case errors.Is(e, ErrOptionDefinitionTitle):
		platform.Fail(c, 409, "catalog.option_title_exists", "An active shared option already has this title")
	case errors.Is(e, ErrEditConflict):
		platform.Fail(c, 409, "catalog.option_conflict", "Option changed; reload before saving")
	case errors.Is(e, gorm.ErrRecordNotFound):
		platform.Fail(c, 404, "catalog.not_found", "Option or value not found")
	default:
		platform.Fail(c, 503, "catalog.unavailable", "Options unavailable; retry the same operation")
	}
}

func decodeDefinitionRemoval(r io.Reader) (OptionDefinitionRemoveInput, error) {
	var in OptionDefinitionRemoveInput
	raw, e := io.ReadAll(io.LimitReader(r, 1025))
	if e != nil {
		return in, e
	}
	if len(raw) > 1024 {
		return in, errEditInput
	}
	if _, e = imageObject(raw, "operation_id", "expected_revision"); e != nil {
		return in, e
	}
	if json.Unmarshal(raw, &in) != nil || !validVariantImages(VariantImageInput{OperationID: in.OperationID, ExpectedRevision: in.ExpectedRevision, ImageIDs: []string{}}) {
		return in, errEditInput
	}
	return in, nil
}

func (h Handler) OptionDefinitionsHTTP(c *gin.Context) {
	h.optionDefinitionsHTTP(c, false)
}

func (h Handler) RemoveOptionDefinitionHTTP(c *gin.Context) {
	h.optionDefinitionsHTTP(c, true)
}

func (h Handler) optionDefinitionsHTTP(c *gin.Context, remove bool) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	id, valueID := c.Param("option_id"), c.Param("value_id")
	if id != "" && !validProductID(id) || valueID != "" && (!remove || !validProductID(valueID)) || remove && id == "" || c.Request.URL.ForceQuery {
		optionDefinitionFailure(c, errEditInput)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	if c.Request.Method == http.MethodGet && !remove {
		if id == "" {
			query, e := parseDefinitionQuery(c.Request.URL.RawQuery)
			if e != nil {
				optionDefinitionFailure(c, e)
				return
			}
			out, e := h.ListOptionDefinitions(ctx, actor, query)
			if e != nil {
				optionDefinitionFailure(c, e)
				return
			}
			c.JSON(200, out)
			return
		}
		if c.Request.URL.RawQuery != "" {
			optionDefinitionFailure(c, errEditInput)
			return
		}
		out, e := h.ReadOptionDefinition(ctx, id, actor)
		if e != nil {
			optionDefinitionFailure(c, e)
			return
		}
		c.JSON(200, out)
		return
	}
	if c.Request.Method != http.MethodPost || c.Request.URL.RawQuery != "" {
		optionDefinitionFailure(c, errEditInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "catalog.content_type", "JSON required")
		return
	}
	var out OptionDefinitionResult
	if remove {
		in, err := decodeDefinitionRemoval(c.Request.Body)
		if err != nil {
			platform.InvalidJSON(c, err, "catalog.invalid_option_definition", "Invalid option removal")
			return
		}
		out, e = h.RemoveOptionDefinition(ctx, id, valueID, actor, in)
	} else {
		in, err := decodeOptionDefinition(c.Request.Body)
		if err != nil {
			platform.InvalidJSON(c, err, "catalog.invalid_option_definition", "Invalid option definition")
			return
		}
		if id == "" {
			out, e = h.CreateOptionDefinition(ctx, actor, in)
		} else {
			out, e = h.EditOptionDefinition(ctx, id, actor, in)
		}
	}
	if e != nil {
		optionDefinitionFailure(c, e)
		return
	}
	c.JSON(200, out)
}

func (h Handler) OptionDefinitionMetadataHTTP(c *gin.Context) {
	value, _ := c.Get("admin_user")
	actor, ok := value.(identity.User)
	if !ok {
		platform.Fail(c, 401, "auth.unauthorized", "Sign in to continue")
		return
	}
	if c.Request.Method != http.MethodPost || c.Request.URL.RawQuery != "" || c.Request.URL.ForceQuery || !validProductID(c.Param("option_id")) {
		optionDefinitionFailure(c, errEditInput)
		return
	}
	media, _, e := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if e != nil || media != "application/json" {
		platform.Fail(c, 415, "catalog.content_type", "JSON required")
		return
	}
	in, e := decodeDefinitionMetadata(c.Request.Body)
	if e != nil {
		platform.InvalidJSON(c, e, "catalog.invalid_option_definition", "Invalid metadata")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	out, e := h.EditOptionDefinitionMetadata(ctx, c.Param("option_id"), actor, in)
	if e != nil {
		optionDefinitionFailure(c, e)
		return
	}
	c.JSON(200, out)
}
