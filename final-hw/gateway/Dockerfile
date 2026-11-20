FROM golang:1.27-alpine AS builder

WORKDIR /src
COPY proto/go.mod proto/go.sum ./proto/
COPY gateway/go.mod gateway/go.sum ./gateway/
RUN cd gateway && go mod download

COPY proto ./proto
COPY gateway ./gateway
RUN cd gateway && CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/gateway .

FROM alpine:latest

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/gateway ./gateway

USER app
EXPOSE 8080
CMD ["./gateway"]
