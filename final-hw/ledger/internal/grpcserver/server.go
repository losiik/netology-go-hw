package grpcserver

import (
	"bytes"
	"context"
	"errors"
	"log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"ledger/internal/domain"
	"ledger/internal/service"
	"proto/ledgerv1"
)

type Server struct {
	ledgerv1.UnimplementedLedgerServiceServer
	svc service.Service
}

func New(svc service.Service) *Server {
	return &Server{svc: svc}
}

func toStatus(method string, err error) error {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrBudgetExceeded):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, err.Error())
	default:
		log.Printf("%s: %v", method, err)
		return status.Error(codes.Internal, "internal error")
	}
}

func budgetToPB(b domain.Budget) *ledgerv1.Budget {
	return &ledgerv1.Budget{Category: b.Category, Limit: b.Limit}
}

func txToPB(tx domain.Transaction) *ledgerv1.Transaction {
	return &ledgerv1.Transaction{
		Id:          int64(tx.ID),
		Amount:      tx.Amount,
		Category:    tx.Category,
		Description: tx.Description,
		Date:        tx.Date,
	}
}

func txFromPB(tx *ledgerv1.Transaction) domain.Transaction {
	return domain.Transaction{
		Amount:      tx.GetAmount(),
		Category:    tx.GetCategory(),
		Description: tx.GetDescription(),
		Date:        tx.GetDate(),
	}
}

func summaryToPB(s domain.ImportSummary) *ledgerv1.ImportSummary {
	resp := &ledgerv1.ImportSummary{
		Total:    int32(s.Total),
		Accepted: int32(s.Accepted),
		Rejected: int32(s.Rejected),
	}
	for _, e := range s.Errors {
		resp.Errors = append(resp.Errors, &ledgerv1.ImportError{Index: int32(e.Index), Line: int32(e.Line), Error: e.Error})
	}
	return resp
}

func (s *Server) SetBudget(ctx context.Context, req *ledgerv1.SetBudgetRequest) (*ledgerv1.SetBudgetResponse, error) {
	b := domain.Budget{Category: req.GetBudget().GetCategory(), Limit: req.GetBudget().GetLimit()}
	if err := s.svc.SetBudget(ctx, b); err != nil {
		return nil, toStatus("SetBudget", err)
	}
	return &ledgerv1.SetBudgetResponse{Budget: budgetToPB(b)}, nil
}

func (s *Server) ListBudgets(ctx context.Context, _ *ledgerv1.ListBudgetsRequest) (*ledgerv1.ListBudgetsResponse, error) {
	list, err := s.svc.ListBudgets(ctx)
	if err != nil {
		return nil, toStatus("ListBudgets", err)
	}
	resp := &ledgerv1.ListBudgetsResponse{}
	for _, b := range list {
		resp.Budgets = append(resp.Budgets, budgetToPB(b))
	}
	return resp, nil
}

func (s *Server) AddTransaction(ctx context.Context, req *ledgerv1.AddTransactionRequest) (*ledgerv1.AddTransactionResponse, error) {
	created, err := s.svc.AddTransaction(ctx, txFromPB(req.GetTransaction()))
	if err != nil {
		return nil, toStatus("AddTransaction", err)
	}
	return &ledgerv1.AddTransactionResponse{Transaction: txToPB(created)}, nil
}

func (s *Server) ListTransactions(ctx context.Context, _ *ledgerv1.ListTransactionsRequest) (*ledgerv1.ListTransactionsResponse, error) {
	list, err := s.svc.ListTransactions(ctx)
	if err != nil {
		return nil, toStatus("ListTransactions", err)
	}
	resp := &ledgerv1.ListTransactionsResponse{}
	for _, tx := range list {
		resp.Transactions = append(resp.Transactions, txToPB(tx))
	}
	return resp, nil
}

func (s *Server) GetReportSummary(ctx context.Context, req *ledgerv1.GetReportSummaryRequest) (*ledgerv1.GetReportSummaryResponse, error) {
	report, err := s.svc.GetReportSummary(ctx, req.GetFrom(), req.GetTo())
	if err != nil {
		return nil, toStatus("GetReportSummary", err)
	}
	resp := &ledgerv1.GetReportSummaryResponse{From: report.From, To: report.To, Total: report.Total}
	for _, c := range report.Categories {
		resp.Categories = append(resp.Categories, &ledgerv1.CategoryReport{
			Category:      c.Category,
			Total:         c.Total,
			Budget:        c.Budget,
			BudgetPercent: c.BudgetPercent,
		})
	}
	return resp, nil
}

func (s *Server) ImportTransactions(ctx context.Context, req *ledgerv1.ImportTransactionsRequest) (*ledgerv1.ImportTransactionsResponse, error) {
	txs := make([]domain.Transaction, 0, len(req.GetTransactions()))
	for _, tx := range req.GetTransactions() {
		txs = append(txs, txFromPB(tx))
	}
	summary, err := s.svc.ImportTransactions(ctx, txs, int(req.GetWorkers()))
	if err != nil {
		return nil, toStatus("ImportTransactions", err)
	}
	return &ledgerv1.ImportTransactionsResponse{Summary: summaryToPB(summary)}, nil
}

func (s *Server) ImportCSV(ctx context.Context, req *ledgerv1.ImportCSVRequest) (*ledgerv1.ImportCSVResponse, error) {
	summary, err := s.svc.ImportCSV(ctx, bytes.NewReader(req.GetData()), int(req.GetWorkers()))
	if err != nil {
		return nil, toStatus("ImportCSV", err)
	}
	return &ledgerv1.ImportCSVResponse{Summary: summaryToPB(summary)}, nil
}

func (s *Server) ExportCSV(ctx context.Context, req *ledgerv1.ExportCSVRequest) (*ledgerv1.ExportCSVResponse, error) {
	var buf bytes.Buffer
	if err := s.svc.ExportCSV(ctx, &buf, req.GetFrom(), req.GetTo()); err != nil {
		return nil, toStatus("ExportCSV", err)
	}
	return &ledgerv1.ExportCSVResponse{Data: buf.Bytes()}, nil
}
