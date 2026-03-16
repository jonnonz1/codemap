// Package hash provides BLAKE3 content hashing for files.
package hash

import (
	"encoding/hex"

	"lukechampine.com/blake3"
)

// Blake3Hex returns the hex-encoded BLAKE3 hash of data.
func Blake3Hex(data []byte) string {
	h := blake3.Sum256(data)
	return hex.EncodeToString(h[:])
}
