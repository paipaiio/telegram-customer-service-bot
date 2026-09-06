FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/forumdesk ./cmd/forumdesk

FROM alpine:3.21
RUN addgroup -S forumdesk && adduser -S -G forumdesk forumdesk
WORKDIR /app
COPY --from=build /out/forumdesk /usr/local/bin/forumdesk
RUN mkdir -p /app/data && chown -R forumdesk:forumdesk /app
USER forumdesk
ENV DATA_FILE=/app/data/forumdesk.json
VOLUME ["/app/data"]
ENTRYPOINT ["forumdesk"]
