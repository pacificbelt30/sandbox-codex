package credentialstore

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"filippo.io/age"
)

const (
	ageProfileSchema = 2
	// N=2^17, r=8, p=1 uses about 128 MiB only when unlocking an identity.
	ageScryptWorkFactor    = 17
	ageMaxScryptWorkFactor = 18
	UnlockKeyMaxSize       = 1024
)

type ageProfileSecret struct {
	Schema            int    `json:"schema"`
	Identity          string `json:"identity"`
	AuthenticationKey []byte `json:"authentication_key"`
}

func newAgeSecret() (key []byte, recipient string, err error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, "", fmt.Errorf("creating age identity failed")
	}
	auth := make([]byte, 32)
	defer clear(auth)
	if _, err = rand.Read(auth); err != nil {
		return nil, "", err
	}
	key, err = json.Marshal(ageProfileSecret{Schema: 1, Identity: identity.String(), AuthenticationKey: auth})
	return key, identity.Recipient().String(), err
}
func parseAgeSecret(key []byte) (ageProfileSecret, *age.X25519Identity, error) {
	var secret ageProfileSecret
	if len(key) > UnlockKeyMaxSize || json.Unmarshal(key, &secret) != nil || secret.Schema != 1 || len(secret.AuthenticationKey) != 32 {
		clear(secret.AuthenticationKey)
		return ageProfileSecret{}, nil, ErrUnlock
	}
	identity, err := age.ParseX25519Identity(secret.Identity)
	if err != nil {
		clear(secret.AuthenticationKey)
		return ageProfileSecret{}, nil, ErrUnlock
	}
	return secret, identity, nil
}
func ageEncrypt(data []byte, recipient age.Recipient) ([]byte, error) {
	var output bytes.Buffer
	writer, err := age.Encrypt(&output, recipient)
	if err != nil {
		return nil, fmt.Errorf("age encryption initialization failed")
	}
	if _, err = writer.Write(data); err != nil {
		return nil, fmt.Errorf("age encryption failed")
	}
	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("age encryption finalization failed")
	}
	return output.Bytes(), nil
}
func ageDecrypt(data []byte, identity age.Identity, limit int64) ([]byte, error) {
	reader, err := age.Decrypt(bytes.NewReader(data), identity)
	if err != nil {
		return nil, ErrUnlock
	}
	plain, err := io.ReadAll(io.LimitReader(reader, limit+1))
	// Read through EOF to authenticate the final stream chunk. Never use partial
	// plaintext after a truncation, tag error or size-limit failure.
	if err != nil || int64(len(plain)) > limit {
		clear(plain)
		return nil, ErrUnlock
	}
	return plain, nil
}
func passphraseAgeRecipient(pass []byte) (*age.ScryptRecipient, error) {
	if len(pass) < 12 || len(pass) > 4096 {
		return nil, fmt.Errorf("choose a passphrase of 12..4096 bytes")
	}
	recipient, err := age.NewScryptRecipient(string(pass))
	if err != nil {
		return nil, ErrUnlock
	}
	recipient.SetWorkFactor(ageScryptWorkFactor)
	return recipient, nil
}
func passphraseAgeIdentity(pass []byte) (*age.ScryptIdentity, error) {
	if len(pass) == 0 || len(pass) > 4096 {
		return nil, ErrUnlock
	}
	identity, err := age.NewScryptIdentity(string(pass))
	if err != nil {
		return nil, ErrUnlock
	}
	identity.SetMaxWorkFactor(ageMaxScryptWorkFactor)
	return identity, nil
}
func protectPassphraseKey(name string, pass, key []byte) (encryptedHeader, error) {
	secret, identity, err := parseAgeSecret(key)
	if err != nil {
		return encryptedHeader{}, err
	}
	defer clear(secret.AuthenticationKey)
	recipient, err := passphraseAgeRecipient(pass)
	if err != nil {
		return encryptedHeader{}, err
	}
	wrapped, err := ageEncrypt(key, recipient)
	if err != nil {
		return encryptedHeader{}, err
	}
	id, err := newProfileVersion()
	if err != nil {
		return encryptedHeader{}, err
	}
	return encryptedHeader{Schema: ageProfileSchema, Name: name, KeyID: id, Backend: "passphrase", Recipient: identity.Recipient().String(), WrappedKey: wrapped}, nil
}
func newPassphraseHeader(name string, pass []byte) (encryptedHeader, []byte, error) {
	key, _, err := newAgeSecret()
	if err != nil {
		return encryptedHeader{}, nil, err
	}
	header, err := protectPassphraseKey(name, pass, key)
	if err != nil {
		clear(key)
		return encryptedHeader{}, nil, err
	}
	return header, key, nil
}
func passphraseRecordKey(record encryptedRecord, pass []byte) ([]byte, error) {
	if record.Header.Backend != "passphrase" {
		return nil, ErrUnlock
	}
	if record.Header.Schema == 1 {
		return legacyPassphraseKey(record, pass)
	}
	identity, err := passphraseAgeIdentity(pass)
	if err != nil {
		return nil, err
	}
	key, err := ageDecrypt(record.Header.WrappedKey, identity, UnlockKeyMaxSize)
	if err != nil {
		return nil, err
	}
	if _, err = openEncrypted(record, key); err != nil {
		clear(key)
		return nil, ErrUnlock
	}
	return key, nil
}

// age public-key encryption does not authenticate the sender. A separate
// random authentication key, protected together with the age private identity,
// binds the manifest and ciphertext. Merely knowing the public recipient must
// not let a file writer replace credentials or profile metadata undetected.
func ageRecordMAC(header encryptedHeader, ciphertext, authKey []byte) []byte {
	mac := hmac.New(sha256.New, authKey)
	_, _ = mac.Write([]byte("oaiso/age/profile/v2\x00"))
	_, _ = mac.Write(dataAAD(header))
	_, _ = mac.Write(ciphertext)
	return mac.Sum(nil)
}
func openEncrypted(record encryptedRecord, key []byte) (Credentials, error) {
	if record.Header.Schema == 1 {
		return legacyOpenEncrypted(record, key)
	}
	secret, identity, err := parseAgeSecret(key)
	if err != nil {
		return Credentials{}, err
	}
	defer clear(secret.AuthenticationKey)
	return openAgeEncrypted(record, identity, secret.AuthenticationKey)
}
func openAgeEncrypted(record encryptedRecord, identity *age.X25519Identity, authenticationKey []byte) (Credentials, error) {
	if identity.Recipient().String() != record.Header.Recipient || !hmac.Equal(record.MAC, ageRecordMAC(record.Header, record.Ciphertext, authenticationKey)) {
		return Credentials{}, ErrUnlock
	}
	data, err := ageDecrypt(record.Ciphertext, identity, 1<<20)
	if err != nil {
		return Credentials{}, ErrUnlock
	}
	defer clear(data)
	return decodeEncryptedCredentials(data, record.Header.Name)
}
func decodeEncryptedCredentials(data []byte, name string) (Credentials, error) {
	var value fileProfile
	if json.Unmarshal(data, &value) != nil || value.Schema != 1 || value.Name != name {
		return Credentials{}, ErrUnlock
	}
	if validateFileCredentials(value.Credentials) != nil {
		return Credentials{}, ErrUnlock
	}
	return value.Credentials, nil
}
func sealEncrypted(header encryptedHeader, key []byte, credentials Credentials) (encryptedRecord, error) {
	if header.Schema != ageProfileSchema {
		return encryptedRecord{}, ErrMigrationRequired
	}
	if err := validateFileCredentials(credentials); err != nil {
		return encryptedRecord{}, err
	}
	secret, identity, err := parseAgeSecret(key)
	if err != nil {
		return encryptedRecord{}, err
	}
	defer clear(secret.AuthenticationKey)
	if identity.Recipient().String() != header.Recipient {
		return encryptedRecord{}, ErrUnlock
	}
	data, err := json.Marshal(fileProfile{Schema: 1, Name: header.Name, Credentials: credentials})
	if err != nil || len(data) > 1<<20 {
		return encryptedRecord{}, fmt.Errorf("profile exceeds supported size")
	}
	defer clear(data)
	ciphertext, err := ageEncrypt(data, identity.Recipient())
	if err != nil {
		return encryptedRecord{}, err
	}
	return encryptedRecord{Header: header, Ciphertext: ciphertext, MAC: ageRecordMAC(header, ciphertext, secret.AuthenticationKey)}, nil
}
