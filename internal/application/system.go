package application

import (
	"context"
	"errors"

	"github.com/iiwish/semlia/internal/domain"
)

var ErrDependencyUnavailable = errors.New("required dependency is unavailable")

type ReadinessProbe interface {
	Check(context.Context) error
}

type ReadinessProbeFunc func(context.Context) error

func (fn ReadinessProbeFunc) Check(ctx context.Context) error {
	return fn(ctx)
}

type UnavailableReadinessProbe struct{}

func (UnavailableReadinessProbe) Check(context.Context) error {
	return ErrDependencyUnavailable
}

type SystemService struct {
	probe ReadinessProbe
	info  domain.SystemInfo
}

func NewSystemService(probe ReadinessProbe, info domain.SystemInfo) *SystemService {
	if probe == nil {
		probe = UnavailableReadinessProbe{}
	}
	return &SystemService{probe: probe, info: info}
}

func (service *SystemService) CheckReadiness(ctx context.Context) error {
	return service.probe.Check(ctx)
}

func (service *SystemService) Info() domain.SystemInfo {
	return service.info
}
