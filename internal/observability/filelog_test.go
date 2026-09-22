package observability

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileLogRetainsRecentCompleteLinesAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	log, err := OpenFileLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.max, log.keep = 40, 15
	for _, line := range []string{"entry-one\n", "entry-two\n", "entry-three\n", "entry-four\n", "entry-five\n"} {
		if n, err := log.Write([]byte(line)); err != nil || n != len(line) {
			t.Fatalf("write = %d, %v", n, err)
		}
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	log, err = OpenFileLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.max, log.keep = 40, 15
	defer log.Close()
	if _, err := log.Write([]byte("entry-six\n")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "wingman.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "entry-one") || !strings.Contains(string(data), "entry-six\n") || !strings.HasPrefix(string(data), "entry-") || len(data) > 40 {
		t.Fatalf("trimmed log = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("log permissions = %v, %v", info, err)
	}
}

func TestFileLogBoundsOversizedEntry(t *testing.T) {
	log, err := OpenFileLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	log.max, log.keep = 60, 20
	line := "oversized\n" + strings.Repeat("x", 75) + "\n"
	if n, err := log.Write([]byte(line)); err != nil || n != len(line) {
		t.Fatalf("write = %d, %v", n, err)
	}
	data, err := os.ReadFile(log.file.Name())
	if err != nil || len(data) > 60 || !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("bounded log = %q, %v", data, err)
	}
}

func TestFileLogAndLiveBufferReceiveSameManagedEntry(t *testing.T) {
	log, err := OpenFileLog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	buffer := NewLogBuffer(2)
	logger, err := NewBufferedLogger(io.MultiWriter(io.Discard, log), "json", "info", buffer)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("model call finished", "model_call_id", "mcl_test")
	entries := buffer.Entries()
	data, err := os.ReadFile(log.file.Name())
	if err != nil || len(entries) != 1 || entries[0].Msg != "model call finished" || !strings.Contains(string(data), "mcl_test") || !strings.Contains(entries[0].Raw, "mcl_test") {
		t.Fatalf("buffer = %#v, file = %s, err = %v", entries, data, err)
	}
}
