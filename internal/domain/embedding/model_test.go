package embedding

import (
	"math"
	"testing"
)

func TestValidateVectorsRejectsIncompleteMixedAndNonFiniteBatches(t *testing.T) {
	for _, vectors := range [][][]float32{nil, {{1, 2}}, {{1, 2}, {3}}, {{1, 2}, {float32(math.NaN()), 3}}, {{0, 0}, {1, 2}}} {
		if ValidateVectors(vectors, 2, 2) == nil {
			t.Fatalf("accepted invalid batch %v", vectors)
		}
	}
	if err := ValidateVectors([][]float32{{1, 2}, {3, 4}}, 2, 2); err != nil {
		t.Fatal(err)
	}
}
