// Package releaseauth verifies publisher-signed updates without a paid CA.
// Its transport stages verified bytes but has no installation or execution authority.
package releaseauth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	EnvelopeSchema   = "pfremote.signed-release/v1"
	MetadataSchema   = "pfremote.release-metadata/v1"
	MetadataSchemaV2 = "pfremote.release-metadata/v2"
	Domain           = "PF Remote release v1\n"
	MaxEnvelope      = 32 << 10
	MaxArtifact      = int64(2 << 30)
)

type Artifact struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Metadata struct {
	SchemaVersion string         `json:"schema_version"`
	Product       string         `json:"product"`
	Version       string         `json:"version"`
	Channel       string         `json:"channel"`
	Platform      string         `json:"platform"`
	Sequence      uint64         `json:"sequence"`
	IssuedAt      time.Time      `json:"issued_at"`
	ExpiresAt     time.Time      `json:"expires_at"`
	Artifacts     []Artifact     `json:"artifacts"`
	Compatibility *Compatibility `json:"compatibility,omitempty"`
}

// Compatibility is signed by the publisher. DataEpoch means both versions can
// read/write the same persisted data; changing it forbids unattended activation.
type Compatibility struct {
	DataEpoch   uint64 `json:"data_epoch"`
	ProtocolMin uint64 `json:"protocol_min"`
	ProtocolMax uint64 `json:"protocol_max"`
}

func (r Release) Compatible(dataEpoch, protocol uint64) bool {
	c := r.metadata.Compatibility
	return r.checkpoint.Sequence > 0 && r.metadata.SchemaVersion == MetadataSchemaV2 && c != nil && dataEpoch > 0 && protocol > 0 && c.DataEpoch == dataEpoch && c.ProtocolMin <= protocol && protocol <= c.ProtocolMax
}

type Envelope struct {
	SchemaVersion string `json:"schema_version"`
	Payload       string `json:"payload"`
	Signature     string `json:"signature"`
}

// Checkpoint must be held in protected durable local state, scoped to a publisher,
// channel and platform. It is never accepted from an update server.
type Checkpoint struct {
	Sequence uint64 `json:"sequence"`
	Digest   string `json:"digest"`
}

type Policy struct {
	PublicKey ed25519.PublicKey
	Channel   string
	Platform  string
	Previous  Checkpoint
	Now       time.Time
}

type Release struct {
	metadata   Metadata
	checkpoint Checkpoint
	document   []byte
}

func (r Release) Document() []byte { return append([]byte(nil), r.document...) }

func (r Release) Metadata() Metadata {
	m := r.metadata
	m.Artifacts = append([]Artifact(nil), m.Artifacts...)
	if m.Compatibility != nil {
		c := *m.Compatibility
		m.Compatibility = &c
	}
	return m
}

func (r Release) Checkpoint() Checkpoint { return r.checkpoint }

type Fault struct{ Code string }

func (f *Fault) Error() string { return "Update verification failed: " + f.Code }

func fail(code string) error { return &Fault{Code: code} }

var (
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+([.-][0-9A-Za-z]+)*)?$`)
	namePattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	hashPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Verify validates authenticity before interpreting the payload. SHA256 alone
// would not authenticate a release because an attacker can replace both files.
func Verify(document []byte, policy Policy) (Release, error) {
	if len(policy.PublicKey) != ed25519.PublicKeySize || policy.Now.IsZero() ||
		!validChannel(policy.Channel) || !validPlatform(policy.Platform) ||
		(policy.Previous.Sequence == 0 && policy.Previous.Digest != "") ||
		(policy.Previous.Sequence != 0 && !hashPattern.MatchString(policy.Previous.Digest)) {
		return Release{}, fail("TRUST_STATE_INVALID")
	}
	var envelope Envelope
	if len(document) == 0 || len(document) > MaxEnvelope || strictJSON(document, &envelope) != nil || envelope.SchemaVersion != EnvelopeSchema {
		return Release{}, fail("RELEASE_ENVELOPE_INVALID")
	}
	payload, payloadErr := base64.StdEncoding.Strict().DecodeString(envelope.Payload)
	signature, signatureErr := base64.StdEncoding.Strict().DecodeString(envelope.Signature)
	if payloadErr != nil || signatureErr != nil || len(payload) == 0 || len(payload) > 16<<10 ||
		len(signature) != ed25519.SignatureSize || !ed25519.Verify(policy.PublicKey, append([]byte(Domain), payload...), signature) {
		return Release{}, fail("RELEASE_SIGNATURE_INVALID")
	}
	var metadata Metadata
	if strictJSON(payload, &metadata) != nil || !validMetadata(metadata) {
		return Release{}, fail("RELEASE_METADATA_INVALID")
	}
	if metadata.Channel != policy.Channel || metadata.Platform != policy.Platform {
		return Release{}, fail("RELEASE_POLICY_MISMATCH")
	}
	if metadata.IssuedAt.After(policy.Now) || !metadata.ExpiresAt.After(policy.Now) {
		return Release{}, fail("RELEASE_TIME_INVALID")
	}
	digest := sha256.Sum256(payload)
	checkpoint := Checkpoint{Sequence: metadata.Sequence, Digest: hex.EncodeToString(digest[:])}
	if checkpoint.Sequence < policy.Previous.Sequence ||
		(checkpoint.Sequence == policy.Previous.Sequence && checkpoint.Digest != policy.Previous.Digest) {
		return Release{}, fail("RELEASE_REPLAY_REJECTED")
	}
	return Release{metadata: metadata, checkpoint: checkpoint, document: append([]byte(nil), document...)}, nil
}

// Sign accepts a publisher signing function; its key never enters the metadata.
// Callers must provision an independent publisher key, not an enrolled device key.
func Sign(metadata Metadata, sign func([]byte) ([]byte, error)) ([]byte, error) {
	if !validMetadata(metadata) || sign == nil {
		return nil, fail("RELEASE_METADATA_INVALID")
	}
	payload, err := json.Marshal(metadata)
	if err != nil || len(payload) > 16<<10 {
		return nil, fail("RELEASE_METADATA_INVALID")
	}
	signature, err := sign(append([]byte(Domain), payload...))
	if err != nil || len(signature) != ed25519.SignatureSize {
		return nil, fail("RELEASE_SIGNING_FAILED")
	}
	return json.Marshal(Envelope{EnvelopeSchema, base64.StdEncoding.EncodeToString(payload), base64.StdEncoding.EncodeToString(signature)})
}

func (r Release) VerifyArtifact(name string, input io.Reader) error {
	if r.checkpoint.Sequence == 0 || input == nil {
		return fail("RELEASE_NOT_VERIFIED")
	}
	for _, artifact := range r.metadata.Artifacts {
		if artifact.Name != name {
			continue
		}
		hash := sha256.New()
		count, err := io.Copy(hash, io.LimitReader(input, artifact.Size+1))
		if err != nil || count != artifact.Size || hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
			return fail("RELEASE_ARTIFACT_INVALID")
		}
		return nil
	}
	return fail("RELEASE_ARTIFACT_UNKNOWN")
}

func validMetadata(m Metadata) bool {
	if (m.SchemaVersion != MetadataSchema && m.SchemaVersion != MetadataSchemaV2) || m.Product != "PF Remote" || len(m.Version) > 80 || !versionPattern.MatchString(m.Version) ||
		!validChannel(m.Channel) || !validPlatform(m.Platform) || (m.Channel == "stable" && strings.Contains(m.Version, "-")) ||
		m.Sequence == 0 || m.IssuedAt.IsZero() || !m.ExpiresAt.After(m.IssuedAt) || m.ExpiresAt.Sub(m.IssuedAt) > 90*24*time.Hour ||
		len(m.Artifacts) == 0 || len(m.Artifacts) > 16 {
		return false
	}
	if m.SchemaVersion == MetadataSchema && m.Compatibility != nil {
		return false
	}
	if m.SchemaVersion == MetadataSchemaV2 && (m.Compatibility == nil || m.Compatibility.DataEpoch == 0 || m.Compatibility.ProtocolMin == 0 || m.Compatibility.ProtocolMax < m.Compatibility.ProtocolMin) {
		return false
	}
	seen := make(map[string]bool)
	for _, a := range m.Artifacts {
		name := strings.ToUpper(a.Name)
		base, _, _ := strings.Cut(name, ".")
		reserved := base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9')
		if !namePattern.MatchString(a.Name) || strings.HasSuffix(a.Name, ".") || reserved || seen[name] ||
			a.Size < 1 || a.Size > MaxArtifact || !hashPattern.MatchString(a.SHA256) {
			return false
		}
		seen[name] = true
	}
	return true
}

func validChannel(v string) bool  { return v == "stable" || v == "preview" }
func validPlatform(v string) bool { return v == "windows-x64" || v == "linux-x64" }

func strictJSON(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	if err := walkJSON(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing data")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(value)
}

func walkJSON(d *json.Decoder, depth int) error {
	if depth > 12 {
		return errors.New("JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := make(map[string]bool)
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			// encoding/json matches struct fields case-insensitively as well.
			name = strings.ToLower(name)
			if !ok || seen[name] {
				return errors.New("duplicate or invalid JSON member")
			}
			seen[name] = true
			if err := walkJSON(d, depth+1); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	case json.Delim('['):
		for d.More() {
			if err := walkJSON(d, depth+1); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	}
	return nil
}
