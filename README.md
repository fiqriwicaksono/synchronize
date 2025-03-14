# Synchronization Game

Simple avatars game to simulate client-server synchronization

## Overall Architecture
The synchronization engine implements a client-server architecture where:

- Server: Maintains the authoritative game state and broadcasts updates to all connected clients.

- Clients: Apply local inputs immediately while receiving and reconciling with server updates.

## Client-side Prediction and Reconciliation

The client employs a prediction-reconciliation system to provide responsive gameplay:

When a player presses a key, the client:
- Immediately applies the input locally
- Sends the input to the server with a sequence number
- Continues to predict movement until server confirmation

## Building, Running, and Testing

### Prerequisites

- Go 1.20 or higher
- Docker (for containerized deployment)

### Building

Local build:
```
make build
```
Docker build:
```
make docker-build
```

### Running

Run locally:
```
# Start the server
make run-server

# In another terminal, start the client
make run-client
```

Run with Docker:
```
# Start server in Docker
make docker-server

# In another terminal, start the client with Docker
make docker-client
```

### Testing
Run all tests:
```
make test
```

### Game Controls

- Arrow keys: Move your avatar
- Other keys: Stop your avatar
- ESC: Quit