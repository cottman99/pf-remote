package releaseauth

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (Metadata, Policy, ed25519.PrivateKey) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	hash := sha256.Sum256([]byte("synthetic artifact"))
	m := Metadata{MetadataSchema, "PF Remote", "0.1.0-alpha.94", "preview", "windows-x64", 7,
		now.Add(-time.Hour), now.Add(24 * time.Hour), []Artifact{{"payload.zip", 18, hex.EncodeToString(hash[:])}}, nil}
	return m, Policy{PublicKey: pub, Channel: "preview", Platform: "windows-x64", Now: now}, key
}

func signed(t *testing.T, m Metadata, key ed25519.PrivateKey) []byte {
	t.Helper()
	doc, err := Sign(m, func(data []byte) ([]byte, error) { return ed25519.Sign(key, data), nil })
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func expectCode(t *testing.T, err error, code string) {
	t.Helper()
	var fault *Fault
	if !errors.As(err, &fault) || fault.Code != code {
		t.Fatalf("got %v; want %s", err, code)
	}
}

func TestRoundTripAndArtifactIntegrity(t *testing.T) {
	m, policy, key := fixture(t)
	release, err := Verify(signed(t, m, key), policy)
	if err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyArtifact("payload.zip", strings.NewReader("synthetic artifact")); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{"synthetic", "synthetic artifact!", "SYNTHETIC ARTIFACT"} {
		expectCode(t, release.VerifyArtifact("payload.zip", strings.NewReader(content)), "RELEASE_ARTIFACT_INVALID")
	}
	expectCode(t, release.VerifyArtifact("other.exe", strings.NewReader("anything")), "RELEASE_ARTIFACT_UNKNOWN")
	expectCode(t, (Release{}).VerifyArtifact("payload.zip", strings.NewReader("anything")), "RELEASE_NOT_VERIFIED")
	copy := release.Metadata()
	copy.Artifacts[0].SHA256 = strings.Repeat("0", 64)
	if err := release.VerifyArtifact("payload.zip", strings.NewReader("synthetic artifact")); err != nil {
		t.Fatal("metadata mutation changed verified release")
	}
}

func TestTrustAndPolicyFailures(t *testing.T) {
	m, policy, key := fixture(t)
	doc := signed(t, m, key)
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*Policy)
		code   string
	}{
		{"wrong publisher", func(p *Policy) { p.PublicKey = other }, "RELEASE_SIGNATURE_INVALID"},
		{"missing publisher", func(p *Policy) { p.PublicKey = nil }, "TRUST_STATE_INVALID"},
		{"wrong platform", func(p *Policy) { p.Platform = "linux-x64" }, "RELEASE_POLICY_MISMATCH"},
		{"wrong channel", func(p *Policy) { p.Channel = "stable" }, "RELEASE_POLICY_MISMATCH"},
		{"expired", func(p *Policy) { p.Now = m.ExpiresAt }, "RELEASE_TIME_INVALID"},
		{"future", func(p *Policy) { p.Now = m.IssuedAt.Add(-time.Second) }, "RELEASE_TIME_INVALID"},
		{"missing clock", func(p *Policy) { p.Now = time.Time{} }, "TRUST_STATE_INVALID"},
		{"damaged checkpoint", func(p *Policy) { p.Previous.Sequence = 6 }, "TRUST_STATE_INVALID"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := policy
			tc.change(&p)
			_, err := Verify(doc, p)
			expectCode(t, err, tc.code)
		})
	}
}

func TestReplayAndSameSequenceSubstitution(t *testing.T) {
	m, policy, key := fixture(t)
	doc := signed(t, m, key)
	r, err := Verify(doc, policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.Previous = r.Checkpoint()
	if _, err := Verify(doc, policy); err != nil {
		t.Fatal("identical retry should succeed", err)
	}
	m.Version = "0.1.0-alpha.95"
	_, err = Verify(signed(t, m, key), policy)
	expectCode(t, err, "RELEASE_REPLAY_REJECTED")
	m.Sequence--
	_, err = Verify(signed(t, m, key), policy)
	expectCode(t, err, "RELEASE_REPLAY_REJECTED")
	m.Sequence += 2
	if _, err := Verify(signed(t, m, key), policy); err != nil {
		t.Fatal(err)
	}
}

func TestPayloadTamperingAndDomainSeparation(t *testing.T) {
	m, policy, key := fixture(t)
	var envelope Envelope
	if err := json.Unmarshal(signed(t, m, key), &envelope); err != nil {
		t.Fatal(err)
	}
	payload, _ := base64.StdEncoding.DecodeString(envelope.Payload)
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))
	doc, _ := json.Marshal(envelope)
	_, err := Verify(doc, policy)
	expectCode(t, err, "RELEASE_SIGNATURE_INVALID")
	envelope.Payload = base64.StdEncoding.EncodeToString(bytes.ReplaceAll(payload, []byte("alpha.94"), []byte("alpha.95")))
	doc, _ = json.Marshal(envelope)
	_, err = Verify(doc, policy)
	expectCode(t, err, "RELEASE_SIGNATURE_INVALID")
}

func TestRejectAmbiguousOrUnboundedJSON(t *testing.T) {
	m, policy, key := fixture(t)
	doc := signed(t, m, key)
	for _, input := range [][]byte{
		append(append([]byte(nil), doc...), []byte("{}")...),
		bytes.Replace(doc, []byte(`{"schema_version":`), []byte(`{"SCHEMA_VERSION":"other","schema_version":`), 1),
		bytes.Replace(doc, []byte(`{"schema_version":`), []byte(`{"trust_key":"untrusted","schema_version":`), 1),
		bytes.Repeat([]byte(" "), MaxEnvelope+1),
		[]byte(`{"schema_version":` + strings.Repeat("[", 30) + strings.Repeat("]", 30) + "}"),
	} {
		_, err := Verify(input, policy)
		expectCode(t, err, "RELEASE_ENVELOPE_INVALID")
	}
	// Even valid publisher signatures cannot make duplicate metadata acceptable.
	payload, _ := json.Marshal(m)
	payload = bytes.Replace(payload, []byte(`{"schema_version":`), []byte(`{"sequence":8,"schema_version":`), 1)
	envelope := Envelope{EnvelopeSchema, base64.StdEncoding.EncodeToString(payload), base64.StdEncoding.EncodeToString(ed25519.Sign(key, append([]byte(Domain), payload...)))}
	doc, _ = json.Marshal(envelope)
	_, err := Verify(doc, policy)
	expectCode(t, err, "RELEASE_METADATA_INVALID")
}

func TestRejectUnsafeReleaseMetadata(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Metadata)
	}{
		{"traversal", func(m *Metadata) { m.Artifacts[0].Name = "../payload.zip" }},
		{"absolute", func(m *Metadata) { m.Artifacts[0].Name = "C:\\payload.zip" }},
		{"reserved", func(m *Metadata) { m.Artifacts[0].Name = "CON.exe" }},
		{"reserved numbered", func(m *Metadata) { m.Artifacts[0].Name = "LPT1.exe" }},
		{"trailing dot", func(m *Metadata) { m.Artifacts[0].Name = "payload." }},
		{"hidden", func(m *Metadata) { m.Artifacts[0].Name = ".payload" }},
		{"duplicate", func(m *Metadata) { a := m.Artifacts[0]; a.Name = "PAYLOAD.ZIP"; m.Artifacts = append(m.Artifacts, a) }},
		{"zero bytes", func(m *Metadata) { m.Artifacts[0].Size = 0 }},
		{"large", func(m *Metadata) { m.Artifacts[0].Size = MaxArtifact + 1 }},
		{"bad hash", func(m *Metadata) { m.Artifacts[0].SHA256 = "bad" }},
		{"wrong product", func(m *Metadata) { m.Product = "Other" }},
		{"unknown schema", func(m *Metadata) { m.SchemaVersion = "v2" }},
		{"unbounded lifetime", func(m *Metadata) { m.ExpiresAt = m.IssuedAt.Add(91 * 24 * time.Hour) }},
		{"stable prerelease", func(m *Metadata) { m.Channel = "stable" }},
		{"no sequence", func(m *Metadata) { m.Sequence = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _, key := fixture(t)
			tc.change(&m)
			_, err := Sign(m, func(data []byte) ([]byte, error) { return ed25519.Sign(key, data), nil })
			expectCode(t, err, "RELEASE_METADATA_INVALID")
		})
	}
}
