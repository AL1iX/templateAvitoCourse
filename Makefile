.PHONY: migrate run generate lint test test-integration

migrate:
	go tool goose -dir ./migrations postgres "$$DATABASE_URL" up

run:
	go run ./cmd/trip-service

generate:
	go tool oapi-codegen -generate types,chi-server -package generated \
		-include-operation-ids createTrip,getTrip,finishTrip,health,ready \
		-o internal/generated/api.gen.go contracts/openapi/trip-service.openapi.yaml
	go generate ./...

lint:
	go tool golangci-lint run ./...

test:
	go test -race -count=2 -cover ./...

test-integration:
	go test -race -count=1 -tags integration ./...
