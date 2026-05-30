package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/jamestelfer/dollop/internal/buildinfo"
	"github.com/jamestelfer/dollop/internal/cli/configcmd"
	"github.com/jamestelfer/dollop/internal/cli/createcmd"
	"github.com/jamestelfer/dollop/internal/cli/doctorcmd"
	"github.com/jamestelfer/dollop/internal/cli/updatecmd"
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
	authPath, err := config.AuthFilePath()
	if err != nil {
		return err
	}
	pt := config.NewPlaintextStore(authPath)
	readKr := config.NewFallbackStore(kr, pt)

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	accessKey, accessPlaintext, _ := readKr.GetWithSource(config.ServiceName, "r2-key")
	secretKey, secretPlaintext, _ := readKr.GetWithSource(config.ServiceName, "r2-secret")
	// Credentials live in the OS secrets storage unless they were read from the
	// plain-text fallback file.
	secureStorage := !accessPlaintext && !secretPlaintext

	var uploader upload.Uploader
	var lister upload.BucketLister
	if cfg.AccountID != "" && accessKey != "" && secretKey != "" {
		s3up, s3err := upload.NewS3Uploader(cfg.AccountID, accessKey, secretKey)
		if s3err != nil {
			return fmt.Errorf("init uploader: %w", s3err)
		}
		uploader = s3up
		lister = s3up
	}

	cfgCmd := configcmd.New(kr, pt, cfgPath)
	createCmd := createcmd.New(
		uploader,
		cfg.Bucket,
		cfg.BaseURL,
		func() (string, error) { return nanoid.New() },
		func() string { return petname.Generate(2, "-") },
	)
	updateCmd := updatecmd.New(
		uploader,
		cfg.Bucket,
		cfg.BaseURL,
	)
	doctorCmd := doctorcmd.New(
		cfg,
		cfgPath,
		authPath,
		secureStorage,
		accessKey != "",
		secretKey != "",
		uploader,
		lister,
		func() (string, error) { return nanoid.New() },
		func(ctx context.Context, url string) (*http.Response, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			return http.DefaultClient.Do(req) //nolint:bodyclose
		},
	)

	cli.VersionPrinter = func(cmd *cli.Command) {
		_ = buildinfo.Fprint(cmd.Root().Writer, cmd.Root().Name)
	}

	app := &cli.Command{
		Name:    "dollop",
		Version: buildinfo.Version,
		Usage:   "publish files as shareable, expiring browser links",
		Description: `dollop publishes local files or directories as browser-accessible links.
Each 'create' prints a unique URL you can share; the link expires after a
configurable number of days (default: 1). Use --keep for a permanent link.

First-time setup (run once):
  dollop config set bucket      <bucket-name>
  dollop config set account-id  <cloudflare-account-id>
  dollop config set base-url    <https://your-bucket-public-url>
  dollop config auth r2-key     <r2-access-key-id>
  dollop config auth r2-secret  <r2-secret-access-key>

Quick start:
  dollop create photo.jpg              # share a file; link expires in 1 day
  dollop create --days 7 archive.zip   # share a file; link expires in 7 days
  dollop create --keep project/        # share a directory with a permanent link`,
		Commands: []*cli.Command{&cfgCmd, &createCmd, &updateCmd, &doctorCmd},
	}
	return app.Run(ctx, args)
}
