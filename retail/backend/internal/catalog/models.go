package catalog

import (
	"encoding/json"
	"time"
)

type Product struct {
	ID           string          `json:"id" gorm:"primaryKey"`
	Handle       string          `json:"handle"`
	Title        string          `json:"title"`
	Status       string          `json:"status"`
	Thumbnail    *string         `json:"thumbnail"`
	CollectionID *string         `json:"collection_id"`
	Payload      json.RawMessage `json:"-" gorm:"type:json"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	DeletedAt    *time.Time      `json:"deleted_at"`
}

func (Product) TableName() string { return "catalog_products" }

type Variant struct {
	ID        string `gorm:"primaryKey"`
	ProductID string
	SKU       *string
	Title     string
	Payload   json.RawMessage `gorm:"type:json"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

func (Variant) TableName() string { return "catalog_variants" }

type Price struct {
	ID           string `gorm:"primaryKey"`
	VariantID    string
	CurrencyCode string
	Amount       string
	RegionID     *string
	PriceListID  *string
	MinQuantity  *string
	MaxQuantity  *string
	Payload      json.RawMessage `gorm:"type:json"`
	DeletedAt    *time.Time
}

func (Price) TableName() string { return "catalog_prices" }

type Category struct {
	ID               string `gorm:"primaryKey"`
	Handle           string
	Name             string
	ParentCategoryID *string
	IsActive         bool
	IsInternal       bool
	Payload          json.RawMessage `gorm:"type:json"`
	DeletedAt        *time.Time
}

func (Category) TableName() string { return "catalog_categories" }

type CategoryProduct struct {
	ProductID  string
	CategoryID string
}

func (CategoryProduct) TableName() string { return "catalog_category_products" }

type Collection struct {
	ID        string `gorm:"primaryKey"`
	Handle    string
	Title     string
	Payload   json.RawMessage `gorm:"type:json"`
	DeletedAt *time.Time
}

func (Collection) TableName() string { return "catalog_collections" }

type Region struct {
	ID           string `gorm:"primaryKey"`
	Name         string
	CurrencyCode string
	Payload      json.RawMessage `gorm:"type:json"`
	DeletedAt    *time.Time
}

func (Region) TableName() string { return "commerce_regions" }

type Object map[string]any

func Decode(raw json.RawMessage) Object {
	var out Object
	_ = json.Unmarshal(raw, &out)
	if out == nil {
		out = Object{}
	}
	return out
}
