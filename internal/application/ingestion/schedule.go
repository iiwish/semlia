package ingestion

import (
	"strings"
	"time"
	_ "time/tzdata"

	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	cronlib "github.com/robfig/cron/v3"
)

type NominalOccurrence struct {
	WallClockKey string
	ScheduledFor *time.Time
	EligibleAt   time.Time
	DSTGap       bool
}

var fiveFieldCronParser = cronlib.NewParser(cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow)

func NextNominalOccurrence(expression, timezone, afterWallClockKey string, now time.Time) (NominalOccurrence, error) {
	cron, err := parseCron(expression)
	if err != nil {
		return NominalOccurrence{}, domain.ErrInvalid
	}
	location, err := time.LoadLocation(strings.TrimSpace(timezone))
	if err != nil || strings.TrimSpace(timezone) == "" {
		return NominalOccurrence{}, domain.ErrInvalid
	}
	var nominal time.Time
	if afterWallClockKey != "" {
		nominal, err = time.ParseInLocation("2006-01-02T15:04", afterWallClockKey, time.UTC)
		if err != nil {
			return NominalOccurrence{}, domain.ErrInvalid
		}
	} else {
		local := now.In(location)
		nominal = time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), 0, 0, time.UTC)
	}
	nominal = cron.Next(nominal)
	if nominal.IsZero() {
		return NominalOccurrence{}, domain.ErrUnsupported
	}
	key := nominal.Format("2006-01-02T15:04")
	instants, instantsErr := wallClockInstants(location, nominal)
	if instantsErr != nil {
		return NominalOccurrence{}, instantsErr
	}
	if len(instants) == 0 {
		eligible, gapErr := postGapBoundary(location, nominal)
		if gapErr != nil {
			return NominalOccurrence{}, gapErr
		}
		return NominalOccurrence{WallClockKey: key, EligibleAt: eligible, DSTGap: true}, nil
	}
	earlier := instants[0].UTC()
	return NominalOccurrence{WallClockKey: key, ScheduledFor: &earlier, EligibleAt: earlier}, nil
}

func ParseSchedule(expression, timezone string) error {
	if _, err := parseCron(expression); err != nil {
		return err
	}
	if strings.TrimSpace(timezone) == "" || timezone == "Local" || (timezone != "UTC" && !strings.Contains(timezone, "/")) {
		return domain.ErrInvalid
	}
	_, err := time.LoadLocation(timezone)
	if err != nil {
		return domain.ErrInvalid
	}
	return nil
}

func parseCron(value string) (cronlib.Schedule, error) {
	parts := strings.Fields(value)
	if len(parts) != 5 {
		return nil, domain.ErrInvalid
	}
	parsed, err := fiveFieldCronParser.Parse(strings.Join(parts, " "))
	if err != nil {
		return nil, domain.ErrInvalid
	}
	return parsed, nil
}

func wallClockInstants(location *time.Location, nominal time.Time) ([]time.Time, error) {
	want := nominal.Format("2006-01-02T15:04")
	guess := time.Date(nominal.Year(), nominal.Month(), nominal.Day(), nominal.Hour(), nominal.Minute(), 0, 0, location).UTC()
	result := make([]time.Time, 0, 2)
	for candidate := guess.Add(-4 * time.Hour); !candidate.After(guess.Add(4 * time.Hour)); candidate = candidate.Add(time.Minute) {
		if candidate.In(location).Format("2006-01-02T15:04") == want {
			result = append(result, candidate)
		}
	}
	if len(result) > 2 {
		return nil, domain.ErrUnsupported
	}
	return result, nil
}

func postGapBoundary(location *time.Location, nominal time.Time) (time.Time, error) {
	guess := time.Date(nominal.Year(), nominal.Month(), nominal.Day(), nominal.Hour(), nominal.Minute(), 0, 0, location).UTC()
	previousCivil := time.Time{}
	for candidate := guess.Add(-12 * time.Hour); !candidate.After(guess.Add(12 * time.Hour)); candidate = candidate.Add(time.Minute) {
		local := candidate.In(location)
		civil := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), 0, 0, time.UTC)
		if !previousCivil.IsZero() && !previousCivil.After(nominal) && civil.After(nominal) {
			return candidate, nil
		}
		previousCivil = civil
	}
	return time.Time{}, domain.ErrUnsupported
}
