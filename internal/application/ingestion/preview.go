package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	domain "github.com/iiwish/semlia/internal/domain/ingestion"
	"github.com/iiwish/semlia/pkg/identity"
)

func (service *Service) Preview(ctx context.Context, workspace identity.WorkspaceID, setID identity.ArtifactSetID, artifactID identity.ArtifactID, principalRef, traceID string) (domain.Preview, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	set, err := service.GetSet(ctx, workspace, setID, principalRef, traceID)
	if err != nil {
		return domain.Preview{}, err
	}
	var member *domain.ArtifactMember
	for i := range set.Members {
		if set.Members[i].ArtifactID == artifactID {
			member = &set.Members[i]
			break
		}
	}
	if member == nil || set.WorkspaceID != workspace {
		return domain.Preview{}, domain.ErrNotFound
	}
	adapter, ok := service.adapters[member.Kind].(domain.ContentPreviewer)
	if !ok {
		return domain.Preview{}, domain.ErrUnsupported
	}
	artifact, err := service.repository.GetArtifact(ctx, workspace, artifactID)
	if err != nil {
		return domain.Preview{}, err
	}
	if artifact.WorkspaceID != workspace || artifact.ContentDigest != member.ContentDigest || artifact.ByteSize != member.ByteSize || artifact.Kind != member.Kind {
		return domain.Preview{}, domain.ErrNotFound
	}
	if artifact.ContentAvailability != domain.ContentAvailable || member.ContentAvailability != domain.ContentAvailable || (artifact.ExpiresAt != nil && !artifact.ExpiresAt.After(service.clock.Now())) {
		return domain.Preview{}, domain.ErrContentUnavailable
	}
	if member.ByteSize < 1 || member.ByteSize > domain.MaxUploadBytes {
		return domain.Preview{}, domain.ErrLimitExceeded
	}
	select {
	case service.stageSlots <- struct{}{}:
		defer func() { <-service.stageSlots }()
	case <-ctx.Done():
		return domain.Preview{}, domain.ErrStore
	}
	reader, err := service.store.Open(ctx, workspace, member.ContentDigest)
	if err != nil {
		return domain.Preview{}, domain.ErrStore
	}
	content, err := io.ReadAll(io.LimitReader(reader, member.ByteSize+1))
	closeErr := reader.Close()
	if err != nil || closeErr != nil || int64(len(content)) != member.ByteSize {
		return domain.Preview{}, domain.ErrStore
	}
	digest := sha256.Sum256(content)
	if "sha256:"+hex.EncodeToString(digest[:]) != member.ContentDigest {
		return domain.Preview{}, domain.ErrStore
	}
	result, err := adapter.Preview(ctx, member.LogicalPath, content)
	if err != nil {
		return domain.Preview{}, normalizeAdapterError(err)
	}
	// Reauthorize after parsing; revoked access must not receive the buffered content.
	currentSet, err := service.GetSet(ctx, workspace, setID, principalRef, traceID)
	if err != nil {
		return domain.Preview{}, err
	}
	available := false
	for _, current := range currentSet.Members {
		if current.ArtifactID == artifactID && current.ContentDigest == member.ContentDigest && current.ContentAvailability == domain.ContentAvailable {
			available = true
			break
		}
	}
	if !available {
		return domain.Preview{}, domain.ErrContentUnavailable
	}
	result.ArtifactID = artifactID.String()
	result.ContentDigest = member.ContentDigest
	return result, nil
}
