# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25 AS builder

# Build arguments for version injection
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Copy and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build for the target platform with version injection
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s \
    -X main.Version=${VERSION} \
    -X main.GitCommit=${GIT_COMMIT} \
    -X main.BuildDate=${BUILD_DATE}" \
    -o forwardauth main.go

# Final stage - using distroless for security
FROM gcr.io/distroless/static-debian12:nonroot

# Build arguments for labels
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

# Set Go runtime environment for better memory management
ENV GOGC=50
ENV GOMEMLIMIT=100MiB

# Add OCI image labels for better container management
LABEL org.opencontainers.image.title="ELLIO Traefik ForwardAuth" \
      org.opencontainers.image.description="High-performance forward authentication server for Traefik" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${GIT_COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/ELLIO-Technology/ellio-forwardauth-docker" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.vendor="ELLIO Technology"

# Copy the binary
COPY --from=builder /app/forwardauth /forwardauth

# Copy static files
COPY static /static

EXPOSE 8080 9090

ENTRYPOINT ["/forwardauth"]