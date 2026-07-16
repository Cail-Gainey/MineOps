package repository

import (
	"context"

	"github.com/Cail-Gainey/MineOps/internal/model"
)

// InstallationRepository persists task aggregates and ordered checkpointed steps.
type InstallationRepository interface {
	Create(context.Context, *model.InstallationTask, []model.InstallationStep) error
	UpdateTask(context.Context, *model.InstallationTask) error
	UpdateStep(context.Context, *model.InstallationStep) error
	Get(context.Context, model.ID) (*model.InstallationTask, []model.InstallationStep, error)
	ListByServer(context.Context, model.ID, int, int) ([]model.InstallationTask, error)
	ListRecoverable(context.Context) ([]model.InstallationTask, error)
}
