# syntax=docker/dockerfile:1

FROM golang:1.26 AS builder

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

WORKDIR /src

COPY api/go.mod api/go.sum ./api/
COPY server/go.mod server/go.sum ./server/

RUN cd server && go mod download

COPY api/ ./api/
COPY server/ ./server/

RUN cd server && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/flowserver ./cmd/flowserver

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /src/server/etc ./etc
COPY --from=builder /out/flowserver ./flowserver

EXPOSE 7090 7091

ENTRYPOINT ["/app/flowserver", "-conf", "/app/etc/conf.toml.tpl"]
