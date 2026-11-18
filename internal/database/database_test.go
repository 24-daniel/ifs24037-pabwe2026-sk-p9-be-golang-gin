package database

import (
	"path/filepath"
	"testing"

	"app/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestConnectDB_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		DBConnection: "sqlite",
		DBDatabase:   dbPath,
	}

	db, err := ConnectDB(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, db)

	sqlDB, err := db.DB()
	if assert.NoError(t, err) {
		defer sqlDB.Close()
	}

	err = Migrate(db)
	assert.NoError(t, err)
}

func TestConnectDB_Postgres_InvalidHost(t *testing.T) {
	cfg := &config.Config{
		DBConnection: "pgsql",
		DBHost:       "invalid-host-99999",
		DBPort:       "5432",
		DBUsername:   "postgres",
		DBPassword:   "postgres",
		DBDatabase:   "invalid_db",
		DBSSLMode:    "disable",
	}

	// GORM open with invalid host might return db with connection error when pinged
	db, _ := ConnectDB(cfg)
	if db != nil {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func TestConnectTestDB(t *testing.T) {
	db, err := ConnectTestDB()
	assert.NoError(t, err)
	assert.NotNil(t, db)
}

func TestSeed(t *testing.T) {
	db, err := ConnectTestDB()
	assert.NoError(t, err)
	assert.NotNil(t, db)

	cfg := &config.Config{
		AdminUsername: "admin",
		AdminPassword: "admin",
	}

	// First run: seeds users and posts
	err = Seed(db, cfg)
	assert.NoError(t, err)

	var userCount int64
	db.Model(&User{}).Count(&userCount)
	assert.GreaterOrEqual(t, userCount, int64(2))

	var postCount int64
	db.Model(&Post{}).Count(&postCount)
	assert.GreaterOrEqual(t, postCount, int64(2))

	// Second run: idempotency test (already exists, no error)
	err = Seed(db, cfg)
	assert.NoError(t, err)

	// Test with empty AdminPassword fallback
	cfgEmptyPass := &config.Config{
		AdminUsername: "admin2",
		AdminPassword: "",
	}
	err = Seed(db, cfgEmptyPass)
	assert.NoError(t, err)

	// Test with empty AdminUsername fallback
	cfgEmptyUser := &config.Config{
		AdminUsername: "",
		AdminPassword: "",
	}
	err = Seed(db, cfgEmptyUser)
	assert.NoError(t, err)
}
