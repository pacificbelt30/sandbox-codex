package cxd

import "github.com/pacificbelt30/codex-dock/internal/workerpolicy"

type TemporaryStorage = workerpolicy.TemporaryStorage
type StorageMedium = workerpolicy.StorageMedium
type StorageLifetime = workerpolicy.StorageLifetime

const (
	StorageMemory     = workerpolicy.StorageMemory
	StorageLifetimeVM = workerpolicy.StorageLifetimeVM
)
