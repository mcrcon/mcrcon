package rcon

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ErrAuthFailed is returned when the server rejects the RCON password.
var ErrAuthFailed = errors.New("rcon: authentication failed (wrong password?)")

// ErrNotConnected is returned when operating on a closed client.
var ErrNotConnected = errors.New("rcon: not connected")

const (
	defaultDialTimeout = 8 * time.Second
	defaultReqTimeout  = 8 * time.Second
	// How long to wait for trailing fragments of a multi-packet response.
	fragmentWait = 150 * time.Millisecond
)

// Client is a thread-safe Source/Minecraft RCON client.
type Client struct {
	conn    net.Conn
	mu      sync.Mutex // serializes request/response sequences
	id      atomic.Int32
	timeout time.Duration
	closed  atomic.Bool
}

// Dial connects to addr (host:port) and authenticates with password.
// It returns *Client on success or an error (ErrAuthFailed on bad password).
func Dial(addr, password string, timeout time.Duration) (*Client, error) {
	if timeout <= 0 {
		timeout = defaultReqTimeout
	}
	dialer := &net.Dialer{Timeout: defaultDialTimeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("rcon: dial %s: %w", addr, err)
	}
	c := &Client{conn: conn, timeout: timeout}
	c.id.Store(1)

	if err := c.auth(password); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// DialDefault is Dial with default timeout.
func DialDefault(addr, password string) (*Client, error) {
	return Dial(addr, password, defaultReqTimeout)
}

func (c *Client) nextID() int32 {
	for {
		id := c.id.Add(1)
		// 0 and -1 are reserved-ish (auth failure uses -1). Skip them.
		if id == 0 || id == -1 {
			continue
		}
		return id
	}
}

func (c *Client) setDeadline() error {
	if c.timeout > 0 {
		return c.conn.SetDeadline(time.Now().Add(c.timeout))
	}
	return nil
}

// auth performs the RCON handshake. Must be called before any Execute.
// Handles servers that send an extra empty RESPONSE_VALUE before AUTH_RESPONSE.
func (c *Client) auth(password string) error {
	id := c.nextID()
	req := Packet{ID: id, Type: TypeAuth, Body: password}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.setDeadline(); err != nil {
		return err
	}
	if err := req.Write(c.conn); err != nil {
		return fmt.Errorf("rcon: send auth: %w", err)
	}

	// Some servers (incl. vanilla Minecraft wrappers) send an empty
	// RESPONSE_VALUE packet before the AUTH_RESPONSE. Read up to 2 packets.
	for i := 0; i < 2; i++ {
		resp, err := ReadPacket(c.conn)
		if err != nil {
			return fmt.Errorf("rcon: read auth response: %w", err)
		}
		if resp.Type != TypeAuthResponse && resp.Type != TypeResponseValue {
			continue
		}
		if resp.ID == -1 {
			return ErrAuthFailed
		}
		if resp.ID == id {
			return nil
		}
		// Otherwise: stray/empty packet, keep reading once more.
	}
	return ErrAuthFailed
}

// Execute sends a command and returns the server response.
// It is safe for concurrent use (requests are serialized).
// Multi-packet responses are reassembled: after the first packet, it keeps
// reading fragments with the same ID until a short idle timeout.
func (c *Client) Execute(command string) (string, error) {
	if c.closed.Load() {
		return "", ErrNotConnected
	}
	command = strings.TrimRight(command, "\r\n")
	if command == "" {
		return "", nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.nextID()
	req := Packet{ID: id, Type: TypeExecCommand, Body: command}

	if err := c.setDeadline(); err != nil {
		return "", err
	}
	if err := req.Write(c.conn); err != nil {
		return "", fmt.Errorf("rcon: send command: %w", err)
	}

	// First response packet (full timeout).
	resp, err := ReadPacket(c.conn)
	if err != nil {
		return "", fmt.Errorf("rcon: read response: %w", err)
	}
	if resp.ID == -1 {
		return "", ErrAuthFailed
	}
	var sb strings.Builder
	// Accumulate only packets matching our request ID.
	// (Auth responses interleaved shouldn't happen since we're serialized.)
	if resp.ID == id {
		sb.WriteString(resp.Body)
	} else {
		// Unexpected ID — still return its body rather than failing hard,
		// as some proxies behave oddly. But keep trying for our ID briefly.
		sb.WriteString(resp.Body)
	}

	// Drain possible trailing fragments with a short deadline.
	// Vanilla Minecraft usually answers in a single packet; Source servers
	// may split large outputs (e.g. `help`) into several ~4096B packets.
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(fragmentWait))
		frag, err := ReadPacket(c.conn)
		if err != nil {
			// Timeout => no more fragments. Anything else => stop too,
			// response collected so far is still usable.
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			break
		}
		if frag.ID != id {
			// Not ours; stop draining to avoid stealing another response
			// (shouldn't happen while serialized, but be safe).
			// Push-back isn't possible on net.Conn, so just stop.
			// In practice this branch rarely triggers.
			if frag.ID == -1 {
				return sb.String(), ErrAuthFailed
			}
			break
		}
		sb.WriteString(frag.Body)
		// Empty trailing body is a common terminator; if the server sends
		// one, we can stop early — but only if we've already got content
		// OR it's genuinely the terminator. Keep it simple: if fragment is
		// empty, assume end of multi-packet response.
		if frag.Body == "" {
			break
		}
		// Small fragment (< ~4k) usually means last chunk.
		if len(frag.Body) < 4000 {
			// Peek once more in case of exact-multiple splits? The next
			// loop iteration with short timeout handles it.
			// To avoid an extra 150ms on every command, stop here when the
			// fragment is clearly short. Large outputs still work because
			// full-size chunks continue immediately without timeout.
			// Heuristic: if < 4000, do one more non-blocking-ish read?
			// We already loop; break only if next read times out anyway.
			// For snappiness, break now — the next iteration would just wait
			// fragmentWait then break. But that wait is needed to detect
			// exact-multiple splits. Compromise: break now.
			break
		}
	}
	_ = c.conn.SetDeadline(time.Now().Add(c.timeout))

	return sb.String(), nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// RemoteAddr returns the server address, or "" if closed.
func (c *Client) RemoteAddr() string {
	if c.conn != nil {
		return c.conn.RemoteAddr().String()
	}
	return ""
}
