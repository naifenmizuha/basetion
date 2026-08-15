package postgres

import (
	"strings"
	"testing"
)

func TestTemporaryDatabaseName(t *testing.T) {
	first, err := temporaryDatabaseName("basetion_test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := temporaryDatabaseName("basetion_test")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !strings.HasPrefix(first, "basetion_test_") || len(first) > 63 {
		t.Fatalf("temporary names first=%q second=%q", first, second)
	}
	for _, invalid := range []string{"", "Bad", "bad-prefix", strings.Repeat("a", 41)} {
		if _, err := temporaryDatabaseName(invalid); err == nil {
			t.Fatalf("invalid prefix %q accepted", invalid)
		}
	}
}
