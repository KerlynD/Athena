# Market Intelligence Aggregator - Dockerfile
# Multi-stage build for Go + Python

# =============================================================================
# Stage 1: Build Go binaries
# =============================================================================
FROM golang:1.21-alpine AS go-builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build orchestrator
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/orchestrator ./cmd/orchestrator

# Build TUI
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/tui ./cmd/tui

# =============================================================================
# Stage 2: Runtime image with Go + Python
# =============================================================================
FROM python:3.11-slim

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy Go binaries from builder
COPY --from=go-builder /bin/orchestrator /bin/orchestrator
COPY --from=go-builder /bin/tui /bin/tui

# Set up Python environment
WORKDIR /app

# Copy Python requirements
COPY services/robinhood/requirements.txt /tmp/robinhood-requirements.txt
COPY services/analysis/requirements.txt /tmp/analysis-requirements.txt

# Install Python dependencies
RUN pip install --no-cache-dir -r /tmp/robinhood-requirements.txt \
    && pip install --no-cache-dir -r /tmp/analysis-requirements.txt

# Copy application code
COPY . .

# Set environment variables
ENV PYTHONUNBUFFERED=1
ENV PATH="/bin:${PATH}"

# Default command
CMD ["/bin/orchestrator", "run-all"]
