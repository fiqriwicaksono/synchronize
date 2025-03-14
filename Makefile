# Makefile
.PHONY: build test clean run-server run-client docker-build docker-server docker-client

# Go related variables
GOBASE=$(shell pwd)
GOBIN=$(GOBASE)/bin

# Docker related variables
DOCKER_SERVER_IMAGE=sync-server
DOCKER_CLIENT_IMAGE=sync-client

# Build binaries locally
build:
	@echo "Building server and client..."
	go build -o $(GOBIN)/server ./cmd/server
	go build -o $(GOBIN)/client ./cmd/client

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run server locally
run-server:
	@echo "Starting server..."
	go run cmd/server/main.go

# Run client locally
run-client:
	@echo "Starting client..."
	go run cmd/client/main.go

# Build Docker images
docker-build:
	@echo "Building Docker images..."
	docker build  --no-cache -f Dockerfile.server -t $(DOCKER_SERVER_IMAGE) .
	docker build  --no-cache -f Dockerfile.client -t $(DOCKER_CLIENT_IMAGE) .

# Run server in Docker
docker-server:
	@echo "Starting server in Docker..."
	docker run -p 8080:8080 --name sync-server --rm $(DOCKER_SERVER_IMAGE)

# Run client in Docker, connecting to a running server
docker-client:
	@echo "Starting client in Docker..."
	docker run -it --network="host" --name sync-client --rm $(DOCKER_CLIENT_IMAGE)

# Clean up
clean:
	@echo "Cleaning up..."
	rm -rf $(GOBIN)
	docker rmi $(DOCKER_SERVER_IMAGE) $(DOCKER_CLIENT_IMAGE) 2>/dev/null || true