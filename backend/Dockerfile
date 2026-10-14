FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/seed-admin ./cmd/seed-admin
FROM alpine:3.22.1 AS runtime
RUN apk add --no-cache ca-certificates wget && addgroup -S app && adduser -S app -G app
COPY --from=build /out/server /usr/local/bin/server
COPY --from=build /out/seed-admin /usr/local/bin/seed-admin
USER app
WORKDIR /tmp
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s CMD wget -q -O /dev/null http://127.0.0.1:8080/api/v1/health/ready || exit 1
ENTRYPOINT ["server"]
