package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/OpenListTeam/go-wasi-socket/wasip2_net"

	"github.com/jlaffaye/ftp"
)

// do others that not defined in Driver interface

func (d *FTP) _login(ctx context.Context) (*ftp.ServerConn, error) {
	conn, err := ftp.Dial(d.Address, ftp.DialWithDialFunc(func(network, address string) (net.Conn, error) {
		return wasip2_net.DialContext(ctx, network, address)
	}))
	if err != nil {
		return nil, err
	}

	err = conn.Login(d.Username, d.Password)
	if err != nil {
		conn.Quit()
		return nil, err
	}
	return conn, nil
}

// 处理FTP地址，返回格式化后的地址（包含端口）
func ProcessFTPAddress(rawAddr string) (string, error) {
	// 步骤1：检查并去除协议前缀
	const ftpPrefix = "ftp://"
	if strings.HasPrefix(rawAddr, ftpPrefix) {
		// 去除ftp://前缀
		rawAddr = strings.TrimPrefix(rawAddr, ftpPrefix)
	} else if strings.Contains(rawAddr, "://") {
		// 存在其他协议前缀，不支持
		return "", errors.New("unsupported protocol prefix, only ftp:// is allowed")
	}

	// 步骤2：检查是否已包含端口
	host, port, err := net.SplitHostPort(rawAddr)
	if err != nil {
		// 没有端口号的情况
		if strings.Contains(err.Error(), "missing port in address") {
			// 使用默认端口21
			return net.JoinHostPort(rawAddr, "21"), nil
		}
		// 其他错误（如格式无效）
		return "", fmt.Errorf("invalid address format: %w", err)
	}

	// 步骤3：已包含端口，验证端口有效性
	if _, err := wasip2_net.LookupPort(context.Background(), "tcp", port); err != nil {
		return "", fmt.Errorf("invalid port number: %w", err)
	}

	// 返回带端口的地址
	return net.JoinHostPort(host, port), nil
}
