package fixtures

import "embed"

// Files supplies an offline demo and the default CI inputs.
//
//go:embed *.json
var Files embed.FS
