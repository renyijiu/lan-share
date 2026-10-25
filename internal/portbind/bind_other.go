//go:build !darwin

package portbind

import (
	"context"
	"errors"
	"net"
	"runtime"
)

func privilegedListen(ctx context.Context, address string) (net.Listener, error) {
	if runtime.GOOS == "windows" {
		return nil, errors.New("系统拒绝监听 445，请检查 Windows 文件共享是否占用端口，以及防火墙或系统策略")
	}
	return nil, errors.New("监听 445 需要系统端口授权；Linux 请为服务配置绑定低端口的能力。此版本尚未提供 Linux 授权安装器")
}
