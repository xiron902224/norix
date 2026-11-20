# Thin runtime for a prebuilt control-plane binary (bind-mounted in compose).
FROM alpine:3.21
RUN apk add --no-cache ca-certificates curl \
    && addgroup -S norix \
    && adduser -S -D -H -G norix norix
USER norix
EXPOSE 8080
ENTRYPOINT ["norix-control-plane"]
