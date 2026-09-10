package rcon

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"
)

// startFakeServer runs a minimal RCON server on loopback for tests.
// password is required; handler maps command -> response.
func startFakeServer(t *testing.T, password string, handler func(cmd string) string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				authed := false
				for {
					pkt, err := ReadPacket(c)
					if err != nil {
						return
					}
					switch pkt.Type {
					case TypeAuth:
						if pkt.Body == password {
							authed = true
							_ = Packet{ID: pkt.ID, Type: TypeAuthResponse, Body: ""}.Write(c)
						} else {
							_ = Packet{ID: -1, Type: TypeAuthResponse, Body: ""}.Write(c)
							return
						}
					case TypeExecCommand:
						if !authed {
							_ = Packet{ID: -1, Type: TypeAuthResponse, Body: ""}.Write(c)
							return
						}
						resp := handler(pkt.Body)
						// Simulate multi-packet split for large responses.
						const chunk = 4000
						if len(resp) <= chunk {
							_ = Packet{ID: pkt.ID, Type: TypeResponseValue, Body: resp}.Write(c)
						} else {
							for i := 0; i < len(resp); i += chunk {
								end := i + chunk
								if end > len(resp) {
									end = len(resp)
								}
								_ = Packet{ID: pkt.ID, Type: TypeResponseValue, Body: resp[i:end]}.Write(c)
							}
						}
					}
				}
			}(conn)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

func TestPacketRoundTrip(t *testing.T) {
	p := Packet{ID: 42, Type: TypeExecCommand, Body: "say hello"}
	b, err := p.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadPacket(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("got %+v want %+v", got, p)
	}
}

func TestDialAndExecute(t *testing.T) {
	addr := startFakeServer(t, "secret", func(cmd string) string {
		if cmd == "list" {
			return "There are 2 of a max of 20 players online: Steve, Alex"
		}
		return "ok: " + cmd
	})
	c, err := Dial(addr, "secret", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	out, err := c.Execute("list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Steve") {
		t.Fatalf("unexpected response: %q", out)
	}
}

func TestAuthFailure(t *testing.T) {
	addr := startFakeServer(t, "secret", func(cmd string) string { return "x" })
	_, err := Dial(addr, "wrong", 3*time.Second)
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestLargeMultiPacketResponse(t *testing.T) {
	big := strings.Repeat("A", 10000)
	addr := startFakeServer(t, "secret", func(cmd string) string { return big })
	c, err := Dial(addr, "secret", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	out, err := c.Execute("big")
	if err != nil {
		t.Fatal(err)
	}
	if out != big {
		t.Fatalf(" reassembled len=%d want %d", len(out), len(big))
	}
}
