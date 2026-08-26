package daemonstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"
)

func testRegistration() Registration {
	return Registration{InstanceID: "instance", Version: "1.0.0", URL: "https://127.0.0.1:2424", PID: 1, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}

func TestRegistrationValidation(t *testing.T) {
	valid := testRegistration()
	tests := []struct {
		name string
		edit func(*Registration)
	}{
		{name: "empty instance ID", edit: func(r *Registration) { r.InstanceID = " " }},
		{name: "empty version", edit: func(r *Registration) { r.Version = "" }},
		{name: "relative URL", edit: func(r *Registration) { r.URL = "/daemon" }},
		{name: "unsupported URL scheme", edit: func(r *Registration) { r.URL = "ftp://example.test" }},
		{name: "URL without host", edit: func(r *Registration) { r.URL = "https:/daemon" }},
		{name: "zero PID", edit: func(r *Registration) { r.PID = 0 }},
		{name: "invalid creation time", edit: func(r *Registration) { r.CreatedAt = "today" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registration := valid
			test.edit(&registration)
			if err := registration.Validate(); err == nil {
				t.Fatal("Validate() succeeded")
			}
		})
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.URLs = []string{"http://192.0.2.1:2424"}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistrationReadRejectsMalformedJSON(t *testing.T) {
	tests := []struct {
		name     string
		contents string
	}{
		{name: "unknown field", contents: `{"instance_id":"one","version":"1","url":"http://localhost","pid":1,"created_at":"2026-07-31T00:00:00Z","extra":true}`},
		{name: "trailing value", contents: `{"instance_id":"one","version":"1","url":"http://localhost","pid":1,"created_at":"2026-07-31T00:00:00Z"} {}`},
		{name: "invalid registration", contents: `{"instance_id":"one","version":"","url":"http://localhost","pid":1,"created_at":"2026-07-31T00:00:00Z"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := New(t.TempDir())
			if err := os.WriteFile(filepath.Join(state.Dir(), registrationFile), []byte(test.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := state.ReadRegistration(); err == nil {
				t.Fatal("ReadRegistration() succeeded")
			}
		})
	}
}

func TestRegistrationPersistsAdvertisedURLs(t *testing.T) {
	state := New(t.TempDir())
	registration := testRegistration()
	registration.URLs = []string{"http://192.0.2.1:2424", "http://198.51.100.1:2424"}
	if err := state.WriteRegistration(registration); err != nil {
		t.Fatal(err)
	}
	got, err := state.ReadRegistration()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, registration) {
		t.Fatalf("ReadRegistration() = %#v, want %#v", got, registration)
	}
}

func TestRegistrationOwnershipCleanup(t *testing.T) {
	state := New(t.TempDir())
	registration := testRegistration()
	registration.InstanceID = "one"
	registration.URL = "http://localhost:2424"
	registration.PID = 100
	if err := state.WriteRegistration(registration); err != nil {
		t.Fatal(err)
	}
	removed, err := state.RemoveRegistration("other")
	if err != nil || removed {
		t.Fatalf("RemoveRegistration(other) = %v, %v", removed, err)
	}
	if got, err := state.ReadRegistration(); err != nil || !reflect.DeepEqual(got, registration) {
		t.Fatalf("ReadRegistration() = %#v, %v", got, err)
	}
	removed, err = state.RemoveRegistration("one")
	if err != nil || !removed {
		t.Fatalf("RemoveRegistration(one) = %v, %v", removed, err)
	}
	if _, err := state.ReadRegistration(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadRegistration() error = %v, want not exist", err)
	}
}

func TestServiceConfigIsStableAndPrivate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	first, err := EnsureServiceConfig()
	if err != nil {
		t.Fatal(err)
	}
	second, err := EnsureServiceConfig()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("EnsureServiceConfig() = %#v then %#v", first, second)
	}
	read, err := ReadServiceConfig()
	if err != nil || read != first {
		t.Fatalf("ReadServiceConfig() = %#v, %v", read, err)
	}
	dir, _ := DefaultConfigDir()
	assertPrivatePermissions(t, dir, serviceConfigFile)
}

func TestConcurrentServiceConfigCreation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	configs := make(chan ServiceConfig, 16)
	errs := make(chan error, 16)
	var group sync.WaitGroup
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			config, err := EnsureServiceConfig()
			if err != nil {
				errs <- err
				return
			}
			configs <- config
		}()
	}
	group.Wait()
	close(configs)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var first ServiceConfig
	for config := range configs {
		if first == (ServiceConfig{}) {
			first = config
		} else if config != first {
			t.Fatalf("EnsureServiceConfig() values differ: %#v and %#v", first, config)
		}
	}
}

func TestManagedDaemonLock(t *testing.T) {
	state := New(t.TempDir())
	first, err := state.AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AcquireLock(); !errors.Is(err, ErrLocked) {
		t.Fatalf("AcquireLock() error = %v, want ErrLocked", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := state.AcquireLock()
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}

func assertPrivatePermissions(t *testing.T, dir, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}
	info, err = os.Stat(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("file permissions = %o, want 600", got)
	}
}
