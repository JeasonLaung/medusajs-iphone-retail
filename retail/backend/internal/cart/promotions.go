package cart

import (
	"context"
	"time"

	"github.com/eurorefurb/retail/internal/promotion"
	"github.com/eurorefurb/retail/internal/sessionstore"
	"gorm.io/gorm"
)

// PromotionRefresh is a phase of a caller-owned cart command. Requested codes
// and action are intentions only; rules, amounts, automatic promotions and
// attribute usage facts are always reloaded from trusted storage. At is the
// command's server timestamp, never an untrusted browser value.
type PromotionRefresh struct {
	Codes  []string
	Action string
	At     time.Time
}

// RefreshPromotions runs code selection, current cart context, active/manual
// and automatic configuration, attribute preview, computation and replacement
// in the same transaction. It does not reserve/register campaign usage: that is
// checkout, not cart refresh. It also does not refresh prices/taxes/shipping,
// call providers, commit, update version or write the enclosing durable receipt.
// Caller MUST finish payment/totals/version/idempotency and roll back on error.
// No standalone HTTP command or enabled capability is provided here.
func RefreshPromotions(ctx context.Context, tx *gorm.DB, store string, ref sessionstore.CartReference, version string, in PromotionRefresh) (PromotionAdjustmentResult, error) {
	failed := PromotionAdjustmentResult{}
	if in.At.IsZero() {
		return failed, ErrInput
	}
	codes, err := ReadPromotionCodes(ctx, tx, store, ref, version, in.Codes, in.Action)
	if err != nil {
		return failed, err
	}
	facts, err := ReadPromotionContext(ctx, tx, store, ref, version)
	if err != nil {
		return failed, err
	}
	candidates, err := promotion.ReadActive(tx, promotion.ActiveRequest{Codes: codes, IncludeAutomatic: true, At: in.At})
	if err != nil {
		return failed, err
	}
	usages, err := promotion.ReadAttributeUsages(tx, candidates, facts.JSON)
	if err != nil {
		return failed, err
	}
	computed, err := promotion.ComputeCandidates(promotion.ComputeInput{Candidates: candidates, Context: facts.JSON, Epsilon: facts.CurrencyEpsilon, AttributeUsages: usages})
	if err != nil {
		return failed, err
	}
	return ReplacePromotionAdjustments(ctx, tx, store, ref, version, computed.Actions)
}
