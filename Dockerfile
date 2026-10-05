FROM node:22.19.0-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
RUN npm run build

FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/spotter-server ./cmd/spotter-server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S spotter && adduser -S -G spotter spotter
WORKDIR /app
COPY --from=build /out/spotter-server /usr/local/bin/spotter-server
COPY --from=frontend /src/frontend/dist ./web
USER spotter
EXPOSE 8080
ENTRYPOINT ["spotter-server"]
CMD ["-role", "api"]
