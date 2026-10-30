package customer

import (
	"context"
	"errors"
	"github.com/eurorefurb/retail/internal/buyback"
	"gorm.io/gorm"
)

// CreateBuyback binds only the newly authenticated account. The account's
// enabled/verified/auth generation and Redis session are rechecked under lock.
func (s *Store) CreateBuyback(ctx context.Context, token, storeID string, in buyback.Submission) (buyback.CreatedRequest, error) {
	var out buyback.CreatedRequest
	var business error
	e := s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		out, business = buyback.StageCreate(ctx, tx, storeID, account, in)
		return business
	})
	if e != nil {
		if errors.Is(business, buyback.ErrConflict) || errors.Is(business, buyback.ErrInput) {
			return buyback.CreatedRequest{}, business
		}
		return buyback.CreatedRequest{}, e
	}
	return out, nil
}
