FROM node:22-alpine AS dashboard
WORKDIR /src
COPY web/dashboard/package*.json web/dashboard/
RUN cd web/dashboard && npm ci
COPY web/dashboard web/dashboard
RUN cd web/dashboard && npm run build

FROM golang:1.25-alpine AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=dashboard /src/internal/comrad/dashboard_static internal/comrad/dashboard_static
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X comrad/internal/comrad.Version=docker" -o /out/comrad-manager ./cmd/manager

FROM alpine:3.20
RUN apk add --no-cache ca-certificates \
  && adduser -D -H -u 10001 comrad \
  && mkdir -p /var/lib/comrad/artifacts \
  && chown -R comrad:comrad /var/lib/comrad
USER comrad
WORKDIR /var/lib/comrad
ENV COMRAD_MANAGER_ADDR=0.0.0.0:8080
ENV COMRAD_STORAGE_MODE=auto
ENV COMRAD_SQLITE_PATH=/var/lib/comrad/comrad.sqlite
ENV COMRAD_ARTIFACT_DIR=/var/lib/comrad/artifacts
COPY --from=builder /out/comrad-manager /usr/local/bin/comrad-manager
EXPOSE 8080
VOLUME ["/var/lib/comrad"]
ENTRYPOINT ["comrad-manager"]
