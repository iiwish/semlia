package ingestion_test

import (
	"errors"
	"testing"

	"github.com/iiwish/semlia/internal/domain/ingestion"
)

func TestSanitizeOriginalNameRejectsPathsAndControls(t *testing.T) {
	for _, value := range []string{"", "../secret.csv", "folder/file.csv", "bad\x00.csv", "dir\\file.xlsx", "report\u202ecsv.txt"} {
		if _, err := ingestion.SanitizeOriginalName(value); !errors.Is(err, ingestion.ErrInvalid) {
			t.Fatalf("SanitizeOriginalName(%q) = %v", value, err)
		}
	}
	if value, err := ingestion.SanitizeOriginalName("metrics.csv"); err != nil || value != "metrics.csv" {
		t.Fatalf("valid name = %q / %v", value, err)
	}
}
