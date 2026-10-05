FROM golang:1.26.3-alpine AS build
RUN apk add --no-cache git
RUN go install github.com/minio/minio@RELEASE.2025-09-07T16-13-09Z
FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -S minio && adduser -S -G minio minio && mkdir /data && chown minio:minio /data
COPY --from=build /go/bin/minio /usr/local/bin/minio
USER minio
VOLUME /data
ENTRYPOINT ["minio"]
CMD ["server", "/data"]
