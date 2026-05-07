package server

import (
	"encoding/base64"
	"encoding/binary"
	"strconv"
)

func chunkIndexFromBlockID(b64 string) (int, bool) {
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0, false
	}
	switch len(dec) {
	case 64:
		return int(binary.BigEndian.Uint32(dec[16:20])), true
	case 48:
		s := string(dec)
		if len(s) <= 36 {
			return 0, false
		}
		n, err := strconv.Atoi(s[36:])
		if err != nil {
			return 0, false
		}
		return n, true
	}
	return 0, false
}
