FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/trip-service ./cmd/trip-service

FROM alpine:3.20

COPY --from=build /out/trip-service /usr/local/bin/trip-service

USER nobody
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/trip-service"]
