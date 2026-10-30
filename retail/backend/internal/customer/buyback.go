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

func (s *Store) Buyback(ctx context.Context, token, storeID, id string) (buyback.RequestDetail, error) {
	var out buyback.RequestDetail
	var business error
	e := s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		out, business = buyback.ReadRequest(ctx, tx, storeID, account, id)
		return business
	})
	if e != nil {
		if errors.Is(business, buyback.ErrMissing) {
			return buyback.RequestDetail{}, business
		}
		return buyback.RequestDetail{}, e
	}
	return out, nil
}

func (s *Store) Buybacks(ctx context.Context, token, storeID string, offset int) (buyback.RequestList, error) {
	if offset < 0 || offset > 100000 {
		return buyback.RequestList{}, ErrInvalid
	}
	var out buyback.RequestList
	e := s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		var e error
		out, e = buyback.ListRequests(ctx, tx, storeID, account, offset)
		return e
	})
	if e != nil {
		return buyback.RequestList{}, e
	}
	return out, nil
}
