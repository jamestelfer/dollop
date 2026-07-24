package servecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/urfave/cli/v3"
)

// EnvConfig holds the values read from environment variables for the serve
// subcommand. All fields except Addr are required.
type EnvConfig struct {
	AccountID string
	Bucket    string
	BaseURL   string
	R2Key     string
	R2Secret  string
	Addr      string // defaults to ":8080"
}

// ReadEnv reads the serve configuration from environment variables. It returns
// the config and a list of missing required variable names.
func ReadEnv() (EnvConfig, []string) {
	cfg := EnvConfig{
		AccountID: os.Getenv("DOLLOP_ACCOUNT_ID"),
		Bucket:    os.Getenv("DOLLOP_BUCKET"),
		BaseURL:   os.Getenv("DOLLOP_BASE_URL"),
		R2Key:     os.Getenv("DOLLOP_R2_KEY"),
		R2Secret:  os.Getenv("DOLLOP_R2_SECRET"),
		Addr:      os.Getenv("DOLLOP_ADDR"),
	}
	if cfg.Addr == "" {
		cfg.Addr = ":8080"
	}

	var missing []string
	if cfg.AccountID == "" {
		missing = append(missing, "DOLLOP_ACCOUNT_ID")
	}
	if cfg.Bucket == "" {
		missing = append(missing, "DOLLOP_BUCKET")
	}
	if cfg.BaseURL == "" {
		missing = append(missing, "DOLLOP_BASE_URL")
	}
	if cfg.R2Key == "" {
		missing = append(missing, "DOLLOP_R2_KEY")
	}
	if cfg.R2Secret == "" {
		missing = append(missing, "DOLLOP_R2_SECRET")
	}
	return cfg, missing
}

// New returns the serve command. readEnv is injected for testability.
func New(readEnv func() (EnvConfig, []string)) cli.Command {
	return cli.Command{
		Name:  "serve",
		Usage: "start the MCP server for agent file publishing",
		Description: `Starts an MCP server that exposes a create_upload tool over
Streamable HTTP. Configuration is read from environment variables:

  DOLLOP_ACCOUNT_ID   Cloudflare account ID (required)
  DOLLOP_BUCKET       R2 bucket name (required)
  DOLLOP_BASE_URL     Public URL base for uploaded files (required)
  DOLLOP_R2_KEY       R2 access key ID (required)
  DOLLOP_R2_SECRET    R2 secret access key (required)
  DOLLOP_ADDR         Listen address (default: :8080)`,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, missing := readEnv()
			if len(missing) > 0 {
				return cli.Exit(
					fmt.Sprintf("missing required environment variable(s): %s", strings.Join(missing, ", ")),
					1,
				)
			}

			return runServer(ctx, cfg, cmd.Root().ErrWriter)
		},
	}
}

// shutdownTimeout is the maximum time the server waits for in-flight requests
// to complete during graceful shutdown.
const shutdownTimeout = 5 * time.Second

func runServer(ctx context.Context, cfg EnvConfig, stderr io.Writer) error {
	mcpServer := server.NewMCPServer(
		"dollop",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	httpMCP := server.NewStreamableHTTPServer(mcpServer)

	mux := http.NewServeMux()
	mux.Handle("/mcp", httpMCP)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Graceful shutdown on SIGTERM/SIGINT.
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(stderr, "dollop serve listening on %s\n", cfg.Addr) //nolint:errcheck
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	// Block until shutdown signal or server error.
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	case <-shutdownCtx.Done():
		fmt.Fprintln(stderr, "shutting down...") //nolint:errcheck
		sdCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(sdCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}

	return nil
}
