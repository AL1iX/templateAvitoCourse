FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/trip-service ./cmd/trip-service

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/trip-service /trip-service

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/trip-service"]
