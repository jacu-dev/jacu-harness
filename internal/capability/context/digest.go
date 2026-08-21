package context

import (
	"crypto/sha256"
	"encoding/hex"
)

func Digest(pack Pack) string {
	sum := sha256.Sum256(Canonical(pack))
	return hex.EncodeToString(sum[:])
}
