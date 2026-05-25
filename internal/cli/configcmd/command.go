package configcmd

import (
	"context"
	"fmt"

	"github.com/jamestelfer/dollop/internal/config"
	"github.com/urfave/cli/v3"
)

// New returns the config command tree. cfgPath is the config file path;
// kr is the keyring store used by the auth subcommand.
func New(kr config.KeyringStore, cfgPath string) cli.Command {
	return cli.Command{
		Name:  "config",
		Usage: "manage dollop configuration",
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
		Usage:     "set a config key to a value",
		ArgsUsage: "<key> <value>",
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
		Usage: "print all config keys and values",
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
		Usage:     "store a credential in the OS keyring",
		ArgsUsage: "<key> <value>",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 2 {
				return cli.Exit("usage: config auth <key> <value>", 1)
			}
			key, value := cmd.Args().Get(0), cmd.Args().Get(1)

			valid := false
			for _, k := range config.AllowedKeyringKeys {
				if k == key {
					valid = true
					break
				}
			}
			if !valid {
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
