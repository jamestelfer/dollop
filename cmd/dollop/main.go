package main

import (
	"context"
	"fmt"
	"os"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/jamestelfer/dollop/internal/cli/configcmd"
	"github.com/jamestelfer/dollop/internal/cli/createcmd"
	"github.com/jamestelfer/dollop/internal/config"
	"github.com/jamestelfer/dollop/internal/upload"
	nanoid "github.com/matoous/go-nanoid/v2"
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
		Name:  "dollop",
		Usage: "publish files as shareable, expiring browser links",
		Description: `dollop publishes local files or directories as browser-accessible links.
Each 'create' prints a unique URL you can share; the link expires after a
configurable number of days (default: 1). Use --keep for a permanent link.

First-time setup (run once):
  dollop config set bucket      <bucket-name>
  dollop config set account_id  <cloudflare-account-id>
  dollop config set base_url    <https://your-bucket-public-url>
  dollop config auth r2-key     <r2-access-key-id>
  dollop config auth r2-secret  <r2-secret-access-key>

Quick start:
  dollop create photo.jpg              # share a file; link expires in 1 day
  dollop create --days 7 archive.zip   # share a file; link expires in 7 days
  dollop create --keep project/        # share a directory with a permanent link`,
		Commands: []*cli.Command{&cfgCmd, &createCmd},
	}
	return app.Run(ctx, args)
}
