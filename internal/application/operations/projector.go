package operations

import (
	"context"

	domain "github.com/iiwish/semlia/internal/domain/operations"
)

type Projection struct {
	Run   domain.RuntimeRun
	Event *domain.RuntimeRunEvent
}

type ProjectionWriter interface {
	UpsertRuntimeProjection(context.Context, Projection) error
}

type Projector struct{}

func NewProjector() Projector { return Projector{} }

func (Projector) Project(ctx context.Context, writer ProjectionWriter, projection Projection) error {
	if writer == nil {
		return domain.ErrInvalidArgument
	}
	if err := projection.Run.Validate(); err != nil {
		return err
	}
	if projection.Event != nil {
		if projection.Event.WorkspaceID != projection.Run.WorkspaceID || projection.Event.RunID != projection.Run.ID {
			return domain.ErrInvalidArgument
		}
		if err := projection.Event.ValidateForProjection(); err != nil {
			return err
		}
	}
	return writer.UpsertRuntimeProjection(ctx, projection)
}
