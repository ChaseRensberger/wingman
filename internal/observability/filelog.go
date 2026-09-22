package observability

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const logMaxBytes int64 = 50 << 20
const logKeepBytes int64 = 25 << 20

// FileLog keeps a bounded, append-only process log across daemon restarts.
type FileLog struct {
	mu        sync.Mutex
	file      *os.File
	max, keep int64
}

// OpenFileLog opens a private log file in the managed daemon state directory.
func OpenFileLog(stateDir string) (*FileLog, error) {
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(stateDir, "wingman.log"), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &FileLog{file: file, max: logMaxBytes, keep: logKeepBytes}, nil
}

// Write appends a log line and discards the oldest complete lines above the size limit.
func (l *FileLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	original := len(p)
	if int64(len(p)) > l.keep {
		p = []byte("[log entry exceeded limit]\n")
	}
	info, err := l.file.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size()+int64(len(p)) > l.max {
		start := info.Size() - l.keep
		if start < 0 {
			start = 0
		}
		if _, err := l.file.Seek(start, io.SeekStart); err != nil {
			return 0, err
		}
		remaining, err := io.ReadAll(l.file)
		if err != nil {
			return 0, err
		}
		if start > 0 {
			if end := bytes.IndexByte(remaining, '\n'); end >= 0 {
				remaining = remaining[end+1:]
			} else {
				remaining = nil
			}
		}
		if _, err := l.file.WriteAt(remaining, 0); err != nil {
			return 0, err
		}
		if err := l.file.Truncate(int64(len(remaining))); err != nil {
			return 0, err
		}
	}
	if _, err := l.file.Seek(0, io.SeekEnd); err != nil {
		return 0, err
	}
	n, err := l.file.Write(p)
	if err != nil {
		return n, err
	}
	return original, nil
}

// Close closes the daemon's log file.
func (l *FileLog) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
