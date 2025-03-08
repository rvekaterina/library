package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const (
	defaultWaitTime    = 100 * time.Millisecond
	defaultRetryTTL    = 1000 * time.Millisecond
	defaultMaxAttempts = 10
)

var ErrInvalidConfig = errors.New("invalid config")

type (
	Config struct {
		GRPC
		PG
		Outbox
	}

	GRPC struct {
		Port        string `env:"GRPC_PORT"`
		GatewayPort string `env:"GRPC_GATEWAY_PORT"`
	}

	PG struct {
		URL      string
		Host     string `env:"POSTGRES_HOST"`
		Port     string `env:"POSTGRES_PORT"`
		DB       string `env:"POSTGRES_DB"`
		User     string `env:"POSTGRES_USER"`
		Password string `env:"POSTGRES_PASSWORD"`
		MaxConn  string `env:"POSTGRES_MAX_CONN"`
	}

	Outbox struct {
		Mutex           *sync.RWMutex
		Enabled         bool          `env:"OUTBOX_ENABLED"`
		RetryEnabled    bool          `env:"OUTBOX_RETRY_ENABLED"`
		Workers         int           `env:"OUTBOX_WORKERS"`
		BatchSize       int           `env:"OUTBOX_BATCH_SIZE"`
		WaitTimeMS      time.Duration `env:"OUTBOX_WAIT_TIME_MS"`
		InProgressTTLMS time.Duration `env:"OUTBOX_IN_PROGRESS_TTL_MS"`
		AuthorSendURL   string        `env:"OUTBOX_AUTHOR_SEND_URL"`
		BookSendURL     string        `env:"OUTBOX_BOOK_SEND_URL"`
		ConfigFileName  string        `env:"OUTBOX_CONFIG_FILE_NAME"`
		RetryTTLMS      time.Duration
		MaxAttempts     int
	}
)

func New() (*Config, error) {
	cfg := &Config{}
	cfg.Outbox.Mutex = new(sync.RWMutex)
	cfg.GRPC.Port = os.Getenv("GRPC_PORT")
	cfg.GRPC.GatewayPort = os.Getenv("GRPC_GATEWAY_PORT")

	cfg.PG.Host = os.Getenv("POSTGRES_HOST")
	cfg.PG.Port = os.Getenv("POSTGRES_PORT")
	cfg.PG.DB = os.Getenv("POSTGRES_DB")
	cfg.PG.User = os.Getenv("POSTGRES_USER")
	cfg.PG.Password = os.Getenv("POSTGRES_PASSWORD")
	cfg.PG.MaxConn = os.Getenv("POSTGRES_MAX_CONN")

	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	cfg.PG.URL = fmt.Sprintf(
		"postgres://%s:%s@%s/%s?sslmode=disable&pool_max_conns=%s",
		url.QueryEscape(cfg.PG.User),
		url.QueryEscape(cfg.PG.Password),
		net.JoinHostPort(cfg.PG.Host, cfg.PG.Port),
		cfg.PG.DB,
		cfg.PG.MaxConn,
	)

	var err error
	cfg.Outbox.Enabled, err = strconv.ParseBool(os.Getenv("OUTBOX_ENABLED"))
	if err != nil {
		return nil, fmt.Errorf("%w, failed to parse OUTBOX_ENABLED: %w", ErrInvalidConfig, err)
	}

	cfg.Outbox.RetryEnabled, _ = strconv.ParseBool(os.Getenv("OUTBOX_RETRY_ENABLED"))

	cfg.Outbox.RetryTTLMS = defaultRetryTTL
	cfg.Outbox.MaxAttempts = defaultMaxAttempts

	if cfg.Outbox.RetryEnabled {
		cfg.Outbox.ConfigFileName = os.Getenv("OUTBOX_CONFIG_FILE_NAME")
	}

	if cfg.Outbox.Enabled {
		cfg.Outbox.Workers, err = parseInt(os.Getenv("OUTBOX_WORKERS"))
		if err != nil {
			return nil, fmt.Errorf("%w, failed to parse OUTBOX_WORKERS: %w", ErrInvalidConfig, err)
		}
		cfg.Outbox.BatchSize, err = parseInt(os.Getenv("OUTBOX_BATCH_SIZE"))
		if err != nil {
			return nil, fmt.Errorf("%w, failed to parse OUTBOX_BATCH_SIZE: %w", ErrInvalidConfig, err)
		}

		cfg.Outbox.WaitTimeMS, err = parseTime(os.Getenv("OUTBOX_WAIT_TIME_MS"))
		if err != nil {
			return nil, fmt.Errorf("%w, failed to parse OUTBOX_WAIT_TIME_MS: %w", ErrInvalidConfig, err)
		}

		cfg.Outbox.InProgressTTLMS, err = parseTime(os.Getenv("OUTBOX_IN_PROGRESS_TTL_MS"))
		if err != nil {
			return nil, fmt.Errorf("%w, failed to parse OUTBOX_IN_PROGRESS_TTL_MS: %w", ErrInvalidConfig, err)
		}
		cfg.Outbox.AuthorSendURL = os.Getenv("OUTBOX_AUTHOR_SEND_URL")
		cfg.Outbox.BookSendURL = os.Getenv("OUTBOX_BOOK_SEND_URL")

		if err = validateOutboxConfig(cfg); err != nil {
			return nil, err
		}
	} else {
		cfg.Outbox.WaitTimeMS = defaultWaitTime
	}
	return cfg, nil
}

func parseTime(s string) (time.Duration, error) {
	t, err := parseInt(s)
	if err != nil {
		return time.Duration(0), err
	}
	return time.Duration(t) * time.Millisecond, nil
}

func parseInt(s string) (int, error) {
	str, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return int(str), nil
}

func checkConfig(fields map[string]string) error {
	for k, v := range fields {
		if v == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalidConfig, k)
		}
	}
	return nil
}

func validateConfig(cfg *Config) error {
	return checkConfig(map[string]string{
		"GRPC_PORT":         cfg.GRPC.Port,
		"GRPC_GATEWAY_PORT": cfg.GRPC.GatewayPort,
		"POSTGRES_HOST":     cfg.PG.Host,
		"POSTGRES_PORT":     cfg.PG.Port,
		"POSTGRES_DB":       cfg.PG.DB,
		"POSTGRES_USER":     cfg.PG.User,
		"POSTGRES_PASSWORD": cfg.PG.Password,
		"POSTGRES_MAX_CONN": cfg.PG.MaxConn,
	})
}

func validateOutboxConfig(cfg *Config) error {
	return checkConfig(map[string]string{
		"OUTBOX_WORKERS":            strconv.Itoa(cfg.Outbox.Workers),
		"OUTBOX_BATCH_SIZE":         strconv.Itoa(cfg.Outbox.BatchSize),
		"OUTBOX_WAIT_TIME_MS":       strconv.FormatInt(cfg.Outbox.WaitTimeMS.Milliseconds(), 10),
		"OUTBOX_IN_PROGRESS_TTL_MS": strconv.FormatInt(cfg.Outbox.InProgressTTLMS.Milliseconds(), 10),
		"OUTBOX_AUTHOR_SEND_URL":    cfg.Outbox.AuthorSendURL,
		"OUTBOX_BOOK_SEND_URL":      cfg.Outbox.BookSendURL,
	})
}
