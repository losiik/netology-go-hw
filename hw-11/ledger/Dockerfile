FROM golang:1.27-alpine AS builder

WORKDIR /src/ledger

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/ledger ./cmd/ledger

FROM alpine:latest

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/ledger ./ledger
COPY --from=builder /src/ledger/migrations ./migrations
COPY --from=builder /src/ledger/budgets.json ./budgets.json

USER app
CMD ["./ledger"]
