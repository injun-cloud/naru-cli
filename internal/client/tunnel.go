package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// wsURL converts the REST base + path into a ws(s) URL.
func (c *Client) wsURL(path string) string {
	u := c.base + path
	if strings.HasPrefix(u, "https://") {
		return "wss://" + strings.TrimPrefix(u, "https://")
	}
	return "ws://" + strings.TrimPrefix(u, "http://")
}

// RunTunnel listens on localAddr and pipes every accepted TCP connection to the
// server tunnel endpoint over a WebSocket. It blocks until ctx is cancelled.
func (c *Client) RunTunnel(ctx context.Context, path, localAddr string, onListen func(string)) error {
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return err
	}
	defer ln.Close()
	if onListen != nil {
		onListen(ln.Addr().String())
	}
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go c.handleTunnelConn(ctx, path, conn)
	}
}

func (c *Client) handleTunnelConn(ctx context.Context, path string, tcp net.Conn) {
	defer tcp.Close()
	hdr := http.Header{}
	if c.token != "" {
		hdr.Set("Authorization", "Bearer "+c.token)
	}
	ws, resp, err := websocket.DefaultDialer.DialContext(ctx, c.wsURL(path), hdr)
	if err != nil {
		if resp != nil {
			fmt.Fprintf(io.Discard, "tunnel dial %d\n", resp.StatusCode)
		}
		return
	}
	defer ws.Close()

	done := make(chan struct{}, 2)
	// TCP -> WS
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := tcp.Read(buf)
			if n > 0 {
				if ws.WriteMessage(websocket.BinaryMessage, buf[:n]) != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	// WS -> TCP
	go func() {
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				break
			}
			if _, err := tcp.Write(data); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	<-done
}
