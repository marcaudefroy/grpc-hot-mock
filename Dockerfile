# ─── Builder Stage ─────────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 \
    go build -trimpath -ldflags="-s -w" \
      -o /grpc-hot-mock ./cmd/main.go

# ─── Final Stage ──────────────────────────────────────────────────────────────
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/


# Metadata OCI
ARG VERSION=dev
LABEL org.opencontainers.image.title="grpc-hot-mock"
LABEL org.opencontainers.image.version=$VERSION
LABEL org.opencontainers.image.source="https://github.com/marcaudefroy/grpc-hot-mock"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder /grpc-hot-mock /usr/local/bin/grpc-hot-mock

EXPOSE 50051 8080

ENTRYPOINT ["/usr/local/bin/grpc-hot-mock"]