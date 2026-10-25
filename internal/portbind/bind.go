// Package portbind contains the platform boundary for obtaining a TCP listener.
package portbind

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

func Listen(ctx context.Context, address string) (net.Listener, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", address)
	if err == nil {
		return ln, nil
	}
	if errors.Is(err, os.ErrPermission) {
		return privilegedListen(ctx, address)
	}
	return nil, explain(err)
}
func explain(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return errors.New("445 端口已被占用，请检查系统文件共享或其他 SMB 服务，然后重试")
	}
	return fmt.Errorf("无法监听 SMB 端口：%w", err)
}
