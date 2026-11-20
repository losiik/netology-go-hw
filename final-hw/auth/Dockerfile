FROM golang:1.27-alpine AS builder

WORKDIR /src
COPY proto/go.mod proto/go.sum ./proto/
COPY auth/go.mod auth/go.sum ./auth/
RUN cd auth && go mod download

COPY proto ./proto
COPY auth ./auth
RUN cd auth && CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/auth ./cmd/auth

FROM alpine:latest

RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /out/auth ./auth

USER app
EXPOSE 9091
CMD ["./auth"]
