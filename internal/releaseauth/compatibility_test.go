package releaseauth

import "testing"

func TestSignedCompatibilityIsRequiredAndImmutable(t *testing.T) {
	m, p, key := fixture(t)
	v1, err := Verify(signed(t, m, key), p)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Compatible(1, 1) {
		t.Fatal("v1 release permitted automatic activation")
	}
	m.SchemaVersion = MetadataSchemaV2
	m.Compatibility = &Compatibility{DataEpoch: 1, ProtocolMin: 1, ProtocolMax: 2}
	r, err := Verify(signed(t, m, key), p)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Compatible(1, 1) || !r.Compatible(1, 2) || r.Compatible(2, 1) || r.Compatible(1, 3) || r.Compatible(0, 1) {
		t.Fatal("incorrect compatibility gate")
	}
	copy := r.Metadata()
	copy.Compatibility.DataEpoch = 2
	if !r.Compatible(1, 1) {
		t.Fatal("caller changed signed compatibility")
	}
	m.SchemaVersion = MetadataSchema
	if validMetadata(m) {
		t.Fatal("v2 fields accepted under v1 schema")
	}
	m.SchemaVersion = MetadataSchemaV2
	m.Compatibility = nil
	if validMetadata(m) {
		t.Fatal("missing v2 compatibility accepted")
	}
	m.Compatibility = &Compatibility{DataEpoch: 1, ProtocolMin: 2, ProtocolMax: 1}
	if validMetadata(m) {
		t.Fatal("inverted protocol range accepted")
	}
}
