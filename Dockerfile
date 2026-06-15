FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bot .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /bot /bot
RUN adduser -D -u 1000 app
USER app
WORKDIR /app
ENTRYPOINT ["/bot"]
