package rcon

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// Packet types (Source RCON / Minecraft RCON).
const (
	TypeResponseValue = 0 // SERVERDATA_RESPONSE_VALUE
	TypeExecCommand   = 2 // SERVERDATA_EXECCOMMAND
	TypeAuthResponse  = 2 // SERVERDATA_AUTH_RESPONSE (same value as exec)
	TypeAuth          = 3 // SERVERDATA_AUTH
)

// MaxPayloadSize guards against malicious / corrupt servers.
const MaxPayloadSize = 1024 * 1024 // 1 MiB

// Packet is a single RCON packet.
type Packet struct {
	ID   int32
	Type int32
	Body string
}

// Encode serializes the packet to wire format (little endian).
func (p Packet) Encode() ([]byte, error) {
	body := []byte(p.Body)
	if len(body) > MaxPayloadSize {
		return nil, fmt.Errorf("rcon: payload too large (%d bytes)", len(body))
	}
	size := int32(10 + len(body)) // id(4) + type(4) + body + 2x null
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, size); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, p.ID); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, p.Type); err != nil {
		return nil, err
	}
	if _, err := buf.Write(body); err != nil {
		return nil, err
	}
	if _, err := buf.Write([]byte{0x00, 0x00}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Write writes the encoded packet to w.
func (p Packet) Write(w io.Writer) error {
	b, err := p.Encode()
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// ReadPacket reads a single packet from r, handling TCP fragmentation
// via io.ReadFull. It validates size and terminators.
func ReadPacket(r io.Reader) (Packet, error) {
	var size int32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return Packet{}, err
	}
	if size < 10 {
		return Packet{}, fmt.Errorf("rcon: invalid packet size %d", size)
	}
	if size > MaxPayloadSize+10 {
		return Packet{}, fmt.Errorf("rcon: packet too large (%d bytes)", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Packet{}, err
	}
	// Last two bytes must be 0x00 0x00.
	if payload[len(payload)-2] != 0x00 || payload[len(payload)-1] != 0x00 {
		return Packet{}, fmt.Errorf("rcon: missing packet terminator")
	}
	var pkt Packet
	pkt.ID = int32(binary.LittleEndian.Uint32(payload[0:4]))
	pkt.Type = int32(binary.LittleEndian.Uint32(payload[4:8]))
	pkt.Body = string(payload[8 : len(payload)-2])
	return pkt, nil
}
