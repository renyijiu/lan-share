package smbserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	smbclient "github.com/cloudsoda/go-smb2"
	"github.com/go-filesystems/smb"
)

func TestReadOnlyShareEndToEnd(t *testing.T) {
	root := t.TempDir()
	content := bytes.Repeat([]byte("SMB probe integration bytes\n"), 100000)
	if err := os.WriteFile(filepath.Join(root, "中文测试.bin"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "子目录"), 0700); err != nil {
		t.Fatal(err)
	}
	// A sparse file proves offsets beyond 4 GiB work without allocating 4 GiB.
	big, err := os.Create(filepath.Join(root, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	const far = int64(4<<30) + 123
	if _, err = big.WriteAt([]byte("far-end-marker"), far); err != nil {
		t.Fatal(err)
	}
	big.Close()
	var logs bytes.Buffer
	l := &eventLog{w: &logs}
	d, err := newDisk(root, l)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	srv := smb.New()
	srv.AddUser("projector", "test-only-pass")
	if err = srv.Share("Media", d, smb.ReadOnly()); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	tl := &traceListener{Listener: ln, log: l, slots: make(chan struct{}, 8)}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(tl) }()
	stop := func() { ln.Close(); <-done; srv.Close(); tl.finish() }
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dialer := &smbclient.Dialer{Initiator: &smbclient.NTLMInitiator{User: "projector", Password: "wrong"}}
	if s, err := dialer.Dial(ctx, ln.Addr().String()); err == nil {
		s.Logoff()
		t.Fatal("wrong password accepted")
	}
	dialer.Initiator = &smbclient.NTLMInitiator{User: "projector", Password: "test-only-pass"}
	s, err := dialer.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	names, err := s.ListSharenames()
	if err != nil || !slices.Contains(names, "Media") {
		t.Fatalf("share enumeration: %v %v", names, err)
	}
	share, err := s.Mount("Media")
	if err != nil {
		t.Fatal(err)
	}
	entries, err := share.ReadDir(".")
	if err != nil || len(entries) != 3 {
		t.Fatalf("directory listing: %d %v", len(entries), err)
	}
	got, err := share.ReadFile("中文测试.bin")
	if err != nil || !bytes.Equal(got, content) {
		t.Fatalf("file bytes differ: %d %v", len(got), err)
	}
	f, err := share.Open("large.bin")
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, len("far-end-marker"))
	if n, err := f.ReadAt(b, far); n != len(b) || err != nil || string(b) != "far-end-marker" {
		t.Fatalf("4 GiB offset: %q %d %v", b, n, err)
	}
	f.Close()
	if err := share.WriteFile("not-allowed.txt", []byte("no"), 0600); err == nil {
		t.Fatal("write accepted")
	}
	if err := share.Remove("中文测试.bin"); err == nil {
		t.Fatal("delete accepted")
	}
	share.Umount()
	s.Logoff()
	stop()
	stop = nil
	l.mu.Lock()
	text := logs.String()
	l.mu.Unlock()
	for _, event := range []string{"negotiate_offer", "authentication_failed", "authentication_ok", "share_enum_request", "file_read", "connection_summary"} {
		if !strings.Contains(text, `"event":"`+event+`"`) {
			t.Errorf("missing %s", event)
		}
	}
	for _, secret := range []string{"test-only-pass", "中文测试.bin", "SMB probe integration bytes", "far-end-marker"} {
		if strings.Contains(text, secret) {
			t.Errorf("sensitive text in logs: %q", secret)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "not-allowed.txt")); !os.IsNotExist(err) {
		t.Fatal("write created a file")
	}
	if _, err := os.Stat(filepath.Join(root, "中文测试.bin")); err != nil {
		t.Fatal("delete changed the original file")
	}
	if os.Getenv("SMB_PROBE_TEST_LOG") != "" {
		if err := os.WriteFile(os.Getenv("SMB_PROBE_TEST_LOG"), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDirectoryBoundary(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "secret"), []byte("never share"), 0600)
	d, err := newDisk(root, &eventLog{w: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, p := range []string{"../secret", "/../secret", "a/../../secret", `C:\secret`, "a\x00b"} {
		if _, err := d.OpenFile(p); err == nil {
			t.Fatalf("unsafe path accepted: %q", p)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err == nil {
		if _, err := d.OpenFile("/escape/secret"); err == nil {
			t.Fatal("outside symlink followed")
		}
		entries, err := d.ListDir("/")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() == "escape" {
				t.Fatal("outside symlink listed")
			}
		}
	}
}

func TestTraceReassemblyAndSMB1Detection(t *testing.T) {
	var log bytes.Buffer
	c := newTraceConn(nil, 1, &eventLog{w: &log})
	// Legacy NEGOTIATE offers SMB1 only. It must not be confused with the
	// legacy envelope modern clients use to offer SMB2.
	for _, modern := range []bool{false, true} {
		p := make([]byte, 35)
		copy(p, []byte{0xff, 'S', 'M', 'B'})
		p[4] = 0x72
		dialects := []byte("\x02NT LM 0.12\x00")
		if modern {
			dialects = append(dialects, []byte("\x02SMB 2.002\x00")...)
		}
		binary.LittleEndian.PutUint16(p[33:], uint16(len(dialects)))
		p = append(p, dialects...)
		frame := make([]byte, 4)
		binary.BigEndian.PutUint32(frame, uint32(len(p)))
		frame = append(frame, p...)
		for _, b := range frame {
			c.in.feed([]byte{b})
		}
		if c.smb1Only == modern {
			t.Fatalf("wrong SMB1 classification, modern=%v", modern)
		}
	}
	var calls int
	tap := frameTap{onFrame: func(b []byte, n int) {
		calls++
		if len(b) != 4096 || n != 10000 {
			t.Fatalf("prefix unbounded: %d/%d", len(b), n)
		}
	}}
	frame := make([]byte, 10004)
	binary.BigEndian.PutUint32(frame, 10000)
	tap.feed(append(frame, frame...))
	if calls != 2 {
		t.Fatal("coalesced frames not decoded")
	}
}

func FuzzFrameTap(f *testing.F) {
	f.Add([]byte{0, 0, 0, 4, 0xfe, 'S', 'M', 'B'})
	f.Fuzz(func(t *testing.T, b []byte) {
		c := newTraceConn(nil, 1, &eventLog{w: io.Discard})
		mid := len(b) / 2
		c.in.feed(b[:mid])
		c.in.feed(b[mid:])
	})
}
