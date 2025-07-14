.PHONY: build-tests run-tests up down

# ====================================================================================
# DOCKER COMPOSE
# ====================================================================================

up:
	@echo "Starting services with docker-compose..."
	docker-compose up -d --build

down:
	@echo "Stopping services with docker-compose..."
	docker-compose down

# ====================================================================================
# TESTING
# ====================================================================================

build-tests:
	@echo "Building test service image..."
	docker-compose build tests

run-tests: build-tests
	@echo "Running tests..."
	docker-compose run --rm tests 