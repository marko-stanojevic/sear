package common

import (
	"crypto/rand"
	"time"
)

// Crockford's Base32 alphabet — omits I, L, O, U to avoid visual ambiguity.
const ulidAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newULID returns a 26-character ULID string per https://github.com/ulid/spec:
//   - chars  0–9  : 48-bit Unix millisecond timestamp (lexicographically sortable)
//   - chars 10–25 : 80-bit cryptographically random component
func newULID() string {
	var id [26]byte

	// ── Timestamp (48 bits → 10 Crockford base32 chars) ─────────────────────
	ms := uint64(time.Now().UnixMilli())
	id[0] = ulidAlphabet[(ms>>45)&0x1F]
	id[1] = ulidAlphabet[(ms>>40)&0x1F]
	id[2] = ulidAlphabet[(ms>>35)&0x1F]
	id[3] = ulidAlphabet[(ms>>30)&0x1F]
	id[4] = ulidAlphabet[(ms>>25)&0x1F]
	id[5] = ulidAlphabet[(ms>>20)&0x1F]
	id[6] = ulidAlphabet[(ms>>15)&0x1F]
	id[7] = ulidAlphabet[(ms>>10)&0x1F]
	id[8] = ulidAlphabet[(ms>>5)&0x1F]
	id[9] = ulidAlphabet[ms&0x1F]

	// ── Randomness (80 bits = 10 bytes → 16 Crockford base32 chars) ─────────
	var rnd [10]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		panic("kompakt: crypto/rand unavailable: " + err.Error())
	}
	id[10] = ulidAlphabet[(rnd[0]>>3)&0x1F]
	id[11] = ulidAlphabet[((rnd[0]&0x07)<<2|(rnd[1]>>6))&0x1F]
	id[12] = ulidAlphabet[(rnd[1]>>1)&0x1F]
	id[13] = ulidAlphabet[((rnd[1]&0x01)<<4|(rnd[2]>>4))&0x1F]
	id[14] = ulidAlphabet[((rnd[2]&0x0F)<<1|(rnd[3]>>7))&0x1F]
	id[15] = ulidAlphabet[(rnd[3]>>2)&0x1F]
	id[16] = ulidAlphabet[((rnd[3]&0x03)<<3|(rnd[4]>>5))&0x1F]
	id[17] = ulidAlphabet[rnd[4]&0x1F]
	id[18] = ulidAlphabet[(rnd[5]>>3)&0x1F]
	id[19] = ulidAlphabet[((rnd[5]&0x07)<<2|(rnd[6]>>6))&0x1F]
	id[20] = ulidAlphabet[(rnd[6]>>1)&0x1F]
	id[21] = ulidAlphabet[((rnd[6]&0x01)<<4|(rnd[7]>>4))&0x1F]
	id[22] = ulidAlphabet[((rnd[7]&0x0F)<<1|(rnd[8]>>7))&0x1F]
	id[23] = ulidAlphabet[(rnd[8]>>2)&0x1F]
	id[24] = ulidAlphabet[((rnd[8]&0x03)<<3|(rnd[9]>>5))&0x1F]
	id[25] = ulidAlphabet[rnd[9]&0x1F]

	return string(id[:])
}

// NewID returns a 26-character ULID (no prefix).
// Example: "01JFZB4HCR8Y2V4Q5X7N3M6P0W"
func NewID() string {
	return newULID()
}
