package main

import (
	"context"
	"flag"
	"log/slog"

	"github.com/gonotelm-lab/flow/server/internal/app"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/otel"
	"github.com/gonotelm-lab/flow/server/internal/repository"
)

var confPath = flag.String("conf", "./etc/conf.toml.tpl", "config file path")

func main() {
	flag.Parse()

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	config.MustInit(*confPath)
	initTelemetry(rootCtx)

	repository.MustInit(config.Conf.DB.Driver, config.Conf.DB.Config)

	repo := repository.Repo()
	defer repository.Close()
	app, err := app.New(repo, rootCtx)
	if err != nil {
		panic(err)
	}

	app.Run()
}

func initTelemetry(ctx context.Context) {
	oc := config.Conf.Otel
	if oc == nil || !oc.Enabled {
		return
	}
	// 初始化失败不中断启动，仅降级为无追踪
	if err := otel.Init(ctx, oc); err != nil {
		slog.WarnContext(ctx, "[otel] init failed, tracing disabled", slog.Any("err", err))
		return
	}
	slog.InfoContext(ctx, "[otel] tracing enabled")
}
