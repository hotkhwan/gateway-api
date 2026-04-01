# Stage 1: Builder
FROM docker.io/library/golang:1.26 AS builder

WORKDIR /app

ENV GOPROXY=direct

RUN go install github.com/swaggo/swag/cmd/swag@latest

COPY go.mod go.sum ./
RUN go mod tidy

COPY . .

RUN swag init -g main.go -o ./docs

# Build แบบ Production
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o app .

# Stage 2: Runtime (image เล็กสุด)
FROM docker.io/library/alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /app/app .
COPY --from=builder /app/internal/services/authzsvc/schema.perm ./internal/services/authzsvc/schema.perm

# COPY --from=builder /app/.env .
# COPY --from=builder /app/static ./static

EXPOSE 3001

CMD ["./app"]