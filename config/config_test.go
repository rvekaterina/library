package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestConfig(t *testing.T) {
	envVars := map[string]string{
		"GRPC_PORT":         "9090",
		"GRPC_GATEWAY_PORT": "8080",
		"POSTGRES_HOST":     "127.0.0.1",
		"POSTGRES_PORT":     "5432",
		"POSTGRES_DB":       "library",
		"POSTGRES_USER":     "user",
		"POSTGRES_PASSWORD": "password",
		"POSTGRES_MAX_CONN": "10",
		"OUTBOX_ENABLED":    "false",
	}

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr error
		dburl   string
	}{
		{
			name:  "No error",
			dburl: "postgres://user:password@127.0.0.1:5432/library?sslmode=disable&pool_max_conns=10",
		},
		{
			name:    "Missing GRPC_PORT",
			key:     "GRPC_PORT",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing GRPC_GATEWAY_PORT",
			key:     "GRPC_GATEWAY_PORT",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing POSTGRES_HOST",
			key:     "POSTGRES_HOST",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing POSTGRES_PORT",
			key:     "POSTGRES_PORT",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing POSTGRES_DB",
			key:     "POSTGRES_DB",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing POSTGRES_USER",
			key:     "POSTGRES_USER",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing POSTGRES_PASSWORD",
			key:     "POSTGRES_PASSWORD",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing POSTGRES_MAX_CONN",
			key:     "POSTGRES_MAX_CONN",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Cannot parse OUTBOX_ENABLED",
			key:     "OUTBOX_ENABLED",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr != nil {
				_ = os.Unsetenv(tc.key)
			}

			for k, v := range envVars {
				if k == tc.key {
					continue
				}
				t.Setenv(k, v)
			}

			cfg, err := New()
			if tc.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.wantErr)
				require.Contains(t, err.Error(), tc.key)
				require.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				require.Equal(t, tc.dburl, cfg.URL)
				require.Equal(t, envVars["GRPC_PORT"], cfg.GRPC.Port)
				require.Equal(t, envVars["GRPC_GATEWAY_PORT"], cfg.GRPC.GatewayPort)
				require.Equal(t, envVars["POSTGRES_HOST"], cfg.PG.Host)
				require.Equal(t, envVars["POSTGRES_PORT"], cfg.PG.Port)
				require.Equal(t, envVars["POSTGRES_DB"], cfg.PG.DB)
				require.Equal(t, envVars["POSTGRES_USER"], cfg.PG.User)
				require.Equal(t, envVars["POSTGRES_PASSWORD"], cfg.PG.Password)
				require.Equal(t, envVars["POSTGRES_MAX_CONN"], cfg.PG.MaxConn)
				val, _ := strconv.ParseBool(envVars["OUTBOX_ENABLED"])
				require.Equal(t, val, cfg.Outbox.Enabled)
			}
		})
	}
}

func TestConfigSpecialCases(t *testing.T) {
	t.Run("user and password with special chars", func(t *testing.T) {
		envVars := map[string]string{
			"GRPC_PORT":         "9090",
			"GRPC_GATEWAY_PORT": "8080",
			"POSTGRES_HOST":     "127.0.0.1",
			"POSTGRES_PORT":     "5432",
			"POSTGRES_DB":       "library",
			"POSTGRES_USER":     "u:?s/e&r",
			"POSTGRES_PASSWORD": "5&/?@34",
			"POSTGRES_MAX_CONN": "10",
			"OUTBOX_ENABLED":    "false",
		}

		for k, v := range envVars {
			t.Setenv(k, v)
		}
		cfg, err := New()
		require.NoError(t, err)
		require.NotNil(t, cfg)
		expectedURL := fmt.Sprintf(
			"postgres://%s:%s@127.0.0.1:5432/library?sslmode=disable&pool_max_conns=10",
			url.QueryEscape(cfg.PG.User),
			url.QueryEscape(cfg.PG.Password))
		require.Equal(t, expectedURL, cfg.PG.URL)
	})
}

func TestOutboxConfig(t *testing.T) {
	envVars := map[string]string{
		"GRPC_PORT":                 "9090",
		"GRPC_GATEWAY_PORT":         "8080",
		"POSTGRES_HOST":             "127.0.0.1",
		"POSTGRES_PORT":             "5432",
		"POSTGRES_DB":               "library",
		"POSTGRES_USER":             "user",
		"POSTGRES_PASSWORD":         "password",
		"POSTGRES_MAX_CONN":         "10",
		"OUTBOX_ENABLED":            "true",
		"OUTBOX_WORKERS":            "3",
		"OUTBOX_BATCH_SIZE":         "100",
		"OUTBOX_WAIT_TIME_MS":       "100",
		"OUTBOX_IN_PROGRESS_TTL_MS": "100",
		"OUTBOX_AUTHOR_SEND_URL":    "https://example.com/author",
		"OUTBOX_BOOK_SEND_URL":      "https://example.com/book",
	}

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr error
		dburl   string
	}{
		{
			name:  "No error",
			dburl: "postgres://user:password@127.0.0.1:5432/library?sslmode=disable&pool_max_conns=10",
		},
		{
			name:    "Missing OUTBOX_WORKERS",
			key:     "OUTBOX_WORKERS",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing OUTBOX_BATCH_SIZE",
			key:     "OUTBOX_BATCH_SIZE",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing OUTBOX_WAIT_TIME_MS",
			key:     "OUTBOX_WAIT_TIME_MS",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing OUTBOX_IN_PROGRESS_TTL_MS",
			key:     "OUTBOX_IN_PROGRESS_TTL_MS",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing OUTBOX_AUTHOR_SEND_URL",
			key:     "OUTBOX_AUTHOR_SEND_URL",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "Missing OUTBOX_BOOK_SEND_URL",
			key:     "OUTBOX_BOOK_SEND_URL",
			value:   "",
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr != nil {
				_ = os.Unsetenv(tc.key)
			}

			for k, v := range envVars {
				if k == tc.key {
					continue
				}
				t.Setenv(k, v)
			}

			cfg, err := New()
			if tc.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tc.wantErr)
				require.Contains(t, err.Error(), tc.key)
				require.Nil(t, cfg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, cfg)
				require.Equal(t, tc.dburl, cfg.URL)
				require.Equal(t, envVars["GRPC_PORT"], cfg.GRPC.Port)
				require.Equal(t, envVars["GRPC_GATEWAY_PORT"], cfg.GRPC.GatewayPort)
				require.Equal(t, envVars["POSTGRES_HOST"], cfg.PG.Host)
				require.Equal(t, envVars["POSTGRES_PORT"], cfg.PG.Port)
				require.Equal(t, envVars["POSTGRES_DB"], cfg.PG.DB)
				require.Equal(t, envVars["POSTGRES_USER"], cfg.PG.User)
				require.Equal(t, envVars["POSTGRES_PASSWORD"], cfg.PG.Password)
				require.Equal(t, envVars["POSTGRES_MAX_CONN"], cfg.PG.MaxConn)

				require.True(t, cfg.Outbox.Enabled)
				require.Equal(t, 3, cfg.Outbox.Workers)
				require.Equal(t, 100, cfg.Outbox.BatchSize)

				interval := time.Duration(100) * time.Millisecond
				require.Equal(t, interval, cfg.Outbox.WaitTimeMS)
				require.Equal(t, interval, cfg.Outbox.InProgressTTLMS)

				require.Equal(t, envVars["OUTBOX_AUTHOR_SEND_URL"], cfg.Outbox.AuthorSendURL)
				require.Equal(t, envVars["OUTBOX_BOOK_SEND_URL"], cfg.BookSendURL)
			}
		})
	}
}

func TestRetryConfig(t *testing.T) {
	envVars := map[string]string{
		"GRPC_PORT":               "9090",
		"GRPC_GATEWAY_PORT":       "8080",
		"POSTGRES_HOST":           "127.0.0.1",
		"POSTGRES_PORT":           "5432",
		"POSTGRES_DB":             "library",
		"POSTGRES_USER":           "user",
		"POSTGRES_PASSWORD":       "password",
		"POSTGRES_MAX_CONN":       "10",
		"OUTBOX_ENABLED":          "false",
		"OUTBOX_RETRY_ENABLED":    "true",
		"OUTBOX_CONFIG_FILE_NAME": "outbox-config.yaml",
	}

	tests := []struct {
		name    string
		value   string
		enabled bool
	}{
		{
			name:    "No error",
			enabled: true,
			value:   "true",
		},
		{
			name:    "Error",
			enabled: false,
			value:   "false",
		},
		{
			name:    "OUTBOX_RETRY_ENABLED not set",
			enabled: false,
			value:   "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range envVars {
				if k == "OUTBOX_RETRY_ENABLED" {
					t.Setenv(k, tc.value)
					continue
				}
				t.Setenv(k, v)
			}

			cfg, err := New()
			require.NoError(t, err)
			require.Equal(t, tc.enabled, cfg.Outbox.RetryEnabled)
			require.Equal(t, defaultMaxAttempts, cfg.Outbox.MaxAttempts)
			require.Equal(t, defaultRetryTTL, cfg.Outbox.RetryTTLMS)
			if tc.enabled {
				require.Equal(t, envVars["OUTBOX_CONFIG_FILE_NAME"], cfg.Outbox.ConfigFileName)
			}
		})
	}
}
