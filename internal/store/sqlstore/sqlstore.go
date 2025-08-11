package sqlstore

import (
	"database/sql"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type DBRole string

const (
	DBRolePrimary DBRole = "primary"
	DBRoleMessage DBRole = "message"
)

type Stores struct {
	Primary *sql.DB // MySQL for structured data
	Message *sql.DB // TiDB (or MySQL) for chat data
}

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
