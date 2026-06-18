package main

import (
	"flag"
	"log/slog"

	"github.com/gonotelm-lab/flow/server/internal/config"
)

func main() {
	confPath := flag.String("conf", "./etc/conf.toml.tpl", "config file path")
	flag.Parse()

	config.MustInit(*confPath)

	slog.Info("config initialized", slog.String("path", *confPath))
}
