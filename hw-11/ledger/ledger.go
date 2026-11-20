package ledger

import (
	"context"

	"ledger/internal/app"
	"ledger/internal/domain"
	"ledger/internal/service"
)

type (
	Service       = service.Service
	Config        = app.Config
	Transaction   = domain.Transaction
	Budget        = domain.Budget
	ReportSummary = domain.ReportSummary
	ImportSummary = domain.ImportSummary
	ImportError   = domain.ImportError
	Validatable   = domain.Validatable
)

const (
	DefaultImportWorkers = service.DefaultImportWorkers
	MaxImportWorkers     = service.MaxImportWorkers
)

var (
	ErrBudgetExceeded = domain.ErrBudgetExceeded
	ErrValidation     = domain.ErrValidation
	ErrInvalidPeriod  = domain.ErrInvalidPeriod
)

func CheckValid(v Validatable) error {
	return domain.CheckValid(v)
}

func ValidatePeriod(from, to string) error {
	return domain.ValidatePeriod(from, to)
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
