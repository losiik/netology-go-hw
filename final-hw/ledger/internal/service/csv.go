package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"ledger/internal/domain"
)

const MaxCSVRows = 10000

var csvColumns = map[string]string{
	"date":        "date",
	"дата":        "date",
	"amount":      "amount",
	"сумма":       "amount",
	"category":    "category",
	"категория":   "category",
	"description": "description",
	"описание":    "description",
}

type csvRow struct {
	index int
	line  int
	tx    domain.Transaction
}

func (s *ledgerService) ImportCSV(ctx context.Context, r io.Reader, workers int) (domain.ImportSummary, error) {
	rows, rowErrors, err := parseCSV(r)
	if err != nil {
		return domain.ImportSummary{}, domain.NewValidationError(err)
	}

	txs := make([]domain.Transaction, len(rows))
	for i, row := range rows {
		txs[i] = row.tx
	}

	summary, err := s.ImportTransactions(ctx, txs, workers)

	for i := range summary.Errors {
		row := rows[summary.Errors[i].Index]
		summary.Errors[i].Index = row.index
		summary.Errors[i].Line = row.line
	}
	summary.Errors = append(summary.Errors, rowErrors...)
	sort.Slice(summary.Errors, func(i, j int) bool { return summary.Errors[i].Index < summary.Errors[j].Index })

	summary.Total += len(rowErrors)
	summary.Rejected += len(rowErrors)
	return summary, err
}

func parseCSV(r io.Reader) ([]csvRow, []domain.ImportError, error) {
	br := bufio.NewReader(r)
	if b, err := br.Peek(3); err == nil && bytes.Equal(b, []byte("\xef\xbb\xbf")) {
		br.Discard(3)
	}

	head, err := br.Peek(4096)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, nil, fmt.Errorf("не удалось прочитать CSV: %w", err)
	}
	firstLine, _, _ := strings.Cut(string(head), "\n")

	cr := csv.NewReader(br)
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		cr.Comma = ';'
	}
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil, errors.New("пустой CSV-файл")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("неверный заголовок CSV: %w", err)
	}

	cols := map[string]int{}
	for i, name := range header {
		if col, ok := csvColumns[strings.ToLower(strings.TrimSpace(name))]; ok {
			cols[col] = i
		}
	}
	for _, need := range []string{"date", "amount", "category"} {
		if _, ok := cols[need]; !ok {
			return nil, nil, fmt.Errorf("в заголовке CSV нет колонки %q (нужны date, amount, category, description)", need)
		}
	}

	var rows []csvRow
	var rowErrors []domain.ImportError
	index := -1
	for {
		record, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		index++
		if index >= MaxCSVRows {
			return nil, nil, fmt.Errorf("слишком много строк в CSV, максимум %d", MaxCSVRows)
		}
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			rowErrors = append(rowErrors, domain.ImportError{Index: index, Line: pe.StartLine, Error: "не удалось разобрать строку CSV"})
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("не удалось прочитать CSV: %w", err)
		}
		line, _ := cr.FieldPos(0)
		tx, err := parseCSVRecord(record, cols)
		if err != nil {
			rowErrors = append(rowErrors, domain.ImportError{Index: index, Line: line, Error: err.Error()})
			continue
		}
		rows = append(rows, csvRow{index: index, line: line, tx: tx})
	}
	return rows, rowErrors, nil
}

func field(record []string, cols map[string]int, name string) string {
	i, ok := cols[name]
	if !ok || i >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[i])
}

func parseCSVRecord(record []string, cols map[string]int) (domain.Transaction, error) {
	rawAmount := field(record, cols, "amount")
	amountStr := strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(rawAmount)
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return domain.Transaction{}, fmt.Errorf("неверная сумма %q", rawAmount)
	}

	date := field(record, cols, "date")
	if t, err := time.Parse("02.01.2006", date); err == nil {
		date = t.Format(domain.DateLayout)
	}

	return domain.Transaction{
		Amount:      amount,
		Category:    field(record, cols, "category"),
		Description: field(record, cols, "description"),
		Date:        date,
	}, nil
}

func (s *ledgerService) ExportCSV(ctx context.Context, w io.Writer, from, to string) error {
	if from != "" || to != "" {
		if err := domain.ValidatePeriod(from, to); err != nil {
			return domain.NewValidationError(err)
		}
	}

	list, err := s.expenses.List(ctx, domain.UserIDFromContext(ctx))
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"id", "date", "amount", "category", "description"}); err != nil {
		return err
	}
	for i := len(list) - 1; i >= 0; i-- {
		tx := list[i]
		if from != "" && (tx.Date < from || tx.Date > to) {
			continue
		}
		err := cw.Write([]string{
			strconv.Itoa(tx.ID),
			tx.Date,
			strconv.FormatFloat(tx.Amount, 'f', 2, 64),
			tx.Category,
			tx.Description,
		})
		if err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
