package main

import (
	"flag"

	"github.com/gonotelm-lab/flow/server/internal/app"
	"github.com/gonotelm-lab/flow/server/internal/config"
	"github.com/gonotelm-lab/flow/server/internal/repository"
)

var confPath = flag.String("conf", "./etc/conf.toml.tpl", "config file path")

func main() {
	flag.Parse()

	config.MustInit(*confPath)
	repository.MustInit(config.Conf.DB.Driver, config.Conf.DB.Config)

	repo := repository.Repo()
	defer repository.Close()
	app, err  := app.New(repo)
	if err != nil {
		panic(err)
	}

	app.Run()
}
