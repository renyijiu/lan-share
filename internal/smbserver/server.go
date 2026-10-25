// Package smbserver owns the read-only SMB service, independently of the UI.
package smbserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/go-filesystems/smb"
)

const ShareName = "Media"

type Config struct {
	Folder, Username, Password string
	// Address is set by the application, never taken from remote input.
	Address string
}

func (cfg Config) Validate() error {
	if strings.TrimSpace(cfg.Folder) == "" {
		return errors.New("请选择共享文件夹")
	}
	i, err := os.Stat(cfg.Folder)
	if err != nil || !i.IsDir() {
		return errors.New("共享文件夹不存在或无法访问，请重新选择")
	}
	if cfg.Username == "" || len(cfg.Username) > 128 || strings.TrimSpace(cfg.Username) != cfg.Username || strings.ContainsAny(cfg.Username, "/\\:;|=,+*?<>\"[]") || strings.ContainsFunc(cfg.Username, unicode.IsControl) {
		return errors.New("请输入有效的用户名，不含路径符号或首尾空格")
	}
	if cfg.Password == "" || len(cfg.Password) > 256 || strings.ContainsRune(cfg.Password, '\x00') {
		return errors.New("请输入密码（最多 256 字节）")
	}
	return nil
}

type State string

const (
	Stopped  State = "stopped"
	Starting State = "starting"
	Running  State = "running"
	Stopping State = "stopping"
	Failed   State = "failed"
)

type Snapshot struct {
	State       State
	Address     string
	Error       string
	Connections int64
	StartedAt   time.Time
}

type ListenFunc func(context.Context, string) (net.Listener, error)

type Controller struct {
	op      sync.Mutex
	mu      sync.Mutex
	current Snapshot
	run     *session
	cancel  context.CancelFunc
	listen  ListenFunc
	log     *eventLog
	Changed chan struct{}
}

type session struct {
	listener *traceListener
	done     chan struct{}
}

func New(w io.Writer, listen ListenFunc) *Controller {
	if w == nil {
		w = io.Discard
	}
	if listen == nil {
		listen = func(ctx context.Context, address string) (net.Listener, error) {
			return (&net.ListenConfig{}).Listen(ctx, "tcp4", address)
		}
	}
	return &Controller{current: Snapshot{State: Stopped}, listen: listen, log: &eventLog{w: w}, Changed: make(chan struct{}, 1)}
}

func (c *Controller) notifyLocked() {
	select {
	case c.Changed <- struct{}{}:
	default:
	}
}

func (c *Controller) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.current
	if c.run != nil {
		s.Connections = c.run.listener.active.Load()
	}
	return s
}

// Start blocks during port authorization only. Call it outside the UI thread.
// ctx is the lifetime of this service run; cancellation also closes live sessions.
func (c *Controller) Start(parent context.Context, cfg Config) (err error) {
	c.op.Lock()
	defer c.op.Unlock()
	c.mu.Lock()
	if c.current.State == Starting || c.current.State == Running || c.current.State == Stopping {
		c.mu.Unlock()
		return errors.New("服务正在启动或运行，请先停止")
	}
	ctx, cancel := context.WithCancel(parent)
	c.cancel = cancel
	c.current = Snapshot{State: Starting}
	c.notifyLocked()
	c.mu.Unlock()
	defer func() {
		if err != nil {
			cancel()
			c.mu.Lock()
			c.cancel = nil
			c.current = Snapshot{State: Failed, Error: err.Error()}
			if errors.Is(err, context.Canceled) {
				c.current = Snapshot{State: Stopped}
			}
			c.notifyLocked()
			c.mu.Unlock()
			c.log.event("start_failed", map[string]any{"canceled": errors.Is(err, context.Canceled)})
		}
	}()
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = cfg.Validate(); err != nil {
		return err
	}
	root, err := filepath.Abs(cfg.Folder)
	if err != nil {
		return errors.New("无法解析共享文件夹")
	}
	d, err := newDisk(root, c.log)
	if err != nil {
		return errors.New("无法打开共享文件夹，请检查访问权限")
	}
	// Opening a directory can succeed when listing it is not permitted.
	probe, err := d.root.Open(".")
	if err == nil {
		_, err = probe.ReadDir(1)
		probe.Close()
		if errors.Is(err, io.EOF) {
			err = nil
		}
	}
	if err != nil {
		d.Close()
		return errors.New("无法读取共享文件夹，请检查访问权限")
	}
	srv := smb.New()
	srv.SetName("LAN-SHARE")
	srv.AddUser(cfg.Username, cfg.Password)
	if err = srv.Share(ShareName, d, smb.ReadOnly(), smb.AllowUsers(cfg.Username)); err != nil {
		d.Close()
		return errors.New("无法创建只读共享")
	}
	address := cfg.Address
	if address == "" {
		address = "0.0.0.0:445"
	}
	ln, err := c.listen(ctx, address)
	if err != nil {
		srv.Close()
		d.Close()
		return err
	}
	if err = ctx.Err(); err != nil {
		ln.Close()
		srv.Close()
		d.Close()
		return err
	}
	tl := &traceListener{Listener: ln, log: c.log, slots: make(chan struct{}, 32), onChange: func() { c.mu.Lock(); c.notifyLocked(); c.mu.Unlock() }}
	r := &session{listener: tl, done: make(chan struct{})}
	c.mu.Lock()
	c.run = r
	c.current = Snapshot{State: Running, Address: ln.Addr().String(), StartedAt: time.Now()}
	c.notifyLocked()
	c.mu.Unlock()
	c.log.event("started", map[string]any{"address": ln.Addr().String(), "share": ShareName, "read_only": true})
	go func() {
		served := make(chan error, 1)
		go func() { served <- srv.Serve(tl) }()
		var serveErr error
		select {
		case <-ctx.Done():
			ln.Close()
			serveErr = <-served
		case serveErr = <-served:
			ln.Close()
		}
		// Wait for Accept to finish before Server.Close: v0.7.0 registers
		// accepted connections after Accept, outside its close lock.
		srv.Close()
		tl.finish()
		d.Close()
		canceled := ctx.Err() != nil
		cancel()
		c.log.event("stopped", nil)
		c.mu.Lock()
		c.run = nil
		c.cancel = nil
		c.current = Snapshot{State: Stopped}
		if !canceled && serveErr != nil && !errors.Is(serveErr, net.ErrClosed) {
			c.current = Snapshot{State: Failed, Error: "SMB 服务意外停止，请重新启动"}
		}
		c.notifyLocked()
		c.mu.Unlock()
		close(r.done)
	}()
	return nil
}

// Stop cancels a pending authorization or stops the listener and every session.
func (c *Controller) Stop() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
		c.current.State = Stopping
		c.notifyLocked()
	}
	c.mu.Unlock()
	c.op.Lock()
	defer c.op.Unlock()
	c.mu.Lock()
	r := c.run
	c.mu.Unlock()
	if r != nil {
		<-r.done
	}
}

type eventLog struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *eventLog) event(kind string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["event"] = kind
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = json.NewEncoder(l.w).Encode(fields)
}

type traceListener struct {
	net.Listener
	log      *eventLog
	allowed  net.IP
	slots    chan struct{}
	next     atomic.Uint64
	active   atomic.Int64
	wg       sync.WaitGroup
	onChange func()
}

func (l *traceListener) changed() {
	if l.onChange != nil {
		l.onChange()
	}
}
func (l *traceListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		if l.allowed != nil && !l.allowed.Equal(net.ParseIP(host)) {
			conn.Close()
			continue
		}
		select {
		case l.slots <- struct{}{}:
		default:
			conn.Close()
			continue
		}
		id := l.next.Add(1)
		l.wg.Add(1)
		l.active.Add(1)
		t := newTraceConn(conn, id, l.log)
		t.release = func() { <-l.slots; l.active.Add(-1); l.changed(); l.wg.Done() }
		l.log.event("connected", map[string]any{"conn": id, "peer": conn.RemoteAddr().String()})
		l.changed()
		return t, nil
	}
}
func (l *traceListener) finish() { l.wg.Wait() }

// Addresses reports IPv4 interfaces the projector can be configured to use.
func Addresses(port string) []string {
	var result []string
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 || virtualInterface(iface.Name) {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil || ip.To4() == nil || !ip.IsPrivate() {
				continue
			}
			host := ip.String()
			if port != "445" {
				host = net.JoinHostPort(host, port)
			}
			result = append(result, fmt.Sprintf("smb://%s/%s", host, ShareName))
		}
	}
	slices.Sort(result)
	result = slices.Compact(result)
	return result
}

func virtualInterface(name string) bool {
	name = strings.ToLower(name)
	for _, prefix := range []string{"utun", "tun", "tap", "tailscale", "docker", "veth", "virbr", "vmnet", "vboxnet", "bridge", "vethernet"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
