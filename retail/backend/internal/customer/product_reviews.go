package customer

import (
	"context"
	"errors"

	"github.com/eurorefurb/retail/internal/productreviews"
	"gorm.io/gorm"
)

func (s *Store) productReviewTransaction(ctx context.Context, token string, action func(*gorm.DB, string, string) error) error {
	var domain error
	e := s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		var current record
		if e := tx.Select("id,first_name").Where("id=?", account).Take(&current).Error; e != nil {
			return e
		}
		domain = action(tx, account, current.FirstName)
		return domain
	})
	if e != nil && domain != nil {
		for _, known := range []error{productreviews.ErrInput, productreviews.ErrProduct, productreviews.ErrOrder, productreviews.ErrIneligible, productreviews.ErrDuplicate, productreviews.ErrConflict} {
			if errors.Is(domain, known) {
				return known
			}
		}
	}
	return e
}
func (s *Store) SubmitProductReview(ctx context.Context, token string, service *productreviews.Service, in productreviews.Submission) (productreviews.Result, error) {
	var out productreviews.Result
	if service == nil {
		return out, ErrUnavailable
	}
	e := s.productReviewTransaction(ctx, token, func(tx *gorm.DB, account, first string) error {
		var e error
		out, e = service.SubmitTx(ctx, tx, account, first, in)
		return e
	})
	if e != nil {
		return productreviews.Result{}, e
	}
	return out, nil
}
func (s *Store) ProductReviewPurchases(ctx context.Context, token string, service *productreviews.Service, q productreviews.ListQuery) (productreviews.Purchases, error) {
	var out productreviews.Purchases
	if service == nil {
		return out, ErrUnavailable
	}
	e := s.productReviewTransaction(ctx, token, func(tx *gorm.DB, account, _ string) error {
		var e error
		out, e = productreviews.PurchasesTx(tx, account, service.StoreID, q)
		return e
	})
	if e != nil {
		return productreviews.Purchases{}, e
	}
	return out, nil
}
func (s *Store) OrderProductReviews(ctx context.Context, token string, service *productreviews.Service, order string) (productreviews.OrderContext, error) {
	var out productreviews.OrderContext
	if service == nil {
		return out, ErrUnavailable
	}
	e := s.productReviewTransaction(ctx, token, func(tx *gorm.DB, account, _ string) error {
		var e error
		out, e = productreviews.OrderContextTx(tx, account, service.StoreID, order)
		return e
	})
	if e != nil {
		return productreviews.OrderContext{}, e
	}
	return out, nil
}
