package database

import (
	"testing"
)

func TestDatabase(t *testing.T) {
	db := SetupTestDB(t)
	defer db.Close()
}
