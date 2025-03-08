package main

import (
	"os"

	"github.com/rvekaterina/library/config"
	"github.com/rvekaterina/library/internal/app"
	log "github.com/sirupsen/logrus"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.New()

	if err != nil {
		log.Fatalf("can not get application config: %s", err)
	}

	var logger *zap.Logger

	logger, err = zap.NewProduction()

	if err != nil {
		log.Fatalf("can not initialize logger: %s", err)
	}

	if err = app.Run(logger, cfg); err != nil {
		os.Exit(-1)
	}
}
