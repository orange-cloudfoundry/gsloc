package main

import (
	"fmt"
	"github.com/alecthomas/kong"
	"github.com/orange-cloudfoundry/gsloc/app"
	"github.com/orange-cloudfoundry/gsloc/config"
)

var (
	version = "0.0.1-dev"
	commit  = "none"
	date    = "unknown"
)

type ServeCmd struct {
	ConfigPath   string `required:"" short:"c" help:"path to the configuration file" type:"path" default:"./config.yml"`
	OnlyServeDns bool   `help:"only serve dns and remove api, healthcheck and consul registering. This is for separate dns (data-plane) and backend/api (control-plane)" default:"false"`
	NoServeDns   bool   `help:"do not serve dns server and only have api, healthcheck and and consul registering. This is for separate dns (data-plane) and backend/api (control-plane)" default:"false"`
}

func (r *ServeCmd) Run() error {
	cnf, err := config.LoadConfig(r.ConfigPath)
	if err != nil {
		return fmt.Errorf("read config: %s", err)
	}
	appRun, err := app.NewApp(cnf, r.OnlyServeDns, r.NoServeDns)
	if err != nil {
		return fmt.Errorf("create app: %s", err)
	}
	return appRun.Run()
}

type VersionCmd struct {
}

func (l *VersionCmd) Run() error {
	fmt.Printf("gsloc %s, commit %s, built at %s", version, commit, date)
	return nil
}

var cli struct {
	Serve   ServeCmd   `cmd:"" help:"Run server."`
	Version VersionCmd `cmd:"" help:"Show version."`
}

func main() {
	ctx := kong.Parse(&cli)
	// Call the Run() method of the selected parsed command.
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}
