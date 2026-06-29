FROM alpine:3.20 AS certs

RUN apk --no-cache add ca-certificates

FROM scratch

COPY icsmcp /usr/local/bin/icsmcp
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 65532:65532
EXPOSE 3333
ENTRYPOINT ["/usr/local/bin/icsmcp"]
CMD ["serve", "--http-addr", "0.0.0.0:3333", "--db-path", "/data/icsmcp.sqlite3"]
