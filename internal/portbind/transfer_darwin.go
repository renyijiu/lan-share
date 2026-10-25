package portbind

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func peerUID(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var inner error
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		inner = e
		if e == nil {
			uid = cred.Uid
		}
	})
	if err != nil {
		return 0, err
	}
	return uid, inner
}

// RunHelper is deliberately limited to one socket handoff, with no filesystem,
// password, command execution, or persistent installation functionality.
func RunHelper(socket string, uid int) error {
	if os.Geteuid() != 0 || uid <= 0 {
		return errors.New("helper requires a non-root recipient and administrator authorization")
	}
	parent := filepath.Dir(socket)
	if filepath.Dir(parent) != "/private/tmp" || !strings.HasPrefix(filepath.Base(parent), "lan-share-bind-") || filepath.Base(socket) != "control.sock" {
		return errors.New("invalid control path")
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode().Perm() != 0700 || int(stat.Uid) != uid {
		return errors.New("invalid control directory")
	}
	info, err = os.Lstat(socket)
	if err != nil {
		return err
	}
	stat, ok = info.Sys().(*syscall.Stat_t)
	if !ok || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0600 || int(stat.Uid) != uid {
		return errors.New("invalid control socket")
	}
	c, err := net.DialTimeout("unix", socket, 3*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	conn := c.(*net.UnixConn)
	peer, err := peerUID(conn)
	if err != nil || peer != uint32(uid) {
		return errors.New("recipient identity mismatch")
	}
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4zero, Port: 445})
	if err != nil {
		code := "F"
		if errors.Is(err, syscall.EADDRINUSE) {
			code = "B"
		}
		_, _ = conn.Write([]byte(code))
		return nil
	}
	defer ln.Close()
	return sendListener(conn, ln)
}

func sendListener(conn *net.UnixConn, ln *net.TCPListener) error {
	f, err := ln.File()
	if err != nil {
		return err
	}
	defer f.Close()
	_, _, err = conn.WriteMsgUnix([]byte("O"), unix.UnixRights(int(f.Fd())), nil)
	return err
}

func receiveListener(conn *net.UnixConn, expectedUID uint32, port int) (net.Listener, error) {
	peer, err := peerUID(conn)
	if err != nil || peer != expectedUID {
		return nil, errors.New("端口授权助手身份验证失败")
	}
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	data := make([]byte, 128)
	oob := make([]byte, unix.CmsgSpace(4)*4)
	n, oobn, flags, _, err := conn.ReadMsgUnix(data, oob)
	if err != nil {
		return nil, errors.New("无法接收已授权的监听端口")
	}
	messages, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return nil, err
	}
	var fds []int
	for _, m := range messages {
		rights, e := unix.ParseUnixRights(&m)
		if e != nil {
			for _, fd := range fds {
				unix.Close(fd)
			}
			return nil, e
		}
		fds = append(fds, rights...)
	}
	defer func() {
		for _, fd := range fds {
			unix.Close(fd)
		}
	}()
	if flags&unix.MSG_CTRUNC != 0 {
		return nil, errors.New("端口授权消息不完整")
	}
	if string(data[:n]) != "O" || len(fds) != 1 {
		if string(data[:n]) == "B" {
			return nil, explain(syscall.EADDRINUSE)
		}
		return nil, errors.New("端口授权失败，请检查 445 端口占用和系统权限")
	}
	unix.CloseOnExec(fds[0])
	f := os.NewFile(uintptr(fds[0]), "authorized-smb-listener")
	fds = nil // f owns this descriptor; net.FileListener creates its own copy.
	defer f.Close()
	ln, err := net.FileListener(f)
	if err != nil {
		return nil, err
	}
	a, ok := ln.Addr().(*net.TCPAddr)
	if !ok || a.Port != port || !a.IP.IsUnspecified() {
		ln.Close()
		return nil, errors.New("授权返回了意外的监听端口")
	}
	return ln, nil
}
