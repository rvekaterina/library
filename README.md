# Library Service

## Description

Library Service is a gRPC service for managing books and authors with REST API support,
integrated with a PostgreSQL database.

## Peculiarities

- Service provides operations for managing authors and books,
  see the [library.proto](https://github.com/rvekaterina/library/blob/main/api/library/library.proto) file
- PostgreSQL database integration with migrations support
- REST to gRPC API support
- Configuration via environment variables

## Installation

### Generate code and build project

```bash 
make generate
```

## Start the service

### Service Configuration

The service requires the following environment variables:

#### Database Configuration

- `POSTGRES_HOST` - PostgreSQL host address
- `POSTGRES_PORT` - PostgreSQL port
- `POSTGRES_DB` - Database name
- `POSTGRES_USER` - Database username
- `POSTGRES_PASSWORD` - Database password
- `POSTGRES_MAX_CONN` - Maximum database connections

#### Server Configuration

- `GRPC_PORT` - Port for gRPC server
- `GRPC_GATEWAY_PORT` - Port for gRPC Gateway

#### Outbox Configuration

- `OUTBOX_ENABLED` - Enable/disable outbox (`true`/`false`)

if `OUTBOX_ENABLED=false` next environment variables are optional (unused):

- `OUTBOX_WORKERS` - Number of outbox workers
- `OUTBOX_BATCH_SIZE` - Maximum number of messages to process in one batch
- `OUTBOX_WAIT_TIME_MS` - Delay between processing cycles in milliseconds
- `OUTBOX_IN_PROGRESS_TTL_MS` - Time in milliseconds after which messages are reprocessed
- `OUTBOX_AUTHOR_SEND_URL` - Endpoint URL for register author event handling
- `OUTBOX_BOOK_SEND_URL` - Endpoint URL for add book event handling
- `OUTBOX_RETRY_ENABLED` - optional argument to enable/disable dynamic .yaml config for
  retrying tasks in outbox. If it is not set, default values for
  outbox.retry_ttl_ms and outbox.max_attempts will be set, otherwise .yaml file in
  project root with name specified in `OUTBOX_CONFIG_FILE_NAME` variable will be used
  to get outbox.retry_ttl_ms and outbox.max_attempts values.

Before starting the service, ensure that you have access to a PostgreSQL database.
You may use an existing database or set up a new one, for example, by running the PostgreSQL database locally using
Docker.
You can start the service from the project directory by explicitly specifying the environment variables, for example:

```bash 
env GRPC_PORT=9090 GRPC_GATEWAY_PORT=8080 \
    POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=5432 POSTGRES_DB=library \
    POSTGRES_USER=user POSTGRES_PASSWORD=12345 POSTGRES_MAX_CONN=10 \
    OUTBOX_ENABLED=true OUTBOX_WORKERS=3 OUTBOX_BATCH_SIZE=100 \
    OUTBOX_WAIT_TIME_MS=3000 OUTBOX_IN_PROGRESS_TTL_MS=10000 OUTBOX_AUTHOR_SEND_URL=https://service/author \
    OUTBOX_BOOK_SEND_URL=https://service/book \
    go run ./cmd/library/main.go
```

## API

### Validation

- The book and author's IDs must be in the format [UUID](https://ru.wikipedia.org/wiki/UUID)
- Author's name must be allowed by the regular expression `^[A-Za-z0-9]+( [A-Za-z0-9]+)*$`,
  len(author_name) must be in [1; 512]
- Len(book_name) must be >= 1

Each gRPC method corresponds to an HTTP endpoint:

### RegisterAuthor

```POST /v1/library/author```

### ChangeAuthorInfo

```PUT /v1/library/author```

### GetAuthorInfo

```GET /v1/library/author/{id}```

### GetAuthorBooks

```GET /v1/library/author_books/{authorId}```

### AddBook

```POST /v1/library/book```

### UpdateBookInfo

```PUT /v1/library/book```

### GetBookInfo

```GET /v1/library/book/{id}```