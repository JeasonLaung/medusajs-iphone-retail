package platform

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open only connects. Schema changes belong to the explicit migrate command.
func Open() (*gorm.DB, error) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		return nil, errors.New("MYSQL_DSN is required; use a dedicated migration database")
	}
	// GORM's default error logger interpolates bound parameters. Queries can
	// contain password hashes, session digests and private business values.
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	n, _ := strconv.Atoi(os.Getenv("DB_MAX_OPEN"))
	if n < 1 || n > 32 {
		n = 8
	}
	pool.SetMaxOpenConns(n)
	pool.SetMaxIdleConns(2)
	pool.SetConnMaxLifetime(10 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return db, nil
}
