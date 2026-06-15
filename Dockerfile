FROM golang:1.26.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bot .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /bot /bot
RUN adduser -D -u 1000 app
USER app
WORKDIR /app
ENTRYPOINT ["/bot"]
