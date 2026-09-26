.PHONY: migrate run test generate

migrate:
	go tool goose -dir ./migrations postgres "$$DATABASE_URL" up

run:
	go run ./cmd/trip-service

test:
	go test -race ./...

generate:
	go tool oapi-codegen -generate types,chi-server -package api \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-o api/api.gen.go contracts/openapi/trip-service.openapi.yaml