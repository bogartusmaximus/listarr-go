# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/listarr-go ./cmd/listarr-go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget \
	&& adduser -D -H -u 65532 listarr \
	&& mkdir -p /data/polars \
	&& chown -R listarr:listarr /data
USER listarr
WORKDIR /app
COPY --from=build /out/listarr-go /app/listarr-go
# Container must bind all interfaces; host binary default remains 127.0.0.1.
ENV LISTARR_LISTEN=0.0.0.0:8787 \
	LISTARR_STORE_BACKEND=polars \
	LISTARR_POLARS_DIR=/data/polars
EXPOSE 8787
VOLUME /data
HEALTHCHECK --interval=15s --timeout=3s --start-period=5s --retries=3 \
	CMD wget -qO- http://127.0.0.1:8787/health || exit 1
ENTRYPOINT ["/app/listarr-go"]
