FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sandrone ./cmd/sandrone

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 sandrone
WORKDIR /app
COPY --from=build /out/sandrone /app/sandrone
COPY web /app/web
ENV SANDRONE_TEMPLATE_DIR=/app/web/template \
    SANDRONE_STATIC_DIR=/app/web/static \
    SANDRONE_HTTP_ADDR=:8080
USER sandrone
EXPOSE 8080
ENTRYPOINT ["/app/sandrone"]
