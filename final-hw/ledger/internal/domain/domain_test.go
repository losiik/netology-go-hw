package domain

import (
	"errors"
	"testing"
)

func TestTransactionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tx      Transaction
		wantErr bool
	}{
		{"valid", Transaction{Amount: 100, Category: "еда", Date: "2025-09-10"}, false},
		{"valid_fraction", Transaction{Amount: 0.01, Category: "еда", Date: "2025-09-10"}, false},
		{"zero_amount", Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"}, true},
		{"negative_amount", Transaction{Amount: -50, Category: "еда", Date: "2025-09-10"}, true},
		{"empty_category", Transaction{Amount: 100, Category: "", Date: "2025-09-10"}, true},
		{"spaces_category", Transaction{Amount: 100, Category: "   ", Date: "2025-09-10"}, true},
		{"empty_date", Transaction{Amount: 100, Category: "еда", Date: ""}, true},
		{"wrong_date_format", Transaction{Amount: 100, Category: "еда", Date: "10.09.2025"}, true},
		{"invalid_date", Transaction{Amount: 100, Category: "еда", Date: "2025-13-40"}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.tx.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() ошибка = %v, ожидали ошибку = %v", err, tc.wantErr)
			}
		})
	}
}

func TestBudgetValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		b       Budget
		wantErr bool
	}{
		{"valid", Budget{Category: "еда", Limit: 5000}, false},
		{"valid_small", Budget{Category: "еда", Limit: 0.01}, false},
		{"zero_limit", Budget{Category: "еда", Limit: 0}, true},
		{"negative_limit", Budget{Category: "еда", Limit: -100}, true},
		{"empty_category", Budget{Category: "", Limit: 5000}, true},
		{"spaces_category", Budget{Category: "  ", Limit: 5000}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.b.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() ошибка = %v, ожидали ошибку = %v", err, tc.wantErr)
			}
		})
	}
}

func TestCheckValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       Validatable
		wantErr bool
	}{
		{"transaction_ok", Transaction{Amount: 1, Category: "еда", Date: "2025-09-10"}, false},
		{"transaction_bad", Transaction{Amount: 0, Category: "еда", Date: "2025-09-10"}, true},
		{"budget_ok", Budget{Category: "еда", Limit: 1}, false},
		{"budget_bad", Budget{Category: "еда", Limit: 0}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := CheckValid(tc.v); (err != nil) != tc.wantErr {
				t.Errorf("CheckValid() ошибка = %v, ожидали ошибку = %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidatePeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		from, to string
		wantErr  bool
	}{
		{"ok", "2025-09-01", "2025-09-30", false},
		{"same_day", "2025-09-10", "2025-09-10", false},
		{"empty", "", "", true},
		{"bad_from", "01.09.2025", "2025-09-30", true},
		{"bad_to", "2025-09-01", "завтра", true},
		{"from_after_to", "2025-10-01", "2025-09-01", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidatePeriod(tc.from, tc.to)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidatePeriod() ошибка = %v, ожидали ошибку = %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidPeriod) {
				t.Errorf("ошибка должна оборачивать ErrInvalidPeriod: %v", err)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	t.Parallel()

	inner := errors.New("сумма должна быть больше 0")
	err := NewValidationError(inner)

	if err.Error() != inner.Error() {
		t.Errorf("Error() = %q, ожидали %q", err.Error(), inner.Error())
	}
	if !errors.Is(err, ErrValidation) {
		t.Error("errors.Is(err, ErrValidation) = false")
	}
	if !errors.Is(err, inner) {
		t.Error("errors.Is(err, inner) = false")
	}
	if errors.Is(err, ErrBudgetExceeded) {
		t.Error("ошибка валидации не должна быть ErrBudgetExceeded")
	}
}
