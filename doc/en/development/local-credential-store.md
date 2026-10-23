# Local encrypted credential store specification

[日本語](../../development/local-credential-store.md) | **English**

- This page specifies the implementation to build. Encrypted storage is not available yet.
- Current storage and support: [Credential Store](../credential-store.md).
- Priorities: simple operation, local processing, no additional persistent service, and WSL/headless use.

## Storage

| Item | Specification |
|---|---|
| Default | Retain existing 0700 directories and 0600 plaintext files |
| Encryption | Explicit, optional activation |
| Location | Product-owned profile storage on the control host; never shared with projects |
| Encrypted data | Credentials, including API keys and OAuth access, refresh and ID tokens |
| Public metadata | Format version, profile name, unlock backend, salt, KDF parameters and nonce; no secrets |
| External services | None; storage and unlocking work offline |
| Implementation | Embedded in the Go binary, without another Vault, database or container |

## Keys and encryption

| Item | Specification |
|---|---|
| Data key | A cryptographically random 256-bit key per profile |
| Data encryption | XChaCha20-Poly1305 with a fresh nonce for every write |
| Tamper and substitution detection | Authenticate the format version and profile identity as associated data |
| Passphrase | Derive a wrapping key with Argon2id and encrypt the data key |
| Salt | Random per profile; regenerate when changing the passphrase |
| KDF cost | Measure on small control VMs before fixing parameters; bound parameters read from files |
| Runtime overhead | Reuse the unlocked data key; no KDF per inference or OAuth refresh |
| Cryptographic implementation | Existing Go cryptographic libraries, without custom algorithms |

- Implementations: [XChaCha20-Poly1305](https://pkg.go.dev/golang.org/x/crypto/chacha20poly1305), [Argon2id](https://pkg.go.dev/golang.org/x/crypto/argon2).
- Do not support automatic unlocking through a plaintext key stored alongside the ciphertext.

## Unlocking and everyday use

| Method or operation | Specification |
|---|---|
| Basic method | Hidden passphrase prompt when starting the Auth Proxy |
| Running proxy | No repeated prompt per task; the existing proxy process holds the key in memory |
| Proxy stop or host reboot | Unlock again on next startup |
| OS key storage | Optional, explicitly selected backend using an available Secret Service or equivalent |
| WSL, SSH, no GUI | Passphrase path; no mandatory systemd, D-Bus, TPM or Windows integration |
| Noninteractive operation | Explicit dedicated input FD; fail if no input mechanism is available |
| Key transfer | No secrets in argv, environment, configuration files, container metadata or logs |

- An unavailable OS backend never silently switches to a passphrase or plaintext storage.
- Do not introduce a product-specific daemon solely for key storage.
- Secret Service uses an [existing user-session service](https://specifications.freedesktop.org/secret-service/latest-single/).

## Auth Proxy and work VMs

1. Unlock the profile on the host.
2. The Auth Proxy loads credentials for upstream connections.
3. Work VMs receive only short-lived placeholders.
4. Encrypt and durably save OAuth refresh results before using them.

- Do not give work VMs ciphertext, real credentials, unlocking keys or credential-store sockets.
- A login Kata is a separate use: collect official CLI login results from temporary memory, then remove the session on success, cancellation or failure.
- Never automatically import the host CLI's authentication.

## Saving, locking and migration

| Operation or condition | Specification |
|---|---|
| OAuth refresh | Preserve the Store contract, expected-version comparison and connection generation |
| Durable save | Encrypted temporary file, fsync, atomic rename and directory fsync |
| Save failure or version conflict | Fail; revoke new requests and existing proxy streams |
| Explicit lock | Stop/revoke affected executions and stop credential-holding proxies |
| After locking | Retain ciphertext and work products |
| Plaintext migration | Explicit operation; validate encrypted save/reload before switching |
| Existing profiles | No automatic conversion or overwrite |
| Passphrase change | Rewrap the data key; preserve the previous method if the change fails |
| Lost key | Treat as undecryptable; reauthenticate to establish a new connection |
| Backup restore or cloning | Reauthenticate with a new connection generation; never automatically reauthorize old workers |

## Protection limits

- Main protection: credential files read from stopped systems, disks or backups.
- Storage encryption alone does not protect against control-host root, same-UID process access, running memory, swap or VM memory snapshots.
- Go memory management cannot guarantee zeroing every secret copy. Minimize copies and bound their lifetime by stopping credential-holding processes.
- A local store cannot guarantee detecting an outer VM snapshot rollback that restores keys, ciphertext and generation state together.
- Prefer the Linux filesystem on WSL. Support DrvFS/shared filesystems only after validating their permission and persistence guarantees.

## Required validation

- Reject wrong passphrases, tampered/substituted ciphertext and excessive KDF parameters.
- Never fall back to plaintext or host authentication after save failures, concurrent updates, cancellation, proxy restart or unavailable key backends.
- Check work-VM files, environment and logs, plus normal host logs and container metadata, for real-secret retention.
- Pass existing Store conformance and real-Kata acceptance tests.
- Measure unlocking time/memory and overhead during inference and OAuth refresh.
