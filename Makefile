.PHONY: up down api mobile generate check smoke demo clean

up:            ## start mongo (single-node replica set, so transactions work)
	docker compose up -d
	@echo "waiting for mongo to report healthy..."
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' capacity-mongo 2>/dev/null)" = "healthy" ]; do sleep 2; done
	@echo "mongo ready on :27117"

down:
	docker compose down

api:           ## run the graphql api on :8080
	cd api && go run ./cmd/server

mobile:        ## run the expo client
	cd mobile && npm start

generate:      ## regenerate gqlgen code after editing graph/schema.graphqls
	cd api && go run github.com/99designs/gqlgen generate

check:         ## what we run before looking at your submission
	cd api && go build ./... && go vet ./... && go test ./...
	@[ -d mobile/node_modules ] || (echo "installing mobile deps..." && cd mobile && npm ci)
	cd mobile && npx tsc --noEmit

smoke:         ## walk every mutation and print each refusal sentence (needs make api)
	python3 scripts/smoke.py

demo:          ## put the seeded data in a known state for a demo (needs make api)
	python3 scripts/demo.py

clean:
	docker compose down -v
