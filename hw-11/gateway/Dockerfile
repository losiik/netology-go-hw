FROM golang:1.27-alpine AS builder

WORKDIR /src/gateway

COPY --from=ledger go.mod go.sum /src/ledger/
COPY go.mod go.sum ./
RUN go mod download

COPY --from=ledger . /src/ledger/
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/gateway .

FROM alpine:latest

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/gateway ./gateway

USER app
EXPOSE 8080
CMD ["./gateway"]
