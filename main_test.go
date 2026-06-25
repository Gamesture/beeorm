package beeorm

import (
	"os"
	"testing"
)

// TestMain forces UTC for the whole test process. BeeORM requires the
// application to run in UTC (apps do os.Setenv("TZ","UTC") in main()), but
// `go test` never runs main(), so without this the test machine's local
// timezone would shift datetimes written/read through MySQL.
func TestMain(m *testing.M) {
	if err := os.Setenv("TZ", "UTC"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}
