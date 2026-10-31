.PHONY: migrate run test generate

LOAD_ENV = set -a; [ ! -f .env ] || . ./.env; set +a;

migrate:
	$(LOAD_ENV) go tool goose -dir ./migrations postgres "$$DATABASE_URL" up

run:
	$(LOAD_ENV) go run ./cmd/trip-service

test:
	go test -race ./...

generate:
	go tool oapi-codegen -generate types,chi-server -package generated \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-o internal/generated/api.gen.go contracts/openapi/trip-service.openapi.yaml