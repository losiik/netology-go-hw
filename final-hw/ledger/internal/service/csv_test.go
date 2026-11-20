package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"ledger/internal/domain"
)

func TestImportCSV(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	if err := svc.SetBudget(ctx, domain.Budget{Category: "кино", Limit: 100}); err != nil {
		t.Fatal(err)
	}

	data := "\xef\xbb\xbfdate,amount,category,description\n" +
		"2025-09-10,450,еда,обед\n" +
		"10.09.2025,\"1 234,56\",еда,ноутбук\n" +
		"2025-09-11,abc,еда,\n" +
		"2025-09-12,-5,еда,\n" +
		"2025-09-13,500,кино,\n" +
		"\n" +
		"2025-09-14,20,такси,\"поездка, домой\"\n"

	got, err := svc.ImportCSV(ctx, strings.NewReader(data), 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 6 || got.Accepted != 3 || got.Rejected != 3 {
		t.Errorf("сводка %+v", got)
	}

	wantErrors := []struct {
		index, line int
		msg         string
	}{
		{2, 4, "неверная сумма"},
		{3, 5, "сумма должна быть больше 0"},
		{4, 6, "budget exceeded"},
	}
	if len(got.Errors) != len(wantErrors) {
		t.Fatalf("ошибки %+v", got.Errors)
	}
	for i, w := range wantErrors {
		e := got.Errors[i]
		if e.Index != w.index || e.Line != w.line || !strings.Contains(e.Error, w.msg) {
			t.Errorf("errors[%d] = %+v, ожидали index=%d line=%d %q", i, e, w.index, w.line, w.msg)
		}
	}

	list, _ := svc.ListTransactions(ctx)
	if len(list) != 3 {
		t.Fatalf("сохранено %d транзакций", len(list))
	}
	found := false
	for _, tx := range list {
		if tx.Amount == 1234.56 && tx.Date == "2025-09-10" {
			found = true
		}
		if tx.Category == "такси" && tx.Description != "поездка, домой" {
			t.Errorf("описание в кавычках разобрано неверно: %q", tx.Description)
		}
	}
	if !found {
		t.Error("не разобрана строка с датой ДД.ММ.ГГГГ и суммой с запятой")
	}
}

func TestImportCSVFormats(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		accepted int
		wantErr  bool
	}{
		{"semicolon_russian_header", "Дата;Сумма;Категория;Описание\n05.09.2025;100,50;еда;кофе\n06.09.2025;200;такси;\n", 2, false},
		{"columns_in_any_order", "category,amount,date\nеда,10,2025-09-01\n", 1, false},
		{"no_description_column", "date,amount,category\n2025-09-01,10,еда\n", 1, false},
		{"header_only", "date,amount,category\n", 0, false},
		{"empty_file", "", 0, true},
		{"no_category_column", "date,amount\n2025-09-01,10\n", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := newService().ImportCSV(context.Background(), strings.NewReader(tc.data), 2)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Errorf("ожидали ошибку валидации, получили %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Accepted != tc.accepted || got.Rejected != 0 {
				t.Errorf("сводка %+v, ожидали accepted=%d", got, tc.accepted)
			}
		})
	}
}

func TestImportCSVBrokenRow(t *testing.T) {
	data := "date,amount,category\n2025-09-01,10,еда\n2025-09-02,\"20,еда\n"
	got, err := newService().ImportCSV(context.Background(), strings.NewReader(data), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Accepted != 1 || got.Rejected != 1 || len(got.Errors) != 1 || got.Errors[0].Line != 3 {
		t.Errorf("сводка %+v", got)
	}
}

func TestImportCSVTooManyRows(t *testing.T) {
	var b strings.Builder
	b.WriteString("date,amount,category\n")
	for i := 0; i <= MaxCSVRows; i++ {
		b.WriteString("2025-09-01,1,еда\n")
	}
	_, err := newService().ImportCSV(context.Background(), strings.NewReader(b.String()), 4)
	if !errors.Is(err, domain.ErrValidation) || !strings.Contains(err.Error(), "слишком много") {
		t.Errorf("ожидали ошибку о количестве строк, получили %v", err)
	}
}

func TestExportCSV(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	for _, tx := range []domain.Transaction{
		{Amount: 10.5, Category: "еда", Description: "кофе, с собой", Date: "2025-09-10"},
		{Amount: 300, Category: "такси", Date: "2025-08-31"},
		{Amount: 20, Category: "еда", Description: "обед", Date: "2025-09-30"},
	} {
		if _, err := svc.AddTransaction(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}

	header := "id,date,amount,category,description\n"
	tests := []struct {
		name     string
		from, to string
		want     string
	}{
		{"all", "", "", header +
			"2,2025-08-31,300.00,такси,\n" +
			"1,2025-09-10,10.50,еда,\"кофе, с собой\"\n" +
			"3,2025-09-30,20.00,еда,обед\n"},
		{"period", "2025-09-01", "2025-09-30", header +
			"1,2025-09-10,10.50,еда,\"кофе, с собой\"\n" +
			"3,2025-09-30,20.00,еда,обед\n"},
		{"empty_period", "2020-01-01", "2020-01-31", header},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := svc.ExportCSV(ctx, &buf, tc.from, tc.to); err != nil {
				t.Fatal(err)
			}
			if buf.String() != tc.want {
				t.Errorf("CSV:\n%s\nожидали:\n%s", buf.String(), tc.want)
			}
		})
	}

	t.Run("bad_period", func(t *testing.T) {
		var buf bytes.Buffer
		if err := svc.ExportCSV(ctx, &buf, "2025-09-01", ""); !errors.Is(err, domain.ErrValidation) {
			t.Errorf("ожидали ErrValidation, получили %v", err)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		var buf bytes.Buffer
		if err := svc.ExportCSV(ctx, &buf, "", ""); err != nil {
			t.Fatal(err)
		}
		got, err := newService().ImportCSV(ctx, &buf, 2)
		if err != nil || got.Accepted != 3 {
			t.Fatalf("повторный импорт экспорта: %+v, %v", got, err)
		}
	})
}
