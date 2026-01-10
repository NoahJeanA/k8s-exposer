package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/noahjeana/k8s-exposer/pkg/types"
)

// SendMessage sends a message over the connection with length prefix framing
func SendMessage(w io.Writer, msg *types.Message) error {
	// Validate message before sending
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("message validation failed: %w", err)
	}

	// Encode message to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Write length prefix (4 bytes, big endian)
	length := uint32(len(data))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	// Write message data
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write message data: %w", err)
	}

	return nil
}

// ReceiveMessage receives a message from the connection with length prefix framing
func ReceiveMessage(r io.Reader) (*types.Message, error) {
	// Read length prefix (4 bytes, big endian)
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to read message length: %w", err)
	}

	// Sanity check: limit message size to 10MB
	if length > 10*1024*1024 {
		return nil, fmt.Errorf("message too large: %d bytes", length)
	}

	// Read message data
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read message data: %w", err)
	}

	// Decode JSON
	var msg types.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	// Validate received message
	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("received invalid message: %w", err)
	}

	return &msg, nil
}
