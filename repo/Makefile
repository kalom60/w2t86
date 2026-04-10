.PHONY: build run vendor-assets docker-build docker-up docker-down docker-logs test

build:
	CGO_ENABLED=1 go build -tags sqlite_fts5 -o bin/server ./cmd/server

run:
	go run -tags sqlite_fts5 ./cmd/server

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-dev:
	docker compose -f docker-compose.yml -f docker-compose.dev.yml up

docker-down:
	docker compose down

docker-logs:
	docker compose logs -f portal

test:
	go test -tags sqlite_fts5 ./...

test-verbose:
	go test -tags sqlite_fts5 -v ./...

lint:
	go vet -tags sqlite_fts5 ./...

# Re-download vendored frontend assets into web/static/.
# Run this when you need to update library versions.
vendor-assets:
	@echo "Downloading frontend assets..."
	@mkdir -p web/static/js web/static/css web/static/css/fonts web/static/css/images
	curl -fsSL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js             -o web/static/js/htmx.min.js
	curl -fsSL https://unpkg.com/alpinejs@3.14.3/dist/cdn.min.js              -o web/static/js/alpine.min.js
	curl -fsSL https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/js/bootstrap.bundle.min.js \
	                                                                           -o web/static/js/bootstrap.bundle.min.js
	curl -fsSL https://unpkg.com/leaflet@1.9.4/dist/leaflet.js                -o web/static/js/leaflet.js
	curl -fsSL https://unpkg.com/htmx-ext-sse@2.2.2/sse.js                   -o web/static/js/htmx-ext-sse.js
	curl -fsSL https://cdn.jsdelivr.net/npm/bootstrap@5.3.3/dist/css/bootstrap.min.css \
	                                                                           -o web/static/css/bootstrap.min.css
	curl -fsSL https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/bootstrap-icons.min.css \
	                                                                           -o web/static/css/bootstrap-icons.min.css
	curl -fsSL https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/fonts/bootstrap-icons.woff2 \
	                                                                           -o web/static/css/fonts/bootstrap-icons.woff2
	curl -fsSL https://cdn.jsdelivr.net/npm/bootstrap-icons@1.11.3/font/fonts/bootstrap-icons.woff \
	                                                                           -o web/static/css/fonts/bootstrap-icons.woff
	curl -fsSL https://unpkg.com/leaflet@1.9.4/dist/leaflet.css               -o web/static/css/leaflet.css
	curl -fsSL https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon.png    -o web/static/css/images/marker-icon.png
	curl -fsSL https://unpkg.com/leaflet@1.9.4/dist/images/marker-icon-2x.png -o web/static/css/images/marker-icon-2x.png
	curl -fsSL https://unpkg.com/leaflet@1.9.4/dist/images/marker-shadow.png  -o web/static/css/images/marker-shadow.png
	curl -fsSL https://unpkg.com/leaflet@1.9.4/dist/images/layers.png         -o web/static/css/images/layers.png
	curl -fsSL https://unpkg.com/leaflet@1.9.4/dist/images/layers-2x.png      -o web/static/css/images/layers-2x.png
	@echo "Assets downloaded."
