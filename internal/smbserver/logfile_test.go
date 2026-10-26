package smbserver

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLogRotationKeepsThreeFiles(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	line := append(bytes.Repeat([]byte("x"), 1023), '\n')
	for i := 0; i < 4*1024; i++ {
		if _, err := l.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"service.jsonl", "service.1.jsonl", "service.2.jsonl"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Size() > 1<<20 {
			t.Fatalf("%s: %v %v", name, info, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "service.3.jsonl")); !os.IsNotExist(err) {
		t.Fatal("more than three log files kept")
	}
}

func TestLogKeepsWritingWhenRotationFails(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	line := append(bytes.Repeat([]byte("x"), 1023), '\n')
	write := func(n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			if _, err := l.Write(line); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(1025) // service.jsonl becomes service.1.jsonl once
	// A non-empty directory cannot be removed or replaced by a rename.
	blocked := filepath.Join(dir, "service.2.jsonl")
	if err := os.MkdirAll(filepath.Join(blocked, "keep"), 0700); err != nil {
		t.Fatal(err)
	}
	write(1025) // the next rotation fails
	before, err := os.Stat(filepath.Join(dir, "service.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	write(1)
	after, err := os.Stat(filepath.Join(dir, "service.jsonl"))
	if err != nil || after.Size() <= before.Size() {
		t.Fatalf("logging stopped after a failed rotation: %v %v", after, err)
	}
}
