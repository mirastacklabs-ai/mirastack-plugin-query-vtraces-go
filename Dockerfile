# MIRASTACK Plugin — Query VTraces Go (multi-arch: linux/amd64, linux/arm64)
#
# Build:
#   docker buildx build --platform linux/amd64,linux/arm64 \
#     -f agents/oss/mirastack-plugin-query-vtraces-go/Dockerfile .

FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Copy plugin module
COPY agents/oss/mirastack-plugin-query-vtraces-go/go.mod agents/oss/mirastack-plugin-query-vtraces-go/go.sum* agents/oss/mirastack-plugin-query-vtraces-go/
WORKDIR /src/agents/oss/mirastack-plugin-query-vtraces-go
RUN go mod download

WORKDIR /src
COPY agents/oss/mirastack-plugin-query-vtraces-go/ agents/oss/mirastack-plugin-query-vtraces-go/

WORKDIR /src/agents/oss/mirastack-plugin-query-vtraces-go
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w" -o /mirastack-plugin-query-vtraces .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /mirastack-plugin-query-vtraces /usr/local/bin/mirastack-plugin-query-vtraces
EXPOSE 50051
ENTRYPOINT ["mirastack-plugin-query-vtraces"]
