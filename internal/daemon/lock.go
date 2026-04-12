package daemon

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
)

type lockFile struct {
	file *os.File
	path string
}

func acquireLock(path string) (*lockFile, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	if err := file.Truncate(0); err != nil {
		_ = file.Close()
		return nil, err
	}
	if _, err := file.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &lockFile{file: file, path: path}, nil
}

func (l *lockFile) close() error {
	if l == nil || l.file == nil {
		return nil
	}
	var firstErr error
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := l.file.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if l.path != "" {
		if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
