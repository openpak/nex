# Multi-stage, cross-compiled: the build stage runs on the runner's native arch and Go
# emits the target arch, so linux/arm64 builds need no QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
# nex-go-patched is a local `replace` target, so it has to be here before download
COPY go.mod go.sum ./
COPY nex-go-patched/ ./nex-go-patched/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/nexd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 65532 -h /app app
WORKDIR /app
COPY --from=build /out/app /app/app
RUN chown -R 65532:65532 /app
# Runs unprivileged: rootless podman maps this uid into the host's subuid range, and every
# NEX block port is >=22100 so no capability is needed to bind.
USER 65532:65532
ENTRYPOINT ["/app/app"]
