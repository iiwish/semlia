package httpapi

import (
	"testing"

	governancedomain "github.com/iiwish/semlia/internal/domain/governance"
)

func TestUnavailableReleaseDetailClearsBindingProtectedObjectPins(t *testing.T) {
	release := governancedomain.Release{Objects: []governancedomain.ObjectManifestEntry{{
		ObjectType: governancedomain.TargetJoinContract, ObjectID: "jnc_01arz3ndektsv4rrffq69g5fav", Version: 2,
	}}}
	detail := unavailableReleaseDetail(release)
	if len(detail.Release.Objects) != 0 || detail.ObjectAvailability != "failed" ||
		len(detail.PriorPinDiff) != 0 || len(detail.CurrentRegistryDiff) != 0 || detail.ConsumerImpact != nil {
		t.Fatalf("unavailable release detail leaked protected data: %+v", detail)
	}
}
