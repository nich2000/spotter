FROM golang:1.22-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/spotter ./cmd/spotter

FROM alpine:3.20

WORKDIR /app
COPY --from=build /out/spotter /usr/local/bin/spotter
RUN printf '%s\n' '#!/bin/sh' 'echo "macOS Automation is unavailable in Docker. Run Personal Spotter natively on macOS to use Calendar, Reminders, Mail and Notes." >&2' 'exit 1' > /usr/local/bin/osascript \
	&& chmod +x /usr/local/bin/osascript
COPY config.docker.yaml ./config.yaml
COPY scripts ./scripts
COPY web ./web

ENV SPOTTER_DOCKER=1
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/spotter"]
CMD ["-config", "/app/config.yaml"]
