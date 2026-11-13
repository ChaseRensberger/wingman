package pluginhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// State stores plugin-owned values and explicit project-trust decisions.
type State struct{ Root string }

// DefaultState returns the effective user's plugin state directory.
func DefaultState() (State, error) {
	home, err := os.UserHomeDir()
	return State{Root: filepath.Join(home, ".local", "share", "wingman", "plugins")}, err
}

func (s State) update(ctx context.Context, name string, fn func(map[string]json.RawMessage) error) error {
	if s.Root == "" {
		return errors.New("plugin state directory is required")
	}
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.Root, name+".lock"))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return err
	}
	if !locked {
		return ctx.Err()
	}
	defer lock.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	values := map[string]json.RawMessage{}
	data, err := os.ReadFile(filepath.Join(s.Root, name+".json"))
	if err == nil {
		if err := json.Unmarshal(data, &values); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := fn(values); err != nil {
		return err
	}
	data, err = json.Marshal(values)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Root, ".state-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(s.Root, name+".json"))
}

// Trust records an explicit decision for one canonical project directory.
func (s State) Trust(ctx context.Context, directory string, trusted bool) error {
	canonical, err := canonicalDirectory(directory)
	if err != nil {
		return err
	}
	return s.update(ctx, "trust", func(values map[string]json.RawMessage) error {
		values[canonical], _ = json.Marshal(trusted)
		return nil
	})
}

// Trusted reports whether a directory has an explicit positive trust decision.
func (s State) Trusted(directory string) (bool, error) {
	canonical, err := canonicalDirectory(directory)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "trust.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var values map[string]bool
	if err := json.Unmarshal(data, &values); err != nil {
		return false, err
	}
	return values[canonical], nil
}

func canonicalDirectory(directory string) (string, error) {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a project directory: %s", directory)
	}
	return canonical, nil
}

// Storage executes a durable operation in a plugin, client, and directory scope.
func (s State) Storage(ctx context.Context, pluginID, method string, raw json.RawMessage) (any, error) {
	return s.storage(ctx, "values", pluginID, method, raw)
}

// Credential executes a plugin-owned credential operation in the client scope.
func (s State) Credential(ctx context.Context, pluginID, method string, raw json.RawMessage) (any, error) {
	identity := Identity(ctx)
	if identity.ClientID == "" {
		return nil, errors.New("credentials require a client-owned invocation")
	}
	identity.WorkDir = ""
	ctx = WithIdentity(ctx, identity)
	return s.storage(ctx, "credentials", pluginID, "storage."+strings.TrimPrefix(method, "credential."), raw)
}

func (s State) storage(ctx context.Context, domain, pluginID, method string, raw json.RawMessage) (any, error) {
	var input struct {
		Key    string          `json:"key"`
		Value  json.RawMessage `json:"value"`
		Prefix string          `json:"prefix"`
		After  string          `json:"after"`
		Limit  int             `json:"limit"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if len(input.Key) > 1024 || len(input.Value) > 1<<20 {
		return nil, errors.New("plugin storage value exceeds its limit")
	}
	identity := Identity(ctx)
	scope, _ := json.Marshal([4]string{domain, pluginID, identity.ClientID, identity.WorkDir})
	sum := sha256.Sum256(scope)
	name := "storage-" + hex.EncodeToString(sum[:])
	var output any
	err := s.update(ctx, name, func(values map[string]json.RawMessage) error {
		switch method {
		case "storage.get":
			output = values[input.Key]
		case "storage.set":
			if !json.Valid(input.Value) {
				return errors.New("storage value must be JSON")
			}
			values[input.Key] = input.Value
		case "storage.remove":
			delete(values, input.Key)
		case "storage.scan":
			keys := make([]string, 0, len(values))
			for key := range values {
				if strings.HasPrefix(key, input.Prefix) && key > input.After {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			limit := input.Limit
			if limit <= 0 || limit > 100 {
				limit = 100
			}
			next := ""
			if len(keys) > limit {
				keys = keys[:limit]
				next = keys[len(keys)-1]
			}
			type entry struct {
				Key   string          `json:"key"`
				Value json.RawMessage `json:"value"`
			}
			entries := make([]entry, 0, len(keys))
			for _, key := range keys {
				entries = append(entries, entry{key, values[key]})
			}
			output = struct {
				Entries []entry `json:"entries"`
				Next    string  `json:"next,omitempty"`
			}{entries, next}
		default:
			return fmt.Errorf("unknown storage operation %q", method)
		}
		return nil
	})
	return output, err
}
