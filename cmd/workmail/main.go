package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/spapas/workmail-mcp/internal/config"
	imapbackend "github.com/spapas/workmail-mcp/internal/imap"
	mcpserver "github.com/spapas/workmail-mcp/internal/mcp"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "workmail-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	command := "help"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "help", "-h", "--help":
		printHelp()
		return nil
	case "version", "--version":
		fmt.Println(version)
		return nil
	case "token":
		token, err := generateToken()
		if err != nil {
			return err
		}
		fmt.Println(token)
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	backend := imapbackend.New(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	switch command {
	case "doctor":
		if err := cfg.ValidateIMAP(); err != nil {
			return fmt.Errorf("configuration: %w", err)
		}
		if err := backend.Doctor(ctx); err != nil {
			return err
		}
		fmt.Println("OK: IMAPS TLS, authentication, and folder listing succeeded")
		return nil
	case "stdio":
		if err := cfg.ValidateIMAP(); err != nil {
			return fmt.Errorf("configuration: %w", err)
		}
		server := mcpserver.NewServer(backend, cfg, logger, version)
		return mcpserver.RunStdio(ctx, server)
	case "serve":
		if err := cfg.ValidateHTTP(); err != nil {
			return fmt.Errorf("configuration: %w", err)
		}
		server := mcpserver.NewServer(backend, cfg, logger, version)
		return mcpserver.RunHTTP(ctx, server, cfg, logger)
	default:
		return fmt.Errorf("unknown command %q; run workmail-mcp help", command)
	}
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func printHelp() {
	fmt.Printf(`workmail-mcp %s

Usage:
  workmail-mcp stdio    Run MCP over stdin/stdout (recommended for Hermes)
  workmail-mcp serve    Run authenticated streamable HTTP MCP on loopback
  workmail-mcp doctor   Verify IMAPS TLS/login and folder listing
  workmail-mcp token    Generate a random 256-bit bearer token
  workmail-mcp version  Print version
  workmail-mcp help     Show this help

Configuration is supplied through WORKMAIL_* environment variables or *_FILE secret paths.
See README.md for the complete configuration reference.
`, version)
}
