package daemonstate

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

const serviceConfigFile = "service.env"

// ServiceConfig contains the private credentials for a managed service.
type ServiceConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// DefaultConfigDir returns Wingman's XDG configuration directory.
func DefaultConfigDir() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "wingman"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "wingman"), nil
}

// ServiceConfigPath returns the private managed-service configuration path.
func ServiceConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, serviceConfigFile), nil
}

// EnsureServiceConfig loads or creates the managed-service credentials.
func EnsureServiceConfig() (ServiceConfig, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return ServiceConfig{}, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ServiceConfig{}, fmt.Errorf("create service config directory: %w", err)
	}
	lock := flock.New(filepath.Join(dir, ".service.lock"))
	if err := lock.Lock(); err != nil {
		return ServiceConfig{}, fmt.Errorf("lock service config: %w", err)
	}
	defer lock.Unlock()
	path := filepath.Join(dir, serviceConfigFile)
	contents, err := os.ReadFile(path)
	if err == nil {
		return decodeServiceConfig(contents)
	}
	if !os.IsNotExist(err) {
		return ServiceConfig{}, fmt.Errorf("read service config: %w", err)
	}
	password, err := generatedPassword()
	if err != nil {
		return ServiceConfig{}, err
	}
	config := ServiceConfig{Username: "wingman", Password: password}
	return config, writeServiceConfig(dir, config)
}

// ReadServiceConfig reads existing managed-service credentials.
func ReadServiceConfig() (ServiceConfig, error) {
	path, err := ServiceConfigPath()
	if err != nil {
		return ServiceConfig{}, err
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return ServiceConfig{}, fmt.Errorf("read service config: %w", err)
	}
	return decodeServiceConfig(contents)
}

func writeServiceConfig(dir string, config ServiceConfig) error {
	contents := []byte("WINGMAN_USERNAME=" + shellQuote(config.Username) + "\nWINGMAN_PASSWORD=" + shellQuote(config.Password) + "\n")
	file, err := os.CreateTemp(dir, ".service-*")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(dir, serviceConfigFile))
}

// Validate checks that service credentials are safe to use.
func (c ServiceConfig) Validate() error {
	if strings.TrimSpace(c.Username) == "" || strings.ContainsAny(c.Username, ":\r\n") {
		return errors.New("service username must not be empty or contain colon or newlines")
	}
	if _, err := validatePassword(c.Password); err != nil {
		return fmt.Errorf("service password: %w", err)
	}
	return nil
}

func generatedPassword() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate service password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func decodeServiceConfig(contents []byte) (ServiceConfig, error) {
	var config ServiceConfig
	for _, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return ServiceConfig{}, errors.New("decode service config: invalid environment entry")
		}
		value, ok = shellUnquote(value)
		if !ok {
			return ServiceConfig{}, errors.New("decode service config: invalid environment value")
		}
		switch key {
		case "WINGMAN_USERNAME":
			config.Username = value
		case "WINGMAN_PASSWORD":
			config.Password = value
		default:
			return ServiceConfig{}, fmt.Errorf("decode service config: unknown environment entry %q", key)
		}
	}
	if err := config.Validate(); err != nil {
		return ServiceConfig{}, err
	}
	return config, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func shellUnquote(value string) (string, bool) {
	if len(value) < 2 || value[0] != '\'' || value[len(value)-1] != '\'' {
		return "", false
	}
	return strings.ReplaceAll(value[1:len(value)-1], "'\\''", "'"), true
}
