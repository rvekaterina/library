package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	rt "runtime"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/spf13/viper"

	"github.com/rvekaterina/library/internal/entity"
	"github.com/rvekaterina/library/internal/usecase/outbox"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rvekaterina/library/config"
	"github.com/rvekaterina/library/db"
	generated "github.com/rvekaterina/library/generated/api/library"
	"github.com/rvekaterina/library/internal/controller"
	"github.com/rvekaterina/library/internal/usecase/library"
	"github.com/rvekaterina/library/internal/usecase/repository"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

const (
	shutdownTimeout       = 3 * time.Second
	maxConn               = 100
	connTimeout           = 90 * time.Second
	handshakeTimeout      = 15 * time.Second
	expectContinueTimeout = 2 * time.Second
	timeout               = 30 * time.Second
	keepAlive             = 180 * time.Second
	internalStatusCode    = http.StatusInternalServerError
)

func Run(logger *zap.Logger, cfg *config.Config) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.PG.URL)
	if err != nil {
		logger.Error("failed to connect to database", zap.Error(err))
		return err
	}
	defer pool.Close()

	db.SetupPostgres(pool, logger)

	repo := repository.New(pool, logger)
	outboxRepo := repository.NewOutbox(pool, logger)

	transactor := repository.NewTransactor(pool, logger)
	runOutbox(ctx, cfg, logger, outboxRepo, transactor)

	useCases := library.New(logger, repo, repo, outboxRepo, transactor)
	ctrl := controller.New(logger, useCases, useCases)
	go runRest(ctx, cfg, logger)
	go runGrpc(cfg, logger, ctrl)

	<-ctx.Done()
	time.Sleep(shutdownTimeout)
	logger.Info("shutting down server")
	return nil
}

func runRest(ctx context.Context, cfg *config.Config, logger *zap.Logger) {
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	address := "localhost:" + cfg.GRPC.Port

	err := generated.RegisterLibraryHandlerFromEndpoint(ctx, mux, address, opts)
	if err != nil {
		logger.Error("can not register grpc gateway", zap.Error(err))
		os.Exit(-1)
	}

	gatewayPort := ":" + cfg.GRPC.GatewayPort
	logger.Info("gateway listening at port", zap.String("port", gatewayPort))
	if err = http.ListenAndServe(gatewayPort, mux); err != nil {
		logger.Error("gateway listen error", zap.Error(err))
	}
}

func runGrpc(cfg *config.Config, logger *zap.Logger, libraryService generated.LibraryServer) {
	port := ":" + cfg.GRPC.Port
	lis, err := net.Listen("tcp", port)
	if err != nil {
		logger.Error("can not open tcp socket", zap.Error(err))
		os.Exit(-1)
	}
	s := grpc.NewServer()

	reflection.Register(s)
	generated.RegisterLibraryServer(s, libraryService)

	logger.Info("grpc server listening at port", zap.String("port", port))
	if err = s.Serve(lis); err != nil {
		logger.Error("grpc server listen error", zap.Error(err))
	}
}

func setupViper(logger *zap.Logger, cfg *config.Config) {
	if cfg.Outbox.RetryEnabled {
		v := viper.New()

		_, filename, _, _ := rt.Caller(0)
		configPath := filepath.Join(filepath.Dir(filename), "../../"+cfg.Outbox.ConfigFileName)

		v.SetConfigFile(configPath)
		v.SetConfigType("yaml")

		if err := v.ReadInConfig(); err != nil {
			logger.Error("failed to read config", zap.Error(err))
			return
		}

		cfg.Outbox.MaxAttempts = v.GetInt("outbox.max_attempts")
		cfg.Outbox.RetryTTLMS = v.GetDuration("outbox.retry_ttl_ms") * time.Millisecond

		v.WatchConfig()
		v.OnConfigChange(func(_ fsnotify.Event) {
			cfg.Outbox.Mutex.Lock()
			defer cfg.Outbox.Mutex.Unlock()
			cfg.Outbox.MaxAttempts = v.GetInt("outbox.max_attempts")
			cfg.Outbox.RetryTTLMS = v.GetDuration("outbox.retry_ttl_ms") * time.Millisecond
		})
	}
}

func runOutbox(
	ctx context.Context,
	cfg *config.Config,
	logger *zap.Logger,
	outboxRepository repository.OutboxRepository,
	transactor repository.Transactor,
) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: keepAlive,
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          maxConn,
		MaxConnsPerHost:       maxConn,
		IdleConnTimeout:       connTimeout,
		TLSHandshakeTimeout:   handshakeTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
		MaxIdleConnsPerHost:   rt.GOMAXPROCS(0) + 1,
	}

	client := new(http.Client)
	client.Transport = transport

	globalHandler := globalOutboxHandler(client, cfg.Outbox.AuthorSendURL, cfg.Outbox.BookSendURL)

	setupViper(logger, cfg)
	outboxService := outbox.New(logger, outboxRepository, globalHandler, cfg, transactor)

	outboxService.Start(
		ctx,
		cfg.Outbox.Workers,
		cfg.Outbox.BatchSize,
		cfg.Outbox.WaitTimeMS,
		cfg.Outbox.InProgressTTLMS,
	)
}

func globalOutboxHandler(
	client *http.Client,
	authorURL string,
	bookURL string,
) outbox.GlobalHandler {
	return func(kind repository.OutboxKind) (outbox.KindHandler, error) {
		switch kind {
		case repository.OutboxKindBook:
			return bookOutboxHandler(client, bookURL), nil
		case repository.OutboxKindAuthor:
			return authorOutboxHandler(client, authorURL), nil
		default:
			return nil, fmt.Errorf("outbox kind %d not supported", kind)
		}
	}
}

func authorOutboxHandler(client *http.Client, url string) outbox.KindHandler {
	return func(_ context.Context, data []byte) error {
		author := entity.Author{}

		err := json.Unmarshal(data, &author)
		if err != nil {
			return fmt.Errorf("cannot deserialize data in author outbox handler: %w", err)
		}

		resp, err := client.Post(url, "application/json", strings.NewReader(author.ID))
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode >= internalStatusCode {
				return fmt.Errorf("%w in author outbox handler: status code: %d", outbox.ErrInternal, resp.StatusCode)
			}
			return fmt.Errorf("error in author outbox handler: %d", resp.StatusCode)
		}
		return nil
	}
}

func bookOutboxHandler(client *http.Client, url string) outbox.KindHandler {
	return func(_ context.Context, data []byte) error {
		book := entity.Book{}

		err := json.Unmarshal(data, &book)
		if err != nil {
			return fmt.Errorf("cannot deserialize data in book outbox handler: %w", err)
		}

		resp, err := client.Post(url, "application/json", strings.NewReader(book.ID))
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode >= internalStatusCode {
				return fmt.Errorf("%w: status code in book outbox handler: %d", outbox.ErrInternal, resp.StatusCode)
			}
			return fmt.Errorf("error in book outbox handler: %d", resp.StatusCode)
		}
		return nil
	}
}
