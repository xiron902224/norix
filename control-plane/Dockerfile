FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY control-plane/go.mod control-plane/go.sum ./
RUN go mod download
COPY control-plane/ ./
COPY --from=web /web/dist ./internal/webui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /norix-control-plane ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates \
    && addgroup -S norix \
    && adduser -S -D -H -G norix norix
USER norix
COPY --from=build /norix-control-plane /usr/local/bin/norix-control-plane
EXPOSE 8080
ENTRYPOINT ["norix-control-plane"]
