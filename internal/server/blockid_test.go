package server

import (
	"encoding/base64"
	"encoding/binary"
	"testing"
)

func TestBlockID_48Byte(t *testing.T) {
	raw := []byte("00000000-0000-0000-0000-000000000000" + "00000000017")
	if len(raw) != 47 {
		t.Fatalf("setup: %d", len(raw))
	}
	raw = append(raw, '0')
	id := base64.StdEncoding.EncodeToString(raw)
	got, ok := chunkIndexFromBlockID(id)
	if !ok || got != 170 {
		t.Errorf("got %d ok=%v want 170", got, ok)
	}
}

func TestBlockID_64Byte(t *testing.T) {
	b := make([]byte, 64)
	binary.BigEndian.PutUint32(b[16:20], 7)
	id := base64.StdEncoding.EncodeToString(b)
	got, ok := chunkIndexFromBlockID(id)
	if !ok || got != 7 {
		t.Errorf("got %d ok=%v", got, ok)
	}
}

func TestBlockID_BadLength(t *testing.T) {
	id := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, ok := chunkIndexFromBlockID(id); ok {
		t.Error("expected !ok")
	}
}
