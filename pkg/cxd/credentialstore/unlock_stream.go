package credentialstore

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WriteUnlockKey sends bounded, length-prefixed secret material only through a
// private host-to-proxy stdin stream. Never place it in arguments or metadata.
func WriteUnlockKey(output io.Writer, key []byte) error {
	if len(key) == 0 || len(key) > UnlockKeyMaxSize {
		return ErrUnlock
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(key)))
	if n, err := output.Write(size[:]); err != nil || n != len(size) {
		return fmt.Errorf("proxy key stream failed")
	}
	if n, err := output.Write(key); err != nil || n != len(key) {
		return fmt.Errorf("proxy key stream failed")
	}
	return nil
}
func ReadUnlockKey(input io.Reader) ([]byte, error) {
	var size [4]byte
	if _, err := io.ReadFull(input, size[:]); err != nil {
		return nil, ErrUnlock
	}
	length := binary.BigEndian.Uint32(size[:])
	if length == 0 || length > UnlockKeyMaxSize {
		return nil, ErrUnlock
	}
	key := make([]byte, length)
	if _, err := io.ReadFull(input, key); err != nil {
		clear(key)
		return nil, ErrUnlock
	}
	return key, nil
}
