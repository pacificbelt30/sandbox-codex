package credentialstore

import (
	"filippo.io/age"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// Schema 1 is read-only. Keep only its decoder so existing development profiles
// can be explicitly migrated; all new writes use age (schema 2).
func legacyOpenEncrypted(record encryptedRecord, key []byte) (Credentials, error) {
	box, err := chacha20poly1305.NewX(key)
	if err != nil {
		return Credentials{}, ErrUnlock
	}
	data, err := box.Open(nil, record.Nonce, record.Ciphertext, dataAAD(record.Header))
	if err != nil {
		return Credentials{}, ErrUnlock
	}
	defer clear(data)
	return decodeEncryptedCredentials(data, record.Header.Name)
}
func legacyPassphraseKey(record encryptedRecord, pass []byte) ([]byte, error) {
	h := record.Header
	if h.Backend != "passphrase" || len(pass) > 4096 {
		return nil, ErrUnlock
	}
	derived := argon2.IDKey(pass, h.Salt, h.Time, h.Memory, h.Parallelism, 32)
	defer clear(derived)
	return legacyKeyringKey(record, derived)
}
func legacyKeyringKey(record encryptedRecord, wrapper []byte) ([]byte, error) {
	box, err := chacha20poly1305.NewX(wrapper)
	if err != nil {
		return nil, ErrUnlock
	}
	key, err := box.Open(nil, record.Header.WrapNonce, record.Header.WrappedKey, wrapAAD(record.Header))
	if err != nil {
		return nil, ErrUnlock
	}
	if _, err = legacyOpenEncrypted(record, key); err != nil {
		clear(key)
		return nil, ErrUnlock
	}
	return key, nil
}
func validEncryptedRecord(r encryptedRecord) bool {
	h := r.Header
	if h.Schema == 1 {
		return h.Recipient == "" && validLegacyUnlockParameters(h) && len(h.WrapNonce) == 24 && len(h.WrappedKey) == 48 && len(r.Nonce) == 24 && len(r.Ciphertext) >= 16 && len(r.MAC) == 0
	}
	if h.Schema != ageProfileSchema || h.Memory != 0 || h.Time != 0 || h.Parallelism != 0 || len(h.Salt) != 0 || len(h.WrapNonce) != 0 || len(r.Nonce) != 0 || len(r.MAC) != 32 || len(r.Ciphertext) < 32 {
		return false
	}
	if len(h.Recipient) > 128 {
		return false
	}
	if _, err := age.ParseX25519Recipient(h.Recipient); err != nil {
		return false
	}
	switch h.Backend {
	case "passphrase":
		return len(h.WrappedKey) > 0 && len(h.WrappedKey) <= 4096
	case "secret-service":
		return len(h.WrappedKey) == 0
	}
	return false
}
func validLegacyUnlockParameters(h encryptedHeader) bool {
	switch h.Backend {
	case "passphrase":
		return h.Memory >= 64*1024 && h.Memory <= 128*1024 && h.Time >= 3 && h.Time <= 6 && h.Parallelism >= 1 && h.Parallelism <= 4 && len(h.Salt) == 16
	case "secret-service":
		return h.Memory == 0 && h.Time == 0 && h.Parallelism == 0 && len(h.Salt) == 0
	}
	return false
}
