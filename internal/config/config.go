package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultIMAPPort                 = 993
	defaultHTTPAddr                 = "127.0.0.1:8787"
	defaultFolder                   = "INBOX"
	defaultMaxResults               = 50
	defaultMaxQueryLength           = 512
	defaultMaxMessageBytes    int64 = 25 << 20
	defaultMaxBodyBytes       int64 = 512 << 10
	defaultMaxAttachmentBytes int64 = 10 << 20
	defaultMaxThreadMessages        = 50
	defaultOperationTimeout         = 30 * time.Second
)

// Config contains runtime settings for the workmail bridge.
type Config struct {
	IMAPHost           string
	IMAPPort           int
	IMAPUsername       string
	IMAPPassword       string
	HTTPAddr           string
	APIToken           string
	DefaultFolder      string
	MaxResults         int
	MaxQueryLength     int
	MaxMessageBytes    int64
	MaxBodyBytes       int64
	MaxAttachmentBytes int64
	MaxThreadMessages  int
	OperationTimeout   time.Duration
}

// Load reads configuration from environment variables and optional *_FILE secret sources.
func Load() (Config, error) {
	password, err := readSecret("WORKMAIL_IMAP_PASSWORD", "WORKMAIL_IMAP_PASSWORD_FILE")
	if err != nil {
		return Config{}, err
	}
	token, err := readSecret("WORKMAIL_API_TOKEN", "WORKMAIL_API_TOKEN_FILE")
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		IMAPHost:           strings.TrimSpace(os.Getenv("WORKMAIL_IMAP_HOST")),
		IMAPPort:           defaultIMAPPort,
		IMAPUsername:       strings.TrimSpace(os.Getenv("WORKMAIL_IMAP_USERNAME")),
		IMAPPassword:       password,
		HTTPAddr:           envOr("WORKMAIL_HTTP_ADDR", defaultHTTPAddr),
		APIToken:           token,
		DefaultFolder:      envOr("WORKMAIL_DEFAULT_FOLDER", defaultFolder),
		MaxResults:         defaultMaxResults,
		MaxQueryLength:     defaultMaxQueryLength,
		MaxMessageBytes:    defaultMaxMessageBytes,
		MaxBodyBytes:       defaultMaxBodyBytes,
		MaxAttachmentBytes: defaultMaxAttachmentBytes,
		MaxThreadMessages:  defaultMaxThreadMessages,
		OperationTimeout:   defaultOperationTimeout,
	}

	if err := loadInt("WORKMAIL_IMAP_PORT", &cfg.IMAPPort); err != nil {
		return Config{}, err
	}
	if err := loadInt("WORKMAIL_MAX_RESULTS", &cfg.MaxResults); err != nil {
		return Config{}, err
	}
	if err := loadInt("WORKMAIL_MAX_QUERY_LENGTH", &cfg.MaxQueryLength); err != nil {
		return Config{}, err
	}
	if err := loadInt64("WORKMAIL_MAX_MESSAGE_BYTES", &cfg.MaxMessageBytes); err != nil {
		return Config{}, err
	}
	if err := loadInt64("WORKMAIL_MAX_BODY_BYTES", &cfg.MaxBodyBytes); err != nil {
		return Config{}, err
	}
	if err := loadInt64("WORKMAIL_MAX_ATTACHMENT_BYTES", &cfg.MaxAttachmentBytes); err != nil {
		return Config{}, err
	}
	if err := loadInt("WORKMAIL_MAX_THREAD_MESSAGES", &cfg.MaxThreadMessages); err != nil {
		return Config{}, err
	}
	if raw := strings.TrimSpace(os.Getenv("WORKMAIL_OPERATION_TIMEOUT")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("WORKMAIL_OPERATION_TIMEOUT: %w", err)
		}
		cfg.OperationTimeout = d
	}
	return cfg, cfg.validateLimits()
}

// ValidateIMAP checks settings required for mailbox access.
func (c Config) ValidateIMAP() error {
	var errs []error
	if c.IMAPHost == "" {
		errs = append(errs, errors.New("WORKMAIL_IMAP_HOST is required"))
	}
	if c.IMAPPort < 1 || c.IMAPPort > 65535 {
		errs = append(errs, errors.New("WORKMAIL_IMAP_PORT must be between 1 and 65535"))
	}
	if c.IMAPUsername == "" {
		errs = append(errs, errors.New("WORKMAIL_IMAP_USERNAME is required"))
	}
	if c.IMAPPassword == "" || strings.EqualFold(c.IMAPPassword, "replace-me") {
		errs = append(errs, errors.New("WORKMAIL_IMAP_PASSWORD or WORKMAIL_IMAP_PASSWORD_FILE is required"))
	}
	if err := validateFolder(c.DefaultFolder); err != nil {
		errs = append(errs, fmt.Errorf("WORKMAIL_DEFAULT_FOLDER: %w", err))
	}
	return errors.Join(errs...)
}

// ValidateHTTP checks settings required for localhost HTTP MCP mode.
func (c Config) ValidateHTTP() error {
	var errs []error
	if err := c.ValidateIMAP(); err != nil {
		errs = append(errs, err)
	}
	if err := ValidateLoopbackAddr(c.HTTPAddr); err != nil {
		errs = append(errs, err)
	}
	if len(c.APIToken) < 32 || strings.HasPrefix(strings.ToLower(c.APIToken), "replace-") {
		errs = append(errs, errors.New("WORKMAIL_API_TOKEN or WORKMAIL_API_TOKEN_FILE must contain a non-placeholder token of at least 32 characters"))
	}
	return errors.Join(errs...)
}

// ValidateLoopbackAddr rejects addresses that can expose the HTTP server beyond localhost.
func ValidateLoopbackAddr(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("WORKMAIL_HTTP_ADDR must be host:port: %w", err)
	}
	if port == "" {
		return errors.New("WORKMAIL_HTTP_ADDR requires a port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("WORKMAIL_HTTP_ADDR must use a loopback IP such as 127.0.0.1 or ::1, got %q", host)
	}
	return nil
}

func (c Config) validateLimits() error {
	if c.MaxResults < 1 || c.MaxResults > 500 {
		return errors.New("WORKMAIL_MAX_RESULTS must be between 1 and 500")
	}
	if c.MaxQueryLength < 1 || c.MaxQueryLength > 8192 {
		return errors.New("WORKMAIL_MAX_QUERY_LENGTH must be between 1 and 8192")
	}
	if c.MaxMessageBytes < 1024 || c.MaxMessageBytes > 100<<20 {
		return errors.New("WORKMAIL_MAX_MESSAGE_BYTES must be between 1024 and 104857600")
	}
	if c.MaxBodyBytes < 1024 || c.MaxBodyBytes > c.MaxMessageBytes {
		return errors.New("WORKMAIL_MAX_BODY_BYTES must be at least 1024 and no greater than WORKMAIL_MAX_MESSAGE_BYTES")
	}
	if c.MaxAttachmentBytes < 1024 || c.MaxAttachmentBytes > c.MaxMessageBytes {
		return errors.New("WORKMAIL_MAX_ATTACHMENT_BYTES must be at least 1024 and no greater than WORKMAIL_MAX_MESSAGE_BYTES")
	}
	if c.MaxThreadMessages < 1 || c.MaxThreadMessages > 200 {
		return errors.New("WORKMAIL_MAX_THREAD_MESSAGES must be between 1 and 200")
	}
	if c.OperationTimeout < time.Second || c.OperationTimeout > 5*time.Minute {
		return errors.New("WORKMAIL_OPERATION_TIMEOUT must be between 1s and 5m")
	}
	return nil
}

func validateFolder(folder string) error {
	if strings.TrimSpace(folder) == "" {
		return errors.New("folder must not be empty")
	}
	if len(folder) > 1024 || strings.ContainsAny(folder, "\x00\r\n") {
		return errors.New("folder name is invalid")
	}
	return nil
}

func readSecret(valueVar, fileVar string) (string, error) {
	value := strings.TrimSpace(os.Getenv(valueVar))
	path := strings.TrimSpace(os.Getenv(fileVar))
	if value != "" && path != "" {
		return "", fmt.Errorf("set only one of %s or %s", valueVar, fileVar)
	}
	if path == "" {
		return value, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", fileVar, err)
	}
	return strings.TrimSpace(string(b)), nil
}

func envOr(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func loadInt(name string, dst *int) error {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = v
	return nil
}

func loadInt64(name string, dst *int64) error {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	*dst = v
	return nil
}
