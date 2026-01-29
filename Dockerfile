# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

# Copy Go module files and download deps
COPY go.mod go.sum ./

# Allow downloading newer Go toolchain if required by go.mod
ENV GOTOOLCHAIN=auto
RUN go mod download

# Copy the source code
COPY . .

# Build the API binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /gtd_api ./cmd/api

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata wget

ENV TZ=Asia/Jakarta

RUN adduser -D -g '' appuser

WORKDIR /app

COPY --from=builder /gtd_api /app/gtd_api
COPY --from=builder /app/migrations /app/migrations

RUN chown -R appuser:appuser /app

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/v1/health || exit 1

CMD ["/app/gtd_api"]
