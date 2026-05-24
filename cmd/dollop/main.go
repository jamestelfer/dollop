package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jamestelfer/dollop/internal/cli/configcmd"
	"github.com/jamestelfer/dollop/internal/config"
	"github.com/urfave/cli/v3"
)

func main() {
	if err := run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	kr := config.NewSystemKeyring()
	cfgPath, err := config.ConfigFilePath()
	if err != nil {
		return err
	}

	cfgCmd := configcmd.New(kr, cfgPath)
	app := &cli.Command{
		Name:     "dollop",
		Usage:    "upload files and directories to a unique, expiring R2 path",
		Commands: []*cli.Command{&cfgCmd},
	}
	return app.Run(ctx, args)
}
