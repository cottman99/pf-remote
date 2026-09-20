package recovery

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	BundleSchema      = "pfremote.recovery-bundle/v1"
	PayloadSchema     = "pfremote.recovery-payload/v1"
	KDFName           = "PBKDF2-HMAC-SHA256"
	CipherName        = "AES-256-GCM"
	DefaultIterations = 600_000
	maxBundleBytes    = 8 << 20
	maxPayloadBytes   = 6 << 20
)

var ErrCannotOpen = errors.New("recovery file could not be opened with this recovery password")

type Payload struct {
	SchemaVersion    string          `json:"schema_version"`
	FabricID         string          `json:"fabric_id"`
	OwnerDeviceID    string          `json:"owner_device_id"`
	DirectoryVersion uint64          `json:"directory_version"`
	GrantVersion     uint64          `json:"grant_version"`
	ExportedAt       time.Time       `json:"exported_at"`
	EnrollmentState  json.RawMessage `json:"enrollment_state"`
	Snapshot         json.RawMessage `json:"snapshot"`
}

type bundle struct {
	SchemaVersion string `json:"schema_version"`
	CreatedAt     string `json:"created_at"`
	KDF           string `json:"kdf"`
	Iterations    int    `json:"iterations"`
	Salt          string `json:"salt"`
	Cipher        string `json:"cipher"`
	Nonce         string `json:"nonce"`
	Ciphertext    string `json:"ciphertext"`
}

func Seal(payload Payload, password string) ([]byte, error) {
	return seal(payload, password, DefaultIterations, rand.Reader, time.Now().UTC())
}

func seal(payload Payload, password string, iterations int, random io.Reader, now time.Time) ([]byte, error) {
	if err := validatePayload(payload); err != nil {
		return nil, err
	}
	if len([]rune(strings.TrimSpace(password))) < 12 {
		return nil, errors.New("recovery password must contain at least 12 characters")
	}
	if iterations < 100_000 || iterations > 2_000_000 {
		return nil, errors.New("recovery key derivation settings are invalid")
	}
	plaintext, err := json.Marshal(payload)
	if err != nil || len(plaintext) > maxPayloadBytes {
		return nil, errors.New("recovery contents are too large")
	}
	salt := make([]byte, 32)
	if _, err := io.ReadFull(random, salt); err != nil {
		return nil, errors.New("create recovery salt")
	}
	key := pbkdf2([]byte(password), salt, iterations, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("initialize recovery encryption")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("initialize recovery authentication")
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(random, nonce); err != nil {
		return nil, errors.New("create recovery nonce")
	}
	createdAt := now.UTC().Format(time.RFC3339Nano)
	aad := associatedData(BundleSchema, createdAt, KDFName, iterations, CipherName)
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)
	encoded, err := json.Marshal(bundle{
		SchemaVersion: BundleSchema, CreatedAt: createdAt, KDF: KDFName, Iterations: iterations,
		Salt: base64.RawURLEncoding.EncodeToString(salt), Cipher: CipherName,
		Nonce: base64.RawURLEncoding.EncodeToString(nonce), Ciphertext: base64.RawURLEncoding.EncodeToString(ciphertext),
	})
	if err != nil || len(encoded) > maxBundleBytes {
		return nil, errors.New("encode recovery file")
	}
	return encoded, nil
}

func Open(encoded []byte, password string) (Payload, error) {
	if len(encoded) == 0 || len(encoded) > maxBundleBytes || len([]rune(password)) < 1 {
		return Payload{}, ErrCannotOpen
	}
	var envelope bundle
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		envelope.SchemaVersion != BundleSchema || envelope.KDF != KDFName || envelope.Cipher != CipherName ||
		envelope.Iterations < 100_000 || envelope.Iterations > 2_000_000 {
		return Payload{}, ErrCannotOpen
	}
	salt, err := base64.RawURLEncoding.Strict().DecodeString(envelope.Salt)
	if err != nil || len(salt) != 32 {
		return Payload{}, ErrCannotOpen
	}
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != 12 {
		return Payload{}, ErrCannotOpen
	}
	ciphertext, err := base64.RawURLEncoding.Strict().DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < 16 || len(ciphertext) > maxPayloadBytes+16 {
		return Payload{}, ErrCannotOpen
	}
	key := pbkdf2([]byte(password), salt, envelope.Iterations, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return Payload{}, ErrCannotOpen
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Payload{}, ErrCannotOpen
	}
	aad := associatedData(envelope.SchemaVersion, envelope.CreatedAt, envelope.KDF, envelope.Iterations, envelope.Cipher)
	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return Payload{}, ErrCannotOpen
	}
	var payload Payload
	decoder = json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil || decoder.Decode(&struct{}{}) != io.EOF || validatePayload(payload) != nil {
		return Payload{}, ErrCannotOpen
	}
	return payload, nil
}

func validatePayload(payload Payload) error {
	if payload.SchemaVersion != PayloadSchema || payload.FabricID == "" || payload.OwnerDeviceID == "" ||
		payload.DirectoryVersion == 0 || payload.GrantVersion == 0 || payload.ExportedAt.IsZero() ||
		len(payload.EnrollmentState) == 0 || len(payload.Snapshot) == 0 {
		return errors.New("recovery contents are incomplete")
	}
	if len(payload.EnrollmentState)+len(payload.Snapshot) > maxPayloadBytes {
		return errors.New("recovery contents are too large")
	}
	return nil
}

func associatedData(schema, createdAt, kdf string, iterations int, cipherName string) []byte {
	return []byte(schema + "\n" + createdAt + "\n" + kdf + "\n" + strconv.Itoa(iterations) + "\n" + cipherName)
}

func pbkdf2(password, salt []byte, iterations, keyLength int, newHash func() hash.Hash) []byte {
	digestSize := newHash().Size()
	blocks := (keyLength + digestSize - 1) / digestSize
	derived := make([]byte, 0, blocks*digestSize)
	var counter [4]byte
	for blockIndex := 1; blockIndex <= blocks; blockIndex++ {
		binary.BigEndian.PutUint32(counter[:], uint32(blockIndex))
		mac := hmac.New(newHash, password)
		mac.Write(salt)
		mac.Write(counter[:])
		u := mac.Sum(nil)
		result := append([]byte(nil), u...)
		for iteration := 1; iteration < iterations; iteration++ {
			mac = hmac.New(newHash, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for index := range result {
				result[index] ^= u[index]
			}
		}
		derived = append(derived, result...)
	}
	return derived[:keyLength]
}
