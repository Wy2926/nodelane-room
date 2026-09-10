FROM node:24.21.0-bookworm-slim AS admin
WORKDIR /web
COPY internal/control/adminweb/package*.json ./
RUN npm ci
COPY internal/control/adminweb ./
RUN npm run build && npm test

FROM golang:1.26.8-bookworm AS source
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=admin /web/dist ./internal/control/adminweb/dist
COPY scripts ./scripts
COPY docs ./docs
COPY desktop/package.json ./desktop/package.json

FROM source AS build
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/nlroom-cli ./cmd/nlroom-cli \
 && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/nlroom-service ./cmd/nlroom-service \
 && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/nlroom-node ./cmd/nlroom-node \
 && CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /out/nodelane-server ./cmd/nodelane-server

FROM debian:bookworm-slim AS runtime
RUN apt-get -o Acquire::Retries=3 update \
 && for attempt in 1 2 3; do \
      apt-get -o Acquire::Retries=3 install -y --no-install-recommends ca-certificates curl iproute2 openssl python3 && break; \
      if [ "$attempt" = 3 ]; then exit 1; fi; \
    done \
 && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ /usr/local/bin/
COPY deploy/test/entrypoint.sh deploy/test/peer.py deploy/test/admin.py /opt/test/
ENTRYPOINT ["bash", "/opt/test/entrypoint.sh"]
