FROM golang:1.24 AS build
ARG CODEX_DOCK_BUILD_VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=secret,id=proxy_ca \
    if [ -r /run/secrets/proxy_ca ]; then export SSL_CERT_FILE=/run/secrets/proxy_ca; fi; \
    go mod download
COPY . .
RUN --mount=type=secret,id=proxy_ca \
    if [ -r /run/secrets/proxy_ca ]; then export SSL_CERT_FILE=/run/secrets/proxy_ca; fi; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X github.com/pacificbelt30/codex-dock/internal/buildinfo.Version=${CODEX_DOCK_BUILD_VERSION}" -o /out/codex-dock .

FROM gcr.io/distroless/base-debian12
ARG CODEX_DOCK_BUILD_VERSION=dev
LABEL io.codex-dock.encrypted-store="stdin-key-v1"
LABEL io.codex-dock.proxy-protocol="external-store-v1"
LABEL io.codex-dock.proxy-build=$CODEX_DOCK_BUILD_VERSION
COPY --from=build /out/codex-dock /codex-dock
ENTRYPOINT ["/codex-dock"]
CMD ["proxy", "serve", "--listen", "0.0.0.0:18080"]
