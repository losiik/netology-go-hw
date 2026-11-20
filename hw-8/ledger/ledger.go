package ledger

import (
	"context"

	"ledger/internal/app"
	"ledger/internal/domain"
	"ledger/internal/service"
)

type (
	Service         = service.Service
	Config          = app.Config
	Transaction     = domain.Transaction
	Budget          = domain.Budget
	ReportSummary   = domain.ReportSummary
	CategorySummary = domain.CategorySummary
	Validatable     = domain.Validatable
)

var (
	ErrBudgetExceeded = domain.ErrBudgetExceeded
	ErrValidation     = domain.ErrValidation
	ErrInvalidPeriod  = domain.ErrInvalidPeriod
)

func CheckValid(v Validatable) error {
	return domain.CheckValid(v)
}

func ConfigFromEnv() (Config, error) {
	return app.ConfigFromEnv()
}

func New(ctx context.Context) (Service, func() error, error) {
	cfg, err := app.ConfigFromEnv()
	if err != nil {
		return nil, nil, err
	}
	return app.New(ctx, cfg)
}

func NewWithConfig(ctx context.Context, cfg Config) (Service, func() error, error) {
	return app.New(ctx, cfg)
}

func NewInMemory() Service {
	return app.NewInMemory()
}
