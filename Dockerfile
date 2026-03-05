# Always run builder on host (amd64) to avoid running arm64 Go toolchain under QEMU
FROM --platform=$BUILDPLATFORM golang:1.26.0-trixie AS builder

WORKDIR /app

# Multi-arch: buildx sets TARGETOS/TARGETARCH; we cross-compile from builder platform
ENV TARGETOS=linux
ENV TARGETARCH=arm64

COPY src/ /app/

RUN go mod download && GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o github-watchtower

FROM alpine:latest

COPY --from=builder /app/github-watchtower /usr/local/bin/github-watchtower

CMD [ "github-watchtower" ]