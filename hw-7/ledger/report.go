package ledger

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

const reportCacheTTL = 30 * time.Second

var ErrInvalidPeriod = errors.New("неверный период отчёта")

type CategorySummary struct {
	Category string  `json:"category"`
	Total    float64 `json:"total"`
}

type ReportSummary struct {
	From       string            `json:"from"`
	To         string            `json:"to"`
	Total      float64           `json:"total"`
	Categories []CategorySummary `json:"categories"`
}

func checkPeriod(from, to string) error {
	f, err := time.Parse(dateLayout, from)
	if err != nil {
		return fmt.Errorf("%w: from=%q, нужен формат ГГГГ-ММ-ДД", ErrInvalidPeriod, from)
	}
	t, err := time.Parse(dateLayout, to)
	if err != nil {
		return fmt.Errorf("%w: to=%q, нужен формат ГГГГ-ММ-ДД", ErrInvalidPeriod, to)
	}
	if f.After(t) {
		return fmt.Errorf("%w: from позже to", ErrInvalidPeriod)
	}
	return nil
}

func reportCacheKey(from, to string) string {
	return "report:summary:" + from + ":" + to
}

func GetReportSummary(ctx context.Context, from, to string) (ReportSummary, error) {
	if err := checkPeriod(from, to); err != nil {
		return ReportSummary{}, err
	}
	if err := ready(); err != nil {
		return ReportSummary{}, err
	}

	key := reportCacheKey(from, to)
	var report ReportSummary
	if cacheGet(ctx, key, &report) {
		return report, nil
	}

	rows, err := pg.QueryContext(ctx,
		`SELECT category, SUM(amount) FROM expenses
		 WHERE date BETWEEN $1 AND $2
		 GROUP BY category ORDER BY category`, from, to)
	if err != nil {
		return ReportSummary{}, fmt.Errorf("построение отчёта: %w", err)
	}
	defer rows.Close()

	report = ReportSummary{From: from, To: to, Categories: []CategorySummary{}}
	for rows.Next() {
		var c CategorySummary
		if err := rows.Scan(&c.Category, &c.Total); err != nil {
			return ReportSummary{}, fmt.Errorf("построение отчёта: %w", err)
		}
		report.Total += c.Total
		report.Categories = append(report.Categories, c)
	}
	report.Total = math.Round(report.Total*100) / 100
	if err := rows.Err(); err != nil {
		return ReportSummary{}, fmt.Errorf("построение отчёта: %w", err)
	}

	cacheSet(ctx, key, report, reportCacheTTL)
	return report, nil
}
