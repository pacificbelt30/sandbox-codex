package credentialstore

import (
	"context"
	"os"
)

// WriteEncryptedFixture is a test-only control for explicit generations in the
// shared conformance suite. It is not compiled into the production package.
func WriteEncryptedFixture(ctx context.Context, s *EncryptedStore, v Credentials) error {
	return s.base.locked(ctx, s.name, func(dir *os.File) error {
		s.mu.Lock()
		key := append([]byte{}, s.key...)
		h := s.header
		s.mu.Unlock()
		defer clear(key)
		record, err := sealEncrypted(h, key, v)
		if err != nil {
			return err
		}
		return writeEncrypted(ctx, dir, record)
	})
}
