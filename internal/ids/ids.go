package ids

import (
	"crypto/rand"
	"encoding/hex"
	"math/big"
)

func UUIDv4() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	dst := make([]byte, 36)
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst)
}

func NumericID() int64 {
	const min = int64(1_000_000_000)
	const span = int64(9_000_000_000)
	n, err := rand.Int(rand.Reader, big.NewInt(span))
	if err != nil {
		panic(err)
	}
	return min + n.Int64()
}
