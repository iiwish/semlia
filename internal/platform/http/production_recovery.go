package httpapi

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/governance"
)

func productionProgress(version domain.ProductionVersion, targets []domain.ProductionTarget) string {
	if version.ReleaseID != nil {
		return "released"
	}
	states := map[string]int{}
	allApproved := true
	for _, target := range targets {
		if target.ProposalState != nil {
			states[*target.ProposalState]++
			allApproved = allApproved && target.ReviewApproved
		} else if target.Outcome != domain.ProductionOutcomeNoChange {
			states["draft"]++
			allApproved = false
		}
	}
	if len(states) == 0 {
		return "no_change"
	}
	if states["rejected"] > 0 {
		return "rejected"
	}
	if states["draft"] > 0 {
		return "draft"
	}
	if a := version.ActiveValidation; a != nil {
		if a.Status == "failed" {
			return "needs_correction"
		}
		if a.Status == "queued" || a.Status == "running" {
			return "validating"
		}
		if a.Status == "succeeded" && allApproved {
			return "ready_to_publish"
		}
	}
	if states["validating"] > 0 {
		return "validating"
	}
	return "in_review"
}

func productionValidationRecovery(version domain.ProductionVersion) (any, []string) {
	unresolved := []string{}
	attempt := version.ActiveValidation
	if attempt == nil {
		return ValidationStatusResponse{Status: "not_requested"}, unresolved
	}
	result := ValidationAttemptResultResponse{AttemptNo: attempt.AttemptNo, Status: attempt.Status,
		SetDigest: attempt.SetDigest, FreshnessDigest: attempt.FreshnessDigest,
		RequiredChecksDigest: attempt.RequiredChecksDigest, ValidationDigest: attempt.ValidationDigest,
		RunIDs: []string{}, Checks: []ValidationCheckResultResponse{}}
	if attempt.Status != "succeeded" {
		result.ValidationDigest = nil
	}
	if attempt.CompletedAt != nil {
		value := attempt.CompletedAt.UTC().Format(time.RFC3339Nano)
		result.CompletedAt = &value
	}
	codes := map[string]bool{}
	for _, run := range version.ValidationRuns {
		result.RunIDs = append(result.RunIDs, run.ID.String())
		check := ValidationCheckResultResponse{RunID: run.ID.String(), ProposalID: run.ProposalID.String(),
			ValidatorID: run.ValidatorID, ValidatorVersion: run.ValidatorVersion, Status: string(run.Status), Results: []ValidationCheckItemResponse{}}
		for _, item := range version.ValidationResults[run.ID.String()] {
			details := map[string]any{}
			_ = json.Unmarshal(item.Details, &details)
			check.Results = append(check.Results, ValidationCheckItemResponse{Severity: string(item.Severity), Code: item.Code, Message: item.Message, InputDigest: item.InputDigest, Details: details})
			if item.Severity == domain.SeverityBlocker {
				codes[item.Code] = true
			}
		}
		result.Checks = append(result.Checks, check)
	}
	for code := range codes {
		unresolved = append(unresolved, code)
	}
	sort.Strings(unresolved)
	return result, unresolved
}

func writeProductionRecovery(response http.ResponseWriter, value any, traceID string) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return writeProductionError(response, err, traceID)
	}
	if len(raw)+1 > 4*1024*1024 {
		return writeProductionError(response, domain.ErrLimitExceeded, traceID)
	}
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(http.StatusOK)
	_, _ = response.Write(append(raw, '\n'))
	return ""
}
