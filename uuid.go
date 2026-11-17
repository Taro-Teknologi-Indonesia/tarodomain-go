package tarodomain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// UUID is a 128-bit identifier (RFC 9562).
type UUID [16]byte

// ErrInvalidUUID is returned, wrapped, by ParseUUID for text that isn't a
// hyphenated UUID.
var ErrInvalidUUID = errors.New("invalid UUID")

// String returns the lower-case hyphenated form.
func (u UUID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:], u[10:])
	return string(b[:])
}

// IsZero reports whether u is the nil UUID.
func (u UUID) IsZero() bool { return u == UUID{} }

// IsV7 reports whether u is an RFC 9562 version 7 UUID.
func (u UUID) IsV7() bool { return u[6]>>4 == 7 && u[8]>>6 == 0b10 }

// UnixMilli returns the 48-bit timestamp of a version 7 UUID.
func (u UUID) UnixMilli() int64 {
	return int64(u[0])<<40 | int64(u[1])<<32 | int64(u[2])<<24 | int64(u[3])<<16 | int64(u[4])<<8 | int64(u[5])
}

// ParseUUID parses the hyphenated 8-4-4-4-12 form of any version, in upper or
// lower case. Other forms (braces, URN prefix, no hyphens) are rejected.
func ParseUUID(s string) (UUID, error) {
	var u UUID
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return u, fmt.Errorf("%w: %q", ErrInvalidUUID, s)
	}
	pos := 0
	for i := 0; i < 36; {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			i++
			continue
		}
		hi, ok1 := fromHex(s[i])
		lo, ok2 := fromHex(s[i+1])
		if !ok1 || !ok2 {
			return UUID{}, fmt.Errorf("%w: %q", ErrInvalidUUID, s)
		}
		u[pos] = hi<<4 | lo
		pos++
		i += 2
	}
	return u, nil
}

func fromHex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// MarshalText implements encoding.TextMarshaler, and so JSON as a string.
func (u UUID) MarshalText() ([]byte, error) { return []byte(u.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler.
func (u *UUID) UnmarshalText(text []byte) error {
	v, err := ParseUUID(string(text))
	if err != nil {
		return err
	}
	*u = v
	return nil
}

func newV7(unixMs int64, random [10]byte) UUID {
	var u UUID
	u[0], u[1], u[2] = byte(unixMs>>40), byte(unixMs>>32), byte(unixMs>>24)
	u[3], u[4], u[5] = byte(unixMs>>16), byte(unixMs>>8), byte(unixMs)
	copy(u[6:], random[:])
	u[6] = 0x70 | u[6]&0x0f
	u[8] = 0x80 | u[8]&0x3f
	return u
}

// NewUUIDv7 returns a version 7 UUID for the current time, with random bits
// from crypto/rand. It panics if the system's random source fails.
func NewUUIDv7() UUID {
	var r [10]byte
	if _, err := rand.Read(r[:]); err != nil {
		panic(fmt.Sprintf("tarodomain: random source failed: %v", err))
	}
	return newV7(time.Now().UnixMilli(), r)
}

// DerivedUUIDv7 returns a valid version 7 UUID whose timestamp is unixMs and
// whose other 74 bits come from SHA-256(seed, index). The same input always
// gives the same ID, so a redelivered event recreates the same command IDs.
func DerivedUUIDv7(seed string, index int, unixMs int64) UUID {
	h := sha256.New()
	h.Write([]byte(seed))
	var idx [8]byte
	binary.BigEndian.PutUint64(idx[:], uint64(index))
	h.Write(idx[:])
	sum := h.Sum(nil)
	var r [10]byte
	copy(r[:], sum)
	return newV7(unixMs, r)
}
