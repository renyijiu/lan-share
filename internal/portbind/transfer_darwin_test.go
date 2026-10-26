package portbind

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Exercise the exact SCM_RIGHTS handoff without prompting for administrator access.
func TestListenerHandoffAndRelease(t *testing.T) {
	dir, err := os.MkdirTemp("/private/tmp", "lan-share-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "socket")
	control, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	source, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	address := source.Addr().String()
	port := source.Addr().(*net.TCPAddr).Port
	sent := make(chan error, 1)
	go func() {
		conn, e := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
		if e != nil {
			source.Close()
			sent <- e
			return
		}
		e = sendListener(conn, source)
		source.Close()
		conn.Close()
		sent <- e
	}()
	conn, err := control.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	listener, err := receiveListener(conn, uint32(os.Getuid()), port)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = <-sent; err != nil {
		t.Fatal(err)
	}
	accepted := make(chan error, 1)
	go func() {
		c, e := listener.Accept()
		if e == nil {
			_, e = c.Write([]byte("ok"))
			c.Close()
		}
		accepted <- e
	}()
	client, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", portString(port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.SetReadDeadline(time.Now().Add(time.Second))
	b := make([]byte, 2)
	_, err = io.ReadFull(client, b)
	client.Close()
	if err != nil || string(b) != "ok" {
		t.Fatalf("handoff: %q %v", b, err)
	}
	if err = <-accepted; err != nil {
		t.Fatal(err)
	}
	listener.Close()
	again, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatalf("descriptor leaked: %v", err)
	}
	again.Close()
}

func portString(port int) string { return strconv.Itoa(port) }

func TestControlPathMustBeExact(t *testing.T) {
	for path, want := range map[string]bool{
		"/private/tmp/lan-share-bind-123/control.sock":        true,
		"/private/tmp/lan-share-bind-123/x/../control.sock":   false,
		"/private/tmp/lan-share-bind-123//control.sock":       false,
		"/private/tmp/lan-share-bind-123/./control.sock":      false,
		"/private/tmp/../tmp/lan-share-bind-123/control.sock": false,
		"/private/tmp/other-123/control.sock":                 false,
		"/private/tmp/lan-share-bind-123/other.sock":          false,
		"/tmp/lan-share-bind-123/control.sock":                false,
		"private/tmp/lan-share-bind-123/control.sock":         false,
	} {
		if got := validControlPath(path); got != want {
			t.Errorf("validControlPath(%q) = %v, want %v", path, got, want)
		}
	}
}
