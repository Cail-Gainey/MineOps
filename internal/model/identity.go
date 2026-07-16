package model

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"time"
)

// ID is a time-ordered UUIDv7-compatible MineOps identifier.
type ID string

// String returns the serialized UUIDv7 identifier.
func (id ID) String() string {
	return string(id)
}

// NewID creates a UUIDv7-compatible identifier using the supplied UTC timestamp.
func NewID(now time.Time) (ID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	milliseconds := uint64(now.UTC().UnixMilli())
	value[0] = byte(milliseconds >> 40)
	value[1] = byte(milliseconds >> 32)
	value[2] = byte(milliseconds >> 24)
	value[3] = byte(milliseconds >> 16)
	value[4] = byte(milliseconds >> 8)
	value[5] = byte(milliseconds)
	value[6] = (value[6] & 0x0f) | 0x70
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return ID(encoded), nil
}

// Valid reports whether the ID has the UUIDv7 shape and version bits.
func (id ID) Valid() bool {
	value := string(id)
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' || value[14] != '7' {
		return false
	}
	decoded := make([]byte, 16)
	compact := value[0:8] + value[9:13] + value[14:18] + value[19:23] + value[24:36]
	if _, err := hex.Decode(decoded, []byte(compact)); err != nil {
		return false
	}
	return decoded[8]&0xc0 == 0x80
}

// Time extracts the millisecond UTC timestamp encoded in the UUIDv7 identifier.
func (id ID) Time() (time.Time, error) {
	if !id.Valid() {
		return time.Time{}, errors.New("invalid MineOps ID")
	}
	value := string(id)
	decoded, err := hex.DecodeString(value[0:8] + value[9:13])
	if err != nil {
		return time.Time{}, err
	}
	var buffer [8]byte
	copy(buffer[2:], decoded)
	milliseconds := binary.BigEndian.Uint64(buffer[:])
	return time.UnixMilli(int64(milliseconds)).UTC(), nil
}
