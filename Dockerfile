# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./

# Copy vendor directory (for air-gapped builds)
COPY vendor/ vendor/

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build using vendor directory (works in air-gapped/disconnected environments)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -a -o webhook ./cmd/webhook

# Final stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/webhook .

USER 65532:65532

ENTRYPOINT ["/webhook"]
