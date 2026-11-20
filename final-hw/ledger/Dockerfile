FROM golang:1.27-alpine AS builder

WORKDIR /src
COPY proto/go.mod proto/go.sum ./proto/
COPY ledger/go.mod ledger/go.sum ./ledger/
RUN cd ledger && go mod download

COPY proto ./proto
COPY ledger ./ledger
RUN cd ledger && CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/ledger ./cmd/ledger

FROM alpine:latest

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/ledger ./ledger

USER app
EXPOSE 9090
CMD ["./ledger"]
