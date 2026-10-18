// Package kataassets embeds the host installer so installed CLIs do not need a source checkout.
package kataassets

import "embed"

//go:embed setup.py configure.py prepare_guest.py prepare-node.sh pids_hook.c assets.lock.json
var Files embed.FS
