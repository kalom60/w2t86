package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// blindIndexSalt domain-separates the HMAC sub-key from the AES encryption
// key so that blind-index tokens are independent from ciphertext operations.
const blindIndexSalt = "blind_index_v1"

// BlindIndex returns a deterministic hex-encoded HMAC-SHA256 of value keyed
// by a sub-key derived from encKey.  The result allows SQL equality matching
// on sensitive fields while the primary storage remains randomized AES-GCM.
//
// encKey must be exactly 32 bytes (the AES-256 key used for EncryptField).
// Returns an empty string if encKey is not exactly 32 bytes.
func BlindIndex(encKey []byte, value string) string {
	if len(encKey) != 32 {
		return ""
	}
	// Derive a domain-specific sub-key.
	prk := hmac.New(sha256.New, encKey)
	prk.Write([]byte(blindIndexSalt))
	indexKey := prk.Sum(nil)

	mac := hmac.New(sha256.New, indexKey)
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// EncryptField encrypts plaintext using AES-256-GCM with the supplied 32-byte
// key.  The nonce is prepended to the ciphertext and the whole thing is
// returned as standard base64 so it is safe to store in a TEXT column.
func EncryptField(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("crypto: encryption key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: generate nonce: %w", err)
	}

	// Seal appends the ciphertext+tag to nonce.
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptField is the inverse of EncryptField.  It decodes the base64 blob,
// extracts the nonce, and returns the plaintext.
func DecryptField(key []byte, ciphertext string) (string, error) {
	if len(key) != 32 {
		return "", fmt.Errorf("crypto: encryption key must be 32 bytes, got %d", len(key))
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("crypto: base64 decode: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("crypto: create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("crypto: create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("crypto: ciphertext too short")
	}

	nonce, data := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: decrypt: %w", err)
	}

	return string(plaintext), nil
}

// GenerateRandomPassword returns a cryptographically-random URL-safe password
// of approximately 22 characters (16 random bytes, base64-URL encoded without
// padding).  Use it to bootstrap secrets that must never be a known value.
func GenerateRandomPassword() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("crypto: generate random password: %w", err)
	}
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b), nil
}

// HashPassword hashes password using bcrypt at cost 12.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("crypto: hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword returns true when password matches the stored bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// MaskName converts a full name into initials with a trailing period on each
// part.  Examples:
//
//	"John Doe"       → "J. D."
//	"Alice"          → "A."
//	"Mary Jane Watson" → "M. J. W."
//
// Empty input is returned unchanged.
func MaskName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	parts := strings.Fields(name)
	var sb strings.Builder
	for i, part := range parts {
		r, size := utf8.DecodeRuneInString(part)
		if size == 0 {
			continue
		}
		_ = r
		// Write just the first Unicode character of each word followed by ".".
		sb.WriteString(string([]rune(part)[:1]))
		sb.WriteByte('.')
		if i < len(parts)-1 {
			sb.WriteByte(' ')
		}
	}
	return sb.String()
}

// Soundex returns the American Soundex phonetic code for name.  The code
// consists of the first letter of the normalized name followed by three digits
// encoding consonant groups.  Names that sound alike produce the same code,
// enabling fuzzy-match duplicate detection without storing plaintext.
//
// Non-letter characters and spaces are stripped so that "John Doe" and
// "JohnDoe" produce the same result.  Input is case-insensitive.
func Soundex(name string) string {
	var runes []rune
	for _, r := range []rune(strings.ToUpper(name)) {
		if r >= 'A' && r <= 'Z' {
			runes = append(runes, r)
		}
	}
	if len(runes) == 0 {
		return ""
	}

	// Soundex code table indexed by letter offset from 'A'.
	// 0 = ignored (vowels, H, W, Y); 1–6 = consonant groups.
	const codeTable = "01230120022455012623010202"
	code := func(r rune) byte { return codeTable[r-'A'] }

	result := make([]byte, 1, 4)
	result[0] = byte(runes[0])
	prev := code(runes[0])

	for i := 1; i < len(runes) && len(result) < 4; i++ {
		c := code(runes[i])
		if c == '0' {
			// Vowel / ignored: acts as separator so the same code on both
			// sides of a vowel is still emitted twice.
			prev = '0'
			continue
		}
		if c != prev {
			result = append(result, c)
			prev = c
		}
	}

	for len(result) < 4 {
		result = append(result, '0')
	}
	return string(result)
}

// MaskID masks all but the last four characters of id with asterisks.
// Examples:
//
//	"1234567890"  → "******7890"
//	"AB12"        → "AB12"   (≤4 chars: returned as-is)
//	""            → ""
func MaskID(id string) string {
	runes := []rune(id)
	n := len(runes)
	if n <= 4 {
		return id
	}
	masked := make([]rune, n)
	for i := 0; i < n-4; i++ {
		masked[i] = '*'
	}
	copy(masked[n-4:], runes[n-4:])
	return string(masked)
}
