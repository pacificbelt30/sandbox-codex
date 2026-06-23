FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/codex-dock .

FROM gcr.io/distroless/base-debian12
COPY --from=build /out/codex-dock /codex-dock
ENTRYPOINT ["/codex-dock"]
CMD ["proxy", "serve", "--listen", "0.0.0.0:18080"]
