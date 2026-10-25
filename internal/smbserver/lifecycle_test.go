package smbserver

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	smbclient "github.com/cloudsoda/go-smb2"
)

func TestControllerRestartReleasesPortAndRevokesOldSession(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "电影.txt"), []byte("read-only movie"), 0600); err != nil {
		t.Fatal(err)
	}
	c := New(io.Discard, nil)
	defer c.Stop()
	cfg := Config{Folder: root, Username: "projector", Password: "first-password", Address: "127.0.0.1:0"}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	address := c.Snapshot().Address
	if err := c.Start(ctx, cfg); err == nil {
		t.Fatal("duplicate start succeeded")
	}
	d := smbclient.Dialer{Initiator: &smbclient.NTLMInitiator{User: cfg.Username, Password: cfg.Password}}
	s, err := d.Dial(ctx, address)
	if err != nil {
		t.Fatal(err)
	}
	share, err := s.Mount(ShareName)
	if err != nil {
		t.Fatal(err)
	}
	f, err := share.Open("电影.txt")
	if err != nil {
		t.Fatal(err)
	}
	c.Stop()
	if c.Snapshot().State != Stopped {
		t.Fatal(c.Snapshot())
	}
	if _, err = f.ReadAt(make([]byte, 1), 0); err == nil {
		t.Fatal("old handle survived Stop")
	}
	f.Close()
	share.Umount()
	s.Logoff()
	ln, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("port not released: %v", err)
	}
	ln.Close()
	cfg.Address = address
	cfg.Password = "second-password"
	for range 3 {
		if err = c.Start(ctx, cfg); err != nil {
			t.Fatal(err)
		}
		if old, e := d.Dial(ctx, address); e == nil {
			old.Logoff()
			t.Fatal("old password accepted")
		}
		current := smbclient.Dialer{Initiator: &smbclient.NTLMInitiator{User: cfg.Username, Password: cfg.Password}}
		newSession, e := current.Dial(ctx, address)
		if e != nil {
			t.Fatal(e)
		}
		newSession.Logoff()
		c.Stop()
	}
}

func TestStopCancelsPendingAuthorization(t *testing.T) {
	entered := make(chan struct{})
	c := New(nil, func(ctx context.Context, _ string) (net.Listener, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	result := make(chan error, 1)
	go func() {
		result <- c.Start(context.Background(), Config{Folder: t.TempDir(), Username: "projector", Password: "password"})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("listen was not called")
	}
	stopped := make(chan struct{})
	go func() { c.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not cancel authorization")
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if c.Snapshot().State != Stopped {
		t.Fatal(c.Snapshot())
	}
}

func TestControllerRecoversFromOccupiedPort(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	c := New(nil, nil)
	defer c.Stop()
	cfg := Config{Folder: t.TempDir(), Username: "projector", Password: "password", Address: ln.Addr().String()}
	if err = c.Start(context.Background(), cfg); err == nil {
		t.Fatal("occupied port succeeded")
	}
	ln.Close()
	if err = c.Start(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
}
