FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata wget && \
    adduser -D -s /bin/sh olla && \
    mkdir -p /app && \
    chown -R olla:olla /app

WORKDIR /app

COPY . .
COPY olla /usr/local/bin/olla

RUN chown -R olla:olla /app

USER olla

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:40114/internal/health || exit 1

EXPOSE 40114
ENTRYPOINT ["olla"]