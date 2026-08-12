// Package daemonstate persists local state used to discover and manage a Wingman daemon.
package daemonstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

const (
	registrationFile = "registration.json"
	registrationLock = "registration.lock"
	daemonLock       = "daemon.lock"
)

// ErrLocked reports that another daemon currently owns the managed-daemon lock.
var ErrLocked = errors.New("daemon state lock is held")

// Registration identifies a running daemon instance.
type Registration struct {
	InstanceID string   `json:"instance_id"`
	Version    string   `json:"version"`
	URL        string   `json:"url"`
	URLs       []string `json:"urls,omitempty"`
	PID        int      `json:"pid"`
	CreatedAt  string   `json:"created_at"`
}

// Validate checks that a registration is safe to persist and use.
func (r Registration) Validate() error {
	if strings.TrimSpace(r.InstanceID) == "" {
		return errors.New("instance_id must not be empty")
	}
	if strings.TrimSpace(r.Version) == "" {
		return errors.New("version must not be empty")
	}
	parsed, err := url.Parse(r.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || !parsed.IsAbs() {
		return fmt.Errorf("url must be an absolute HTTP or HTTPS URL: %q", r.URL)
	}
	for _, raw := range r.URLs {
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || !parsed.IsAbs() {
			return fmt.Errorf("advertised URL must be an absolute HTTP or HTTPS URL: %q", raw)
		}
	}
	if r.PID <= 0 {
		return errors.New("pid must be greater than zero")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err != nil {
		return errors.New("created_at must be an RFC 3339 timestamp")
	}
	return nil
}

// State manages files rooted at dir. The directory is created on first use.
type State struct {
	dir string
}

// New returns state rooted at dir.
func New(dir string) *State {
	return &State{dir: dir}
}

// DefaultDir returns Wingman's XDG state directory.
func DefaultDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "wingman"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", "wingman"), nil
}

// Dir returns the state root directory.
func (s *State) Dir() string {
	return s.dir
}

// WriteRegistration atomically writes a validated daemon registration.
func (s *State) WriteRegistration(registration Registration) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	contents, err := json.Marshal(registration)
	if err != nil {
		return fmt.Errorf("encode registration: %w", err)
	}
	err = s.withLock(registrationLock, func() error {
		return s.atomicWrite(registrationFile, contents)
	})
	return err
}

// ReadRegistration reads and strictly validates the current daemon registration.
func (s *State) ReadRegistration() (Registration, error) {
	if err := s.ensureDir(); err != nil {
		return Registration{}, err
	}
	contents, err := os.ReadFile(s.path(registrationFile))
	if err != nil {
		return Registration{}, fmt.Errorf("read registration: %w", err)
	}
	registration, err := decodeRegistration(contents)
	if err != nil {
		return Registration{}, fmt.Errorf("decode registration: %w", err)
	}
	return registration, nil
}

// RemoveRegistration removes the registration only when instanceID still owns it.
// It reports whether a registration was removed.
func (s *State) RemoveRegistration(instanceID string) (bool, error) {
	if strings.TrimSpace(instanceID) == "" {
		return false, errors.New("instance_id must not be empty")
	}
	var removed bool
	err := s.withLock(registrationLock, func() error {
		contents, err := os.ReadFile(s.path(registrationFile))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read registration: %w", err)
		}
		registration, err := decodeRegistration(contents)
		if err != nil {
			return fmt.Errorf("decode registration: %w", err)
		}
		if registration.InstanceID != instanceID {
			return nil
		}
		if err := os.Remove(s.path(registrationFile)); err != nil {
			return fmt.Errorf("remove registration: %w", err)
		}
		removed = true
		return nil
	})
	return removed, err
}

// AcquireLock tries to acquire the managed-daemon lock without blocking.
func (s *State) AcquireLock() (*Lock, error) {
	if err := s.ensureDir(); err != nil {
		return nil, err
	}
	path := s.path(daemonLock)
	if err := ensurePrivateFile(path); err != nil {
		return nil, err
	}
	file := flock.New(path)
	locked, err := file.TryLock()
	if err != nil {
		return nil, fmt.Errorf("acquire daemon lock: %w", err)
	}
	if !locked {
		return nil, ErrLocked
	}
	return &Lock{file: file}, nil
}

// Lock holds the managed-daemon election lock until released.
type Lock struct {
	mu       sync.Mutex
	file     *flock.Flock
	released bool
}

// Release relinquishes the managed-daemon lock.
func (l *Lock) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	if err := l.file.Unlock(); err != nil {
		return fmt.Errorf("release daemon lock: %w", err)
	}
	l.released = true
	return nil
}

func (s *State) withLock(name string, fn func() error) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	path := s.path(name)
	if err := ensurePrivateFile(path); err != nil {
		return err
	}
	file := flock.New(path)
	if err := file.Lock(); err != nil {
		return fmt.Errorf("lock %s: %w", name, err)
	}
	defer file.Unlock()
	return fn()
}

func (s *State) ensureDir() error {
	if strings.TrimSpace(s.dir) == "" {
		return errors.New("state directory must not be empty")
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.Chmod(s.dir, 0o700); err != nil {
		return fmt.Errorf("set state directory permissions: %w", err)
	}
	return nil
}

func (s *State) path(name string) string {
	return filepath.Join(s.dir, name)
}

func (s *State) atomicWrite(name string, contents []byte) error {
	file, err := os.CreateTemp(s.dir, "."+name+"-*")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(contents); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(path, s.path(name))
}

func ensurePrivateFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func validatePassword(password string) (string, error) {
	if password == "" || strings.TrimSpace(password) != password {
		return "", errors.New("password must not be empty or contain surrounding whitespace")
	}
	return password, nil
}

func decodeRegistration(contents []byte) (Registration, error) {
	var registration Registration
	decoder := json.NewDecoder(strings.NewReader(string(contents)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&registration); err != nil {
		return Registration{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Registration{}, errors.New("trailing JSON value")
		}
		return Registration{}, fmt.Errorf("trailing JSON value: %w", err)
	}
	if err := registration.Validate(); err != nil {
		return Registration{}, err
	}
	return registration, nil
}
