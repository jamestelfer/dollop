package configcmd

import (
	"context"
	"fmt"
	"slices"

	"github.com/jamestelfer/dollop/internal/config"
	"github.com/urfave/cli/v3"
)

// New returns the config command tree. cfgPath is the config file path;
// kr is the keyring store used by the auth subcommand.
func New(kr config.KeyringStore, cfgPath string) cli.Command {
	return cli.Command{
		Name:  "config",
		Usage: "manage dollop configuration",
		Description: `Configuration is split into two stores:

  File     (~/.config/dollop/config.yaml): non-sensitive settings such as
           bucket name and base URL. Use 'config set' and 'config get'.

  Keyring  (OS keyring): R2 credentials stored securely and never written
           to disk. Use 'config auth'.

Run 'dollop config <command> --help' for details on each subcommand.`,
		Commands: []*cli.Command{
			newSetCommand(cfgPath),
			newGetCommand(cfgPath),
			newListCommand(cfgPath),
			newAuthCommand(kr),
		},
	}
}

func newSetCommand(cfgPath string) *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     "set a config key",
		ArgsUsage: "<key> <value>",
		Description: `Writes a setting to the config file. Valid keys:

  bucket      R2 bucket name (e.g. my-uploads)
  account_id  Cloudflare account ID (from dash.cloudflare.com → right sidebar)
  base_url    public base URL for the bucket (e.g. https://files.example.com)

Example:
  dollop config set base_url https://files.example.com`,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 2 {
				return cli.Exit("usage: config set <key> <value>", 1)
			}
			key, value := cmd.Args().Get(0), cmd.Args().Get(1)
			cfg, err := config.LoadFrom(cfgPath)
			if err != nil {
				return cli.Exit(fmt.Sprintf("load config: %v", err), 1)
			}
			if err := cfg.Set(key, value); err != nil {
				return cli.Exit(err.Error(), 1)
			}
			if err := config.SaveTo(cfgPath, cfg); err != nil {
				return cli.Exit(fmt.Sprintf("save config: %v", err), 1)
			}
			return nil
		},
	}
}

func newGetCommand(cfgPath string) *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     "print the value of a config key",
		ArgsUsage: "<key>",
		Description: `Valid keys: bucket, account_id, base_url

Example:
  dollop config get bucket`,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 1 {
				return cli.Exit("usage: config get <key>", 1)
			}
			key := cmd.Args().Get(0)
			cfg, err := config.LoadFrom(cfgPath)
			if err != nil {
				return cli.Exit(fmt.Sprintf("load config: %v", err), 1)
			}
			val, err := cfg.Get(key)
			if err != nil {
				return cli.Exit(err.Error(), 1)
			}
			if _, err := fmt.Fprintln(cmd.Root().Writer, val); err != nil {
				return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
			}
			return nil
		},
	}
}

func newListCommand(cfgPath string) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "print all file config keys and values",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 0 {
				return cli.Exit("usage: config list", 1)
			}
			cfg, err := config.LoadFrom(cfgPath)
			if err != nil {
				return cli.Exit(fmt.Sprintf("load config: %v", err), 1)
			}
			for _, pair := range cfg.List() {
				if _, err := fmt.Fprintf(cmd.Root().Writer, "%s = %s\n", pair[0], pair[1]); err != nil {
					return cli.Exit(fmt.Sprintf("write output: %v", err), 1)
				}
			}
			return nil
		},
	}
}

func newAuthCommand(kr config.KeyringStore) *cli.Command {
	return &cli.Command{
		Name:      "auth",
		Usage:     "store an R2 credential in the OS keyring",
		ArgsUsage: "<key> <value>",
		Description: `Stores an R2 credential securely in the OS keyring under the "dollop"
service. Credentials are never written to the config file.

Valid keys:
  r2-key     S3-compatible access key ID issued by R2
  r2-secret  S3-compatible secret access key issued by R2

To obtain these, go to Cloudflare R2 → Manage R2 API Tokens and create
a token with read/write access to your bucket.

Example:
  dollop config auth r2-key     CAFEF00DCAFEF00D...
  dollop config auth r2-secret  abc123secretkey...`,
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 2 {
				return cli.Exit("usage: config auth <key> <value>", 1)
			}
			key, value := cmd.Args().Get(0), cmd.Args().Get(1)

			if !slices.Contains(config.AllowedKeyringKeys, key) {
				return cli.Exit(fmt.Sprintf("unknown keyring key %q", key), 1)
			}

			if err := kr.Set(config.ServiceName, key, value); err != nil {
				if _, werr := fmt.Fprintf(cmd.Root().ErrWriter, "keyring error: %v\n", err); werr != nil {
					return cli.Exit(fmt.Sprintf("write keyring error: %v", werr), 1)
				}
				return cli.Exit("failed to store credential in keyring", 1)
			}
			return nil
		},
	}
}
