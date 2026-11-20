package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidTOTPCode = errors.New("invalid TOTP code")
	ErrTOTPCodeReused  = errors.New("TOTP code already used in this or a prior time step")
)

type TOTPEnrollment struct {
	OtpAuthURI      string
	PlaintextSecret string
	EncryptedSecret string
}

// VerifyArgon2id checks an encoded PHC Argon2id hash: $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
func VerifyArgon2id(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unsupported argon2id hash format")
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, fmt.Errorf("parse_argon2id_params: %w", err)
	}
	salt, err := decodeRawB64(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode_argon2id_salt: %w", err)
	}
	expectedHash, err := decodeRawB64(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode_argon2id_digest: %w", err)
	}

	derived := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expectedHash)))
	return subtle.ConstantTimeCompare(derived, expectedHash) == 1, nil
}

// HashArgon2id hashes a staff password using Argon2id with a random 16-byte salt and PHC formatting.
func HashArgon2id(password string) (string, error) {
	if strings.TrimSpace(password) == "" {
		return "", errors.New("password cannot be empty")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2id_salt: %w", err)
	}
	const (
		memory      uint32 = 65536
		iterations  uint32 = 3
		parallelism uint8  = 2
		keyLen      uint32 = 32
	)
	digest := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLen)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory,
		iterations,
		parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

// GenerateRandomBase32Secret generates a 160-bit (20-byte) RFC 4226 secret encoded in unpadded Base32.
func GenerateRandomBase32Secret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("rand_totp_secret: %w", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

// GenerateTOTPEnrollment creates a 160-bit secret, standard otpauth:// URI for authenticator apps,
// and an AES-256-GCM encrypted copy for at-rest storage in users.totp_secret.
func GenerateTOTPEnrollment(issuer, accountEmail, encKey string) (TOTPEnrollment, error) {
	if strings.TrimSpace(issuer) == "" {
		issuer = "Venu"
	}
	secret, err := GenerateRandomBase32Secret()
	if err != nil {
		return TOTPEnrollment{}, err
	}
	encrypted, err := EncryptTOTPSecret(secret, encKey)
	if err != nil {
		return TOTPEnrollment{}, err
	}

	label := url.PathEscape(fmt.Sprintf("%s:%s", issuer, accountEmail))
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	uri := fmt.Sprintf("otpauth://totp/%s?%s", label, q.Encode())

	return TOTPEnrollment{
		OtpAuthURI:      uri,
		PlaintextSecret: secret,
		EncryptedSecret: encrypted,
	}, nil
}

const encPrefix = "enc:v1:"

// EncryptTOTPSecret protects a TOTP Base32 secret at rest using AES-256-GCM.
func EncryptTOTPSecret(plaintextSecret, encKey string) (string, error) {
	if strings.TrimSpace(encKey) == "" {
		return plaintextSecret, nil
	}
	keyHash := sha256.Sum256([]byte(encKey))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return "", fmt.Errorf("aes_cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("aes_gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("aes_nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintextSecret), nil)
	return encPrefix + base64.RawURLEncoding.EncodeToString(nonce) + "." + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// DecryptTOTPSecret decrypts an AES-256-GCM encrypted TOTP secret (or returns legacy unencrypted Base32).
func DecryptTOTPSecret(stored, encKey string) (string, error) {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored, nil
	}
	if strings.TrimSpace(encKey) == "" {
		return "", errors.New("MFA_ENCRYPTION_KEY required to decrypt TOTP secret")
	}
	body := strings.TrimPrefix(stored, encPrefix)
	parts := strings.Split(body, ".")
	if len(parts) != 2 {
		return "", errors.New("malformed encrypted TOTP secret")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode_totp_nonce: %w", err)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode_totp_ciphertext: %w", err)
	}
	keyHash := sha256.Sum256([]byte(encKey))
	block, err := aes.NewCipher(keyHash[:])
	if err != nil {
		return "", fmt.Errorf("aes_cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("aes_gcm: %w", err)
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt_totp_secret: %w", err)
	}
	return string(plain), nil
}

// VerifyTOTPWithStep validates an RFC 6238 6-digit TOTP code across [-1, 0, +1] 30-second windows
// without early exit, using constant-time comparison, and enforces step-level replay protection
// against lastAcceptedStep.
func VerifyTOTPWithStep(
	base32Secret string,
	code string,
	lastAcceptedStep *int64,
	now time.Time,
) (int64, error) {
	cleaned := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(base32Secret, " ", "")))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(cleaned)
	if err != nil || len(code) != 6 {
		return 0, ErrInvalidTOTPCode
	}

	serverStep := now.UTC().Unix() / 30
	candidates := []int64{serverStep - 1, serverStep, serverStep + 1}

	var matched bool
	var greatestMatchedStep int64
	for _, step := range candidates {
		if step < 0 {
			continue
		}
		expected := computeHOTP(key, uint64(step))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			if !matched || step > greatestMatchedStep {
				greatestMatchedStep = step
				matched = true
			}
		}
	}

	if !matched {
		return 0, ErrInvalidTOTPCode
	}
	if lastAcceptedStep != nil && greatestMatchedStep <= *lastAcceptedStep {
		return 0, ErrTOTPCodeReused
	}
	return greatestMatchedStep, nil
}

// VerifyTOTP validates a 6-digit RFC 6238 TOTP code across [-1, 0, +1] 30-second windows.
func VerifyTOTP(base32Secret, code string, now time.Time) bool {
	_, err := VerifyTOTPWithStep(base32Secret, code, nil, now)
	return err == nil
}

// GenerateTOTP produces a 6-digit RFC 6238 TOTP code for the given base32 secret and timestamp.
func GenerateTOTP(base32Secret string, now time.Time) (string, error) {
	cleaned := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(base32Secret, " ", "")))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(cleaned)
	if err != nil {
		return "", fmt.Errorf("decode_totp_secret: %w", err)
	}
	step := now.UTC().Unix() / 30
	return computeHOTP(key, uint64(step)), nil
}

func computeHOTP(secret []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, secret)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	binaryCode := (int(sum[offset]&0x7f) << 24) |
		(int(sum[offset+1]&0xff) << 16) |
		(int(sum[offset+2]&0xff) << 8) |
		int(sum[offset+3]&0xff)
	return fmt.Sprintf("%06d", binaryCode%1000000)
}

func decodeRawB64(raw string) ([]byte, error) {
	cleaned := strings.TrimRight(raw, "=")
	return decodeB64Fallback(cleaned)
}
