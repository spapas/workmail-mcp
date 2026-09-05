package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spapas/workmail-mcp/internal/auth"
	"github.com/spapas/workmail-mcp/internal/config"
)

// RunHTTP serves streamable HTTP MCP on the configured loopback address.
func RunHTTP(ctx context.Context, server *mcpsdk.Server, cfg config.Config, logger *slog.Logger) error {
	if err := cfg.ValidateHTTP(); err != nil {
		return err
	}
	mcpHandler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, &mcpsdk.StreamableHTTPOptions{Stateless: true})
	mux := http.NewServeMux()
	mux.Handle("/mcp", auth.Bearer(cfg.APIToken, mcpHandler))

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.OperationTimeout + 5*time.Second,
		WriteTimeout:      cfg.OperationTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	errCh := make(chan error, 1)
	go func() {
		if logger != nil {
			logger.Info("MCP HTTP listening", "addr", cfg.HTTPAddr, "path", "/mcp")
		}
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	}
}
