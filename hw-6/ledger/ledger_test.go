package ledger

import (
	"errors"
	"strings"
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
			if tc.wantErr && err == nil {
				t.Errorf("ожидалась ошибка, получили nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ожидали nil, получили ошибку: %v", err)
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
		{"valid_with_period", Budget{Category: "еда", Limit: 1, Period: "2025-09"}, false},
		{"zero_limit", Budget{Category: "еда", Limit: 0}, true},
		{"negative_limit", Budget{Category: "еда", Limit: -100}, true},
		{"empty_category", Budget{Category: "", Limit: 5000}, true},
		{"spaces_category", Budget{Category: "  ", Limit: 5000}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.b.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("ожидалась ошибка, получили nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ожидали nil, получили ошибку: %v", err)
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
			err := CheckValid(tc.v)
			if (err != nil) != tc.wantErr {
				t.Errorf("CheckValid() ошибка = %v, ожидали ошибку = %v", err, tc.wantErr)
			}
		})
	}
}

func TestBudgetFlow(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	if err := SetBudget(Budget{Category: "еда", Limit: 5000}); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}

	t.Run("within_limit", func(t *testing.T) {
		tx, err := AddTransaction(Transaction{Amount: 3000, Category: "еда", Date: "2025-09-10"})
		if err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
		if tx.ID != 1 {
			t.Errorf("ID = %d, ожидали 1", tx.ID)
		}
	})

	t.Run("exactly_limit", func(t *testing.T) {
		if _, err := AddTransaction(Transaction{Amount: 2000, Category: "еда", Date: "2025-09-11"}); err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
	})

	t.Run("exceeded", func(t *testing.T) {
		before := len(ListTransactions())
		_, err := AddTransaction(Transaction{Amount: 1, Category: "еда", Date: "2025-09-12"})
		if !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("ожидали ErrBudgetExceeded, получили %v", err)
		}
		if err.Error() != "budget exceeded" {
			t.Errorf("текст ошибки = %q", err.Error())
		}
		if after := len(ListTransactions()); after != before {
			t.Errorf("транзакций было %d, стало %d", before, after)
		}
	})

	t.Run("other_category_no_budget", func(t *testing.T) {
		if _, err := AddTransaction(Transaction{Amount: 100000, Category: "кино", Date: "2025-09-12"}); err != nil {
			t.Fatalf("ожидали nil, получили %v", err)
		}
	})

	t.Run("raise_limit", func(t *testing.T) {
		if err := SetBudget(Budget{Category: "еда", Limit: 6000}); err != nil {
			t.Fatalf("SetBudget: %v", err)
		}
		if _, err := AddTransaction(Transaction{Amount: 1000, Category: "еда", Date: "2025-09-13"}); err != nil {
			t.Fatalf("ожидали nil после увеличения лимита, получили %v", err)
		}
	})

	t.Run("totals", func(t *testing.T) {
		if got := len(ListTransactions()); got != 4 {
			t.Errorf("транзакций %d, ожидали 4", got)
		}
		if got := CategoryTotal("еда"); got != 6000 {
			t.Errorf("CategoryTotal(еда) = %v, ожидали 6000", got)
		}
	})
}

func TestAddTransactionInvalid(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	_, err := AddTransaction(Transaction{Amount: -1, Category: "еда", Date: "2025-09-10"})
	if err == nil {
		t.Fatal("ожидалась ошибка валидации")
	}
	if errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("ошибка валидации не должна быть ErrBudgetExceeded")
	}
	if n := len(ListTransactions()); n != 0 {
		t.Errorf("транзакций %d, ожидали 0", n)
	}
}

func TestSetBudgetInvalid(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	if err := SetBudget(Budget{Category: "еда", Limit: 0}); err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if n := len(ListBudgets()); n != 0 {
		t.Errorf("бюджетов %d, ожидали 0", n)
	}
}

func TestListTransactionsReturnsCopy(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	if _, err := AddTransaction(Transaction{Amount: 10, Category: "еда", Date: "2025-09-10"}); err != nil {
		t.Fatal(err)
	}
	list := ListTransactions()
	list[0].Amount = 999

	if got := ListTransactions()[0].Amount; got != 10 {
		t.Errorf("хранилище изменилось через копию: %v", got)
	}
}

func TestListBudgetsSorted(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	for _, c := range []string{"транспорт", "еда", "кино"} {
		if err := SetBudget(Budget{Category: c, Limit: 100}); err != nil {
			t.Fatal(err)
		}
	}

	got := ListBudgets()
	want := []string{"еда", "кино", "транспорт"}
	if len(got) != len(want) {
		t.Fatalf("бюджетов %d, ожидали %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Category != want[i] {
			t.Errorf("[%d] = %q, ожидали %q", i, got[i].Category, want[i])
		}
	}
}

func TestLoadBudgets(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   string
		wantCount int
	}{
		{"ok", `[{"category":"еда","limit":5000},{"category":"кино","limit":1000,"period":"2025-09"}]`, "", 2},
		{"ok_with_bom", "\xef\xbb\xbf" + `[{"category":"еда","limit":5000}]`, "", 1},
		{"empty_array", `[]`, "", 0},
		{"bad_json", `[{"category":`, "не удалось разобрать JSON", 0},
		{"wrong_type", `[{"category":"еда","limit":"abc"}]`, "не удалось разобрать JSON", 0},
		{"invalid_limit", `[{"category":"еда","limit":100},{"category":"кино","limit":-1}]`, "бюджет #2", 0},
		{"empty_category", `[{"category":"","limit":100}]`, "бюджет #1", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			Reset()
			t.Cleanup(Reset)

			err := LoadBudgets(strings.NewReader(tc.input))
			if tc.wantErr == "" && err != nil {
				t.Fatalf("ожидали nil, получили %v", err)
			}
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("ожидали ошибку с %q, получили nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("ошибка %q не содержит %q", err.Error(), tc.wantErr)
				}
			}
			if got := len(ListBudgets()); got != tc.wantCount {
				t.Errorf("бюджетов %d, ожидали %d", got, tc.wantCount)
			}
		})
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func TestLoadBudgetsReadError(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	err := LoadBudgets(errReader{})
	if err == nil || !strings.Contains(err.Error(), "не удалось прочитать") {
		t.Errorf("ожидали ошибку чтения, получили %v", err)
	}
}
