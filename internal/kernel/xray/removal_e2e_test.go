//go:build e2e

package xray

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	xrayCore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"

	"github.com/cedar2025/xboard-node/internal/config"
	"github.com/cedar2025/xboard-node/internal/kernel"
	"github.com/cedar2025/xboard-node/internal/model"
)

// Run with: go test -tags e2e -run TestE2E -v ./internal/kernel/xray/
//
// Starts the real xray kernel as a VLESS node, connects real xray clients
// (with and without Mux.Cool) through it to an endless download, removes one user and checks that the
// user's traffic stops while the other user's keeps flowing.

var (
	e2eUser1 = model.UserSpec{ID: 1, UUID: "aaaaaaaa-1111-2222-3333-444444444444"}
	e2eUser2 = model.UserSpec{ID: 2, UUID: "bbbbbbbb-5555-6666-7777-888888888888"}
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// endlessServer streams data to every accepted connection until it breaks.
func endlessServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	chunk := make([]byte, 32*1024)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				for {
					if _, err := c.Write(chunk); err != nil {
						return
					}
				}
			}()
		}
	}()
	return l.Addr().String()
}

func startClient(t *testing.T, serverPort int, uuid string, mux bool) int {
	t.Helper()
	port := freePort(t)
	cfg := map[string]any{
		"log":      map[string]any{"loglevel": "error"},
		"inbounds": []any{map[string]any{"protocol": "http", "listen": "127.0.0.1", "port": port}},
		"outbounds": []any{map[string]any{
			"protocol": "vless",
			"settings": map[string]any{"vnext": []any{map[string]any{
				"address": "127.0.0.1", "port": serverPort,
				"users": []any{map[string]any{"id": uuid, "encryption": "none"}},
			}}},
			"mux": map[string]any{"enabled": mux, "concurrency": 8},
		}},
	}
	data, _ := json.Marshal(cfg)
	pb, err := serial.LoadJSONConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := xrayCore.New(pb)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { instance.Close() })
	return port
}

// openStream asks the client's mixed inbound to CONNECT to target.
func openStream(clientPort int, target string) (net.Conn, error) {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", clientPort), 3*time.Second)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target)
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		c.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		c.Close()
		return nil, fmt.Errorf("CONNECT: %s", resp.Status)
	}
	// Prove data flows end to end.
	if _, err := io.ReadFull(br, make([]byte, 256*1024)); err != nil {
		c.Close()
		return nil, err
	}
	c.SetReadDeadline(time.Time{})
	return &bufConn{Conn: c, r: br}, nil
}

type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufConn) Read(b []byte) (int, error) { return c.r.Read(b) }

// stopsWithin reads until the stream breaks. The server never stops sending,
// so hitting the deadline means the stream is still flowing.
func stopsWithin(c net.Conn, d time.Duration) (bool, int64) {
	deadline := time.Now().Add(d)
	c.SetReadDeadline(deadline)
	n, err := io.Copy(io.Discard, c)
	if err == nil {
		return true, n // EOF
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return false, n
	}
	return true, n
}

func runRemovalE2E(t *testing.T, mux bool) {
	target := endlessServer(t)
	serverPort := freePort(t)

	spec := &model.NodeSpec{
		Protocol: "vless", ServerPort: serverPort, ListenIP: "127.0.0.1",
		// The node blocks private ranges by default; the test target is on loopback.
		CustomRoutes: []map[string]any{{"type": "field", "ip": []string{"127.0.0.0/8"}, "outboundTag": "direct"}},
	}
	k := New(config.KernelConfig{Type: "xray", LogLevel: "error"})
	if err := k.Start(spec, []model.UserSpec{e2eUser1, e2eUser2}, kernel.TLSCert{}); err != nil {
		t.Fatalf("start node: %v", err)
	}
	t.Cleanup(k.Stop)

	c1Port := startClient(t, serverPort, e2eUser1.UUID, mux)
	c2Port := startClient(t, serverPort, e2eUser2.UUID, mux)

	s1, err := openStream(c1Port, target)
	if err != nil {
		t.Fatalf("user1 stream before removal: %v", err)
	}
	defer s1.Close()
	s2, err := openStream(c2Port, target)
	if err != nil {
		t.Fatalf("user2 stream: %v", err)
	}
	defer s2.Close()

	start := time.Now()
	if _, err := k.RemoveUsers([]model.UserSpec{e2eUser1}); err != nil {
		t.Fatalf("RemoveUsers: %v", err)
	}

	stopped, n := stopsWithin(s1, 3*time.Second)
	if !stopped {
		t.Fatalf("removed user's stream still open after 3s (%d bytes after removal)", n)
	}
	t.Logf("removed user's stream ended %v after removal, %d bytes in between", time.Since(start).Round(time.Millisecond), n)

	if _, err := openStream(c1Port, target); err == nil {
		t.Fatal("removed user could open a new stream")
	} else {
		t.Logf("new stream of removed user rejected: %v", strings.SplitN(err.Error(), "\n", 2)[0])
	}

	s2.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(s2, make([]byte, 1024*1024)); err != nil {
		t.Fatalf("remaining user's stream broke: %v", err)
	}
}

func TestE2ERemoveUserClosesConnections(t *testing.T) {
	t.Run("plain", func(t *testing.T) { runRemovalE2E(t, false) })
	t.Run("mux", func(t *testing.T) { runRemovalE2E(t, true) })
}
