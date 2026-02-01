FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod .
COPY backend ./backend
RUN CGO_ENABLED=0 go build -o /app/server ./backend/cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=build /app/server /app/server
EXPOSE 8080
CMD ["/app/server"]
