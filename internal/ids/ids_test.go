package ids

import (
	"regexp"
	"testing"
)

func TestUUIDv4_Format(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	for i := 0; i < 100; i++ {
		got := UUIDv4()
		if !re.MatchString(got) {
			t.Fatalf("UUIDv4 not v4-shaped: %q", got)
		}
	}
}

func TestUUIDv4_Unique(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 1000; i++ {
		u := UUIDv4()
		if _, dup := seen[u]; dup {
			t.Fatalf("duplicate uuid: %s", u)
		}
		seen[u] = struct{}{}
	}
}

func TestNumericID(t *testing.T) {
	re := regexp.MustCompile(`^[1-9][0-9]{9}$`)
	for i := 0; i < 100; i++ {
		got := NumericID()
		if !re.MatchString(formatInt64(got)) {
			t.Fatalf("NumericID not 10-digit: %d", got)
		}
	}
}

func formatInt64(n int64) string {
	if n < 0 {
		return "-"
	}
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	return string(buf)
}
