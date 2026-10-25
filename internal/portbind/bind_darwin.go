package portbind

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// The script is constant. All arguments are shell-quoted by AppleScript itself.
const authorizeScript = `on run argv
set helperPath to item 1 of argv
set socketPath to item 2 of argv
set userID to item 3 of argv
do shell script (quoted form of helperPath & " --socket " & quoted form of socketPath & " --uid " & quoted form of userID) with administrator privileges
end run`

func privilegedListen(parent context.Context, address string) (net.Listener, error) {
	if address != "0.0.0.0:445" {
		return nil, errors.New("端口授权助手仅允许监听 IPv4 的 445 端口")
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	// Production packages place the minimal Go helper beside the main executable.
	helper := filepath.Join(filepath.Dir(exe), "lan-share-port-helper")
	if info, e := os.Stat(helper); e != nil || !info.Mode().IsRegular() {
		return nil, errors.New("缺少端口授权助手，请使用完整的 LAN Share.app")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()
	dir, err := os.MkdirTemp("/private/tmp", "lan-share-bind-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "control.sock")
	control, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	defer control.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { control.Close() })
	defer stop()
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", authorizeScript, helper, path, strconv.Itoa(os.Getuid()))
	if err = cmd.Start(); err != nil {
		return nil, errors.New("无法打开 macOS 授权窗口")
	}
	commandDone := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		commandDone <- err
		if err != nil {
			control.Close()
		}
	}()
	conn, acceptErr := control.AcceptUnix()
	if acceptErr != nil {
		commandErr := <-commandDone
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		if ctx.Err() != nil {
			return nil, errors.New("管理员授权等待超时，请重试")
		}
		return nil, fmt.Errorf("管理员授权已取消或未完成，请重试（%v）", commandErr)
	}
	defer conn.Close()
	stopConn := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopConn()
	ln, recvErr := receiveListener(conn, 0, 445)
	commandErr := <-commandDone // helper has closed its own copies before we report Running.
	if ctx.Err() != nil {
		if ln != nil {
			ln.Close()
		}
		return nil, ctx.Err()
	}
	if recvErr != nil {
		return nil, recvErr
	}
	if commandErr != nil {
		ln.Close()
		return nil, errors.New("端口授权助手退出异常，请重试")
	}
	return ln, nil
}
