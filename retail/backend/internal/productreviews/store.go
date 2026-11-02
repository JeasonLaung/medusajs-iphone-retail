package productreviews

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/eurorefurb/retail/internal/catalog"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Service struct {
	DB      *gorm.DB
	StoreID string
}
type reviewRecord struct {
	PublicReview `gorm:"embedded"`
	ProductID    string
	CustomerID   string
	AccountID    *string
	OrderID      *string
	Status       string
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

func (reviewRecord) TableName() string { return "product_reviews" }

type command struct {
	OperationID, Command, ActorID, ReviewID, RequestSHA256, ResultStatus string
	PreviousStatus                                                       *string
	CreatedAt                                                            time.Time
}

func (command) TableName() string { return "product_review_commands" }
func runtime(tx *gorm.DB) (bool, error) {
	if tx == nil || tx.Statement == nil {
		return false, ErrUnavailable
	}
	if _, transactional := tx.Statement.ConnPool.(gorm.TxCommitter); !transactional {
		return false, ErrUnavailable
	}
	var gate struct {
		Version                     int
		ReadsEnabled, WritesEnabled bool
	}
	if tx.Table("product_review_runtime").Where("id=1").Clauses(clause.Locking{Strength: "SHARE"}).Take(&gate).Error != nil || gate.Version != 1 || !gate.ReadsEnabled {
		return false, ErrUnavailable
	}
	return gate.WritesEnabled, nil
}
func published(ctx context.Context, tx *gorm.DB, store, p string) error {
	e := catalog.RequirePublicProduct(ctx, tx, store, p)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ErrProduct
	}
	return e
}
func (s Service) PublicPage(ctx context.Context, q ListQuery) (Page, error) {
	out := Page{Reviews: []PublicReview{}, Offset: q.Offset, Limit: q.Limit, Distribution: []Distribution{}}
	if !idOK(q.ProductID) || q.Offset < 0 || q.Offset > 100000 || q.Limit < 1 || q.Limit > 20 {
		return out, ErrInput
	}
	if s.DB == nil {
		return out, ErrUnavailable
	}
	e := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, e := runtime(tx); e != nil {
			return e
		}
		if e := published(ctx, tx, s.StoreID, q.ProductID); e != nil {
			return e
		}
		base := func() *gorm.DB {
			return tx.Table("product_reviews").Where("product_id=? AND status='approved' AND deleted_at IS NULL", q.ProductID)
		}
		var groups []Distribution
		if e := base().Select("rating,COUNT(*) AS count").Group("rating").Scan(&groups).Error; e != nil {
			return e
		}
		var counts [6]int64
		var sum int64
		for _, g := range groups {
			if g.Rating < 1 || g.Rating > 5 || g.Count < 0 || g.Count > 1000000000 {
				return ErrUnavailable
			}
			counts[g.Rating] = g.Count
			out.Count += g.Count
			sum += int64(g.Rating) * g.Count
		}
		for i := 1; i <= 5; i++ {
			out.Distribution = append(out.Distribution, Distribution{i, counts[i]})
		}
		if out.Count > 0 {
			out.Average = float64(sum) / float64(out.Count)
		}
		return base().Select("id,author,rating,title,content,created_at,verified_purchase").Order("created_at DESC,id DESC").Offset(q.Offset).Limit(q.Limit).Scan(&out.Reviews).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return Page{}, e
	}
	return out, nil
}

// SubmitTx participates in the customer account transaction. The caller must
// hold the verified current account lock, recheck its Redis session, and supply
// only that account's first name. This function never commits independently.
func (s Service) SubmitTx(ctx context.Context, tx *gorm.DB, account, firstName string, input Submission) (Result, error) {
	var out Result
	in, e := normalize(input)
	if e != nil || !idOK(account) || !idOK(s.StoreID) {
		return out, ErrInput
	}
	enabled, e := runtime(tx)
	if e != nil {
		return out, e
	}
	sha := digest(in)
	var receipt command
	e = tx.Where("operation_id=?", in.OperationID).Clauses(clause.Locking{Strength: "SHARE"}).Take(&receipt).Error
	if e == nil {
		if receipt.Command != "submit" || receipt.ActorID != account || receipt.RequestSHA256 != sha {
			return out, ErrConflict
		}
		return Result{receipt.ReviewID, receipt.ResultStatus, true}, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	if !enabled {
		return out, ErrUnavailable
	}
	if e := published(ctx, tx, s.StoreID, in.ProductID); e != nil {
		return out, e
	}
	if in.OrderID != nil {
		order, e := ownedOrder(tx, account, s.StoreID, *in.OrderID)
		if e != nil {
			return out, e
		}
		eligible, products, e := orderFacts(tx, order)
		if e != nil {
			return out, e
		}
		if !eligible || !containsProduct(products, in.ProductID) {
			return out, ErrIneligible
		}
	}
	var count int64
	if e := tx.Table("product_reviews").Where("account_id=? AND product_id=? AND deleted_at IS NULL", account, in.ProductID).Count(&count).Error; e != nil {
		return out, e
	}
	if count > 0 {
		return out, ErrDuplicate
	}
	stamp := time.Now().UTC()
	id, e := uuid.NewRandom()
	if e != nil {
		return out, e
	}
	review := reviewRecord{PublicReview: PublicReview{ID: "rev_" + id.String(), Author: PublicAuthor(firstName), Rating: in.Rating, Title: in.Title, Content: in.Content, CreatedAt: stamp, VerifiedPurchase: in.OrderID != nil}, ProductID: in.ProductID, CustomerID: account, AccountID: &account, OrderID: in.OrderID, Status: "pending", UpdatedAt: stamp}
	if e := tx.Create(&review).Error; e != nil {
		return out, e
	}
	receipt = command{OperationID: in.OperationID, Command: "submit", ActorID: account, ReviewID: review.ID, RequestSHA256: sha, ResultStatus: "pending", CreatedAt: stamp}
	if e := tx.Create(&receipt).Error; e != nil {
		return out, e
	}
	return Result{review.ID, "pending", false}, nil
}
