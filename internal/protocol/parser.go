package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

const (
	SSLRequestCode     = 80877103
	StartupMessageCode = 196608
)

type StartupMessage struct {
	User     string
	Database string
	Options  map[string]string
}

func ParseStartup(conn net.Conn) (*StartupMessage, error) {
	for {
		header := make([]byte, 4)
		if _, err := io.ReadFull(conn, header); err != nil {
			return nil, fmt.Errorf("failed to read message length: %w", err)
		}
		length := binary.BigEndian.Uint32(header)

		codeBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, codeBuf); err != nil {
			return nil, fmt.Errorf("failed to read protocol code: %w", err)
		}
		code := binary.BigEndian.Uint32(codeBuf)

		if code == SSLRequestCode {
			if _, err := conn.Write([]byte{'N'}); err != nil {
				return nil, fmt.Errorf("failed to deny SSL: %w", err)
			}
			continue
		}

		if code == StartupMessageCode {
			payloadSize := int(length) - 8
			if payloadSize < 0 {
				return nil, fmt.Errorf("invalid startup message length")
			}

			payload := make([]byte, payloadSize)
			if _, err := io.ReadFull(conn, payload); err != nil {
				return nil, fmt.Errorf("failed to read startup payload: %w", err)
			}

			return decodePayload(payload), nil
		}

		return nil, fmt.Errorf("unknown protocol code: %d", code)
	}
}

func decodePayload(data []byte) *StartupMessage {
	msg := &StartupMessage{
		Options: make(map[string]string),
	}

	parts := bytes.Split(data, []byte{0})

	for i := 0; i < len(parts)-1; i += 2 {
		key := string(parts[i])
		val := string(parts[i+1])

		if key == "" {
			break
		}

		switch key {
		case "user":
			msg.User = val
		case "database":
			msg.Database = val
		default:
			msg.Options[key] = val
		}
	}

	return msg
}
