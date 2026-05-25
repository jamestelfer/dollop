package main

import (
	"context"
	"fmt"
	"os"

	petname "github.com/dustinkirkland/golang-petname"
	nanoid "github.com/matoous/go-nanoid/v2"
	"github.com/jamestelfer/dollop/internal/cli/configcmd"
	"github.com/jamestelfer/dollop/internal/cli/createcmd"
	"github.com/jamestelfer/dollop/internal/config"
	"github.com/jamestelfer/dollop/internal/upload"
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

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	accessKey, _ := kr.Get(config.ServiceName, "r2-key")
	secretKey, _ := kr.Get(config.ServiceName, "r2-secret")

	var uploader upload.Uploader
	if cfg.AccountID != "" && accessKey != "" && secretKey != "" {
		uploader, err = upload.NewS3Uploader(cfg.AccountID, accessKey, secretKey)
		if err != nil {
			return fmt.Errorf("init uploader: %w", err)
		}
	}

	cfgCmd := configcmd.New(kr, cfgPath)
	createCmd := createcmd.New(
		uploader,
		cfg.Bucket,
		cfg.BaseURL,
		func() (string, error) { return nanoid.New() },
		func() string { return petname.Generate(2, "-") },
	)

	app := &cli.Command{
		Name:     "dollop",
		Usage:    "upload files and directories to a unique, expiring R2 path",
		Commands: []*cli.Command{&cfgCmd, &createCmd},
	}
	return app.Run(ctx, args)
}
