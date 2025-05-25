FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata wget && \
    adduser -D -s /bin/sh olla && \
    mkdir -p /app/logs /app/plugins && \
    chown -R olla:olla /app
WORKDIR /app
COPY --from=builder /app/bin/olla /usr/local/bin/olla
COPY --chown=olla:olla config.yaml ./
USER olla
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:19841/internal/health || exit 1
EXPOSE 19841
ENTRYPOINT ["olla"]