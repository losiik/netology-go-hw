package domain

import "errors"

var (
	ErrBudgetExceeded = errors.New("budget exceeded")
	ErrNotFound       = errors.New("not found")
	ErrInvalidPeriod  = errors.New("неверный период отчёта")
	ErrValidation     = errors.New("validation error")
)

type ValidationError struct {
	Err error
}

func NewValidationError(err error) error {
	return &ValidationError{Err: err}
}

func (e *ValidationError) Error() string {
	return e.Err.Error()
}

func (e *ValidationError) Unwrap() []error {
	return []error{ErrValidation, e.Err}
}
