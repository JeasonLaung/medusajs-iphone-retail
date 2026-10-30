package customer

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Notice struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	ResourceID string     `json:"resource_id"`
	Reference  string     `json:"reference"`
	Action     string     `json:"action"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
}
type NoticePage struct {
	Notifications []Notice `json:"notifications"`
	Count         int64    `json:"count"`
	Unread        int64    `json:"unread"`
	Offset        int      `json:"offset"`
	Limit         int      `json:"limit"`
}

func validNoticeID(id string) bool {
	u, e := uuid.Parse(strings.TrimPrefix(id, "notice_"))
	return e == nil && u.Version() == 4 && id == "notice_"+u.String()
}
func (s *Store) Notices(ctx context.Context, token string, offset int) (NoticePage, error) {
	out := NoticePage{Notifications: []Notice{}, Offset: offset, Limit: 20}
	if offset < 0 || offset > 100000 {
		return out, ErrInvalid
	}
	err := s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		query := func() *gorm.DB { return tx.Table("customer_account_notices n").Where("n.customer_id=?", account) }
		if query().Count(&out.Count).Error != nil || query().Where("n.read_at IS NULL").Count(&out.Unread).Error != nil {
			return ErrUnavailable
		}
		return query().Select("n.id,n.kind,n.resource_id,n.reference,n.action,n.read_at,n.created_at").Order("n.created_at DESC,n.id DESC").Limit(20).Offset(offset).Scan(&out.Notifications).Error
	})
	return out, err
}
func (s *Store) ReadNotices(ctx context.Context, token, id string) error {
	if id != "" && !validNoticeID(id) {
		return ErrInvalid
	}
	return s.addressTransaction(ctx, token, func(tx *gorm.DB, account string) error {
		q := tx.Table("customer_account_notices").Where("customer_id=? AND read_at IS NULL", account)
		if id != "" {
			q = q.Where("id=?", id)
		}
		return q.Update("read_at", time.Now().UTC()).Error
	})
}
