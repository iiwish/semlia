package distribution_test

import (
	"errors"
	"testing"
	"time"

	d "github.com/iiwish/semlia/internal/domain/distribution"
)

func TestTimeBucketsAreGroupingAndComparisonSelectors(t *testing.T) {
	for _, intent := range []d.Intent{d.IntentCompare, d.IntentBreakdown} {
		for _, scenario := range []string{"month", "no_granularity", "invalid_granularity", "zero_from", "reversed", "missing_selector", "no_measure"} {
			t.Run(string(intent)+"/"+scenario, func(t *testing.T) {
				q := d.SemanticQueryInput{SchemaVersion: d.QuerySchemaVersion, Intent: intent, Measures: []d.Selector{{Search: "sales"}}, Context: d.ResolutionContext{Mode: d.ResolutionCurrent}, TimeRange: &d.TimeRange{Selector: d.Selector{Search: "paid date"}, From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), Granularity: "month"}}
				switch scenario {
				case "no_granularity":
					q.TimeRange.Granularity = ""
				case "invalid_granularity":
					q.TimeRange.Granularity = "RAW_PRIVATE_VALUE"
				case "zero_from":
					q.TimeRange.From = time.Time{}
				case "reversed":
					q.TimeRange.To = q.TimeRange.From.Add(-time.Hour)
				case "missing_selector":
					q.TimeRange.Selector = d.Selector{}
				case "no_measure":
					q.Measures = nil
				}
				err := q.Validate()
				if scenario == "month" {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					var invalid *d.ValidationError
					if !errors.As(err, &invalid) {
						t.Fatalf("invalid query accepted: %v", err)
					}
				}
			})
		}
	}
}
