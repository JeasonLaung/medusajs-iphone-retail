package orders

import (
	"github.com/eurorefurb/retail/internal/totals"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math/big"
)

// CurrentPendingDifferenceTx reads current native amounts against the exact
// transaction ledger. Cached summary cash totals never authorize a refund.
// Caller owns access, a SQL transaction and the order lock; this does not mutate
// a summary. Locking reads avoid an earlier repeatable-read snapshot hiding
// newly committed cash even after the caller acquires the order lock.
func CurrentPendingDifferenceTx(tx *gorm.DB, id string) (*big.Rat, error) {
	if tx == nil || tx.Statement == nil {
		return nil, ErrSchema
	}
	if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return nil, ErrSchema
	}
	tx = currentAmountReads(tx)
	amounts, e := ReadAmountSnapshot(tx, id)
	if e != nil {
		return nil, e
	}
	balance, ok := new(big.Rat).SetString(amounts.Totals["total"])
	if !ok {
		return nil, ErrSchema
	}
	var rows []struct {
		Amount, RawAmount, CurrencyCode string
		Version                         int
	}
	if tx.Table("order_transactions").Select("CAST(amount AS CHAR) AS amount,raw_amount,currency_code,version").Where("order_id=? AND deleted_at IS NULL", id).Order("id").Limit(10001).Find(&rows).Error != nil || len(rows) > 10000 {
		return nil, ErrSchema
	}
	for _, r := range rows {
		if r.CurrencyCode != amounts.CurrencyCode || r.Version < 1 || r.Version > amounts.Version {
			return nil, ErrSchema
		}
		number, e := totals.StoredNumber(r.Amount, &r.RawAmount)
		if e != nil {
			return nil, ErrSchema
		}
		value, e := totals.Resolve(number)
		if e != nil {
			return nil, ErrSchema
		}
		n, ok := new(big.Rat).SetString(value)
		if !ok {
			return nil, ErrSchema
		}
		balance.Sub(balance, n)
	}
	return balance, nil
}

// Session cloning preserves the read lock without sharing query predicates
// between the independent reads in ReadAmountSnapshot.
func currentAmountReads(tx *gorm.DB) *gorm.DB {
	return tx.Clauses(clause.Locking{Strength: "SHARE"}).Session(&gorm.Session{})
}
