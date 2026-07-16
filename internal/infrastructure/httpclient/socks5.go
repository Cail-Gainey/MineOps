package httpclient

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

func socks5DialContext(proxyAddress, username, password string, bypass []string, timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	direct := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	return func(ctx context.Context, network, targetAddress string) (net.Conn, error) {
		targetHost, targetPortText, err := net.SplitHostPort(targetAddress)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeValidationInvalidArgument, "SOCKS5 目标地址无效", err)
		}
		if bypassProxy(targetHost, bypass) {
			return direct.DialContext(ctx, network, targetAddress)
		}
		connection, err := direct.DialContext(ctx, "tcp", proxyAddress)
		if err != nil {
			return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "连接 SOCKS5 Proxy 失败", err).WithRetryable(true)
		}
		closeConnection := true
		defer func() {
			if closeConnection {
				_ = connection.Close()
			}
		}()
		if deadline, ok := ctx.Deadline(); ok {
			_ = connection.SetDeadline(deadline)
		}
		methods := []byte{0x00}
		if username != "" {
			methods = append(methods, 0x02)
		}
		if _, err := connection.Write(append([]byte{0x05, byte(len(methods))}, methods...)); err != nil {
			return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "发送 SOCKS5 握手失败", err)
		}
		reply := make([]byte, 2)
		if _, err := io.ReadFull(connection, reply); err != nil || reply[0] != 0x05 || reply[1] == 0xff {
			return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "SOCKS5 Proxy 拒绝认证方式", err)
		}
		if reply[1] == 0x02 {
			if len(username) > 255 || len(password) > 255 {
				return nil, apperror.New(apperror.CodeValidationInvalidArgument, "SOCKS5 用户名或密码过长")
			}
			auth := []byte{0x01, byte(len(username))}
			auth = append(auth, []byte(username)...)
			auth = append(auth, byte(len(password)))
			auth = append(auth, []byte(password)...)
			if _, err := connection.Write(auth); err != nil {
				return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "发送 SOCKS5 认证失败", err)
			}
			if _, err := io.ReadFull(connection, reply); err != nil || reply[1] != 0x00 {
				return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "SOCKS5 用户名或密码认证失败", err)
			}
		}
		targetPort, err := strconv.Atoi(targetPortText)
		if err != nil || targetPort < 1 || targetPort > 65535 || len(targetHost) > 255 {
			return nil, apperror.New(apperror.CodeValidationInvalidArgument, "SOCKS5 目标 Host 或 Port 无效")
		}
		request := []byte{0x05, 0x01, 0x00}
		if ip := net.ParseIP(targetHost); ip != nil {
			if ipv4 := ip.To4(); ipv4 != nil {
				request = append(request, 0x01)
				request = append(request, ipv4...)
			} else {
				request = append(request, 0x04)
				request = append(request, ip.To16()...)
			}
		} else {
			request = append(request, 0x03, byte(len(targetHost)))
			request = append(request, []byte(targetHost)...)
		}
		portBytes := make([]byte, 2)
		binary.BigEndian.PutUint16(portBytes, uint16(targetPort))
		request = append(request, portBytes...)
		if _, err := connection.Write(request); err != nil {
			return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "发送 SOCKS5 CONNECT 失败", err)
		}
		header := make([]byte, 4)
		if _, err := io.ReadFull(connection, header); err != nil {
			return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "读取 SOCKS5 CONNECT 响应失败", err)
		}
		if header[0] != 0x05 || header[1] != 0x00 {
			return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "SOCKS5 CONNECT 被拒绝", fmt.Errorf("reply=%v", header))
		}
		addressLength := 0
		switch header[3] {
		case 0x01:
			addressLength = 4
		case 0x04:
			addressLength = 16
		case 0x03:
			length := make([]byte, 1)
			if _, err := io.ReadFull(connection, length); err != nil {
				return nil, err
			}
			addressLength = int(length[0])
		default:
			return nil, apperror.New(apperror.CodeHTTPConnectionFailed, "SOCKS5 返回未知地址类型")
		}
		if _, err := io.CopyN(io.Discard, connection, int64(addressLength+2)); err != nil {
			return nil, apperror.Wrap(apperror.CodeHTTPConnectionFailed, "读取 SOCKS5 CONNECT 响应失败", err)
		}
		_ = connection.SetDeadline(time.Time{})
		closeConnection = false
		return connection, nil
	}
}
