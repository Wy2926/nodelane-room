FROM node:24.21.0-bookworm-slim AS admin
WORKDIR /web
COPY internal/control/adminweb/package*.json ./
RUN npm ci
COPY internal/control/adminweb ./
RUN npm run build

FROM golang:1.26.8-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=admin /web/dist ./internal/control/adminweb/dist
COPY scripts/release ./scripts/release
COPY deploy/node.sh deploy/nlroom-node.service ./deploy/
COPY THIRD_PARTY_NOTICES.md ./
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/nodelane-server ./cmd/nodelane-server \
 && CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w' -o /out/nlroom-node ./cmd/nlroom-node
RUN set -eu; version=$(sed -n 's/^const NodeVersion = "\([^"]*\)"/\1/p' internal/model/version.go); for arch in amd64 arm64; do \
    bundle=/out/nodelane-room-node-$version-linux-$arch; mkdir -p "$bundle/licenses"; \
    CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -buildvcs=false -ldflags='-s -w' -o "$bundle/nlroom-node" ./cmd/nlroom-node; \
    cp THIRD_PARTY_NOTICES.md "$bundle/THIRD_PARTY_NOTICES.txt"; cp /usr/local/go/LICENSE "$bundle/licenses/Go-LICENSE"; \
    go version > "$bundle/BUILD.txt"; go list -m all >> "$bundle/BUILD.txt"; \
    go list -m -f '{{if .Replace}}{{.Replace.Dir}}{{else}}{{.Dir}}{{end}}' all | while read -r directory; do \
      [ -n "$directory" ] || continue; destination="$bundle/licenses/$(basename "$directory")"; mkdir -p "$destination"; \
      find "$directory" -maxdepth 1 -type f \( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' -o -iname 'PATENTS*' \) -exec cp {} "$destination/" \;; \
    done; \
  done; cp -r "/out/nodelane-room-node-$version-linux-amd64/licenses" /out/licenses; go run ./scripts/release -release /out -version "$version"

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/* && useradd --uid 10001 --create-home nodelane
COPY THIRD_PARTY_NOTICES.md /usr/share/doc/nodelane/THIRD_PARTY_NOTICES.txt
COPY --from=build /out/licenses /usr/share/doc/nodelane/licenses
WORKDIR /app

FROM runtime AS node
RUN mkdir -p /var/lib/nlroom-node && chmod 700 /var/lib/nlroom-node
COPY --from=build /out/nlroom-node /usr/local/bin/nlroom-node
ENV NLROOM_DEPLOYMENT=container
EXPOSE 4242/udp
HEALTHCHECK --interval=15s --timeout=3s --start-period=30s CMD curl --fail --silent http://127.0.0.1:9090/healthz || exit 1
ENTRYPOINT ["nlroom-node"]
CMD ["run"]

FROM runtime AS control
RUN mkdir -p /var/lib/nodelane-control && chown 10001:10001 /var/lib/nodelane-control && chmod 700 /var/lib/nodelane-control
COPY --from=build /out/nodelane-server /usr/local/bin/nodelane-server
COPY --from=build /out/releases/ /opt/nodelane/releases/
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s CMD curl --fail --silent http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["nodelane-server"]
CMD ["serve", "--listen", "0.0.0.0:8080", "--behind-proxy"]
