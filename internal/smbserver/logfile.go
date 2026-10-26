package smbserver

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// LogFile bounds diagnostic storage to three files of approximately 1 MiB.
type LogFile struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	size int64
}

func OpenLog(dir string) (*LogFile, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	l := &LogFile{dir: dir}
	return l, l.open()
}
func (l *LogFile) open() error {
	f, err := os.OpenFile(filepath.Join(l.dir, "service.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	l.file = f
	if info, e := f.Stat(); e == nil {
		l.size = info.Size()
	}
	return nil
}
func (l *LogFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return 0, os.ErrClosed
	}
	if l.size+int64(len(p)) > 1<<20 {
		l.file.Close()
		l.file = nil
		rotated := l.rotate()
		if err := l.open(); err != nil {
			return 0, err
		}
		if rotated != nil {
			// Keep appending to the current file and retry after another MiB,
			// rather than losing every later diagnostic.
			l.size = 0
		}
	}
	n, err := l.file.Write(p)
	l.size += int64(n)
	return n, err
}
func (l *LogFile) rotate() error {
	_ = os.Remove(filepath.Join(l.dir, "service.2.jsonl"))
	for i := 1; i >= 0; i-- {
		src := "service.jsonl"
		if i > 0 {
			src = fmt.Sprintf("service.%d.jsonl", i)
		}
		if err := os.Rename(filepath.Join(l.dir, src), filepath.Join(l.dir, fmt.Sprintf("service.%d.jsonl", i+1))); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
func (l *LogFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}
