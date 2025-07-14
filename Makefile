.PHONY: build-tests test up down pull-images

# ====================================================================================
# DOCKER COMPOSE
# ====================================================================================

up:
	@echo "Starting services with docker-compose..."
	docker-compose up -d --build

down:
	@echo "Stopping services with docker-compose..."
	docker-compose down

pull-images:
	@echo "Pulling test images..."
	docker pull confluentinc/cp-zookeeper:7.1.1
	docker pull confluentinc/cp-kafka:7.1.1
	docker pull redis:7.2.5-alpine
	docker pull postgres:16.3-alpine
	docker pull testcontainers/ryuk:0.8.1

# ====================================================================================
# TESTING
# ====================================================================================

build-tests:
	@echo "Building test service image..."
	docker-compose build tests

test: pull-images build-tests
	@echo "Running tests..."
	docker-compose run --rm tests 