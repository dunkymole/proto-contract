package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func lineageSnapshot(t *testing.T, version string, extra bool) *Snapshot {
	t.Helper()
	set := descriptorSet(nil, false, enumValue("ZERO", 0))
	if extra {
		set.File[0].MessageType[0].Field = append(set.File[0].MessageType[0].Field, descriptorpbField("extra", 2))
	}
	s, err := Build(set, "test.S", "test/api:v1", version)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func descriptorpbField(name string, number int32) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(number), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()}
}

func persistLock(t *testing.T, snapshot *Snapshot) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contract.json")
	if err := Write(path, snapshot); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadValidatesDigestAndGraph(t *testing.T) {
	base := lineageSnapshot(t, "1.0.0", false)
	path := persistLock(t, base)
	if got, err := Read(path); err != nil || got.Digest != base.Digest {
		t.Fatalf("Read valid lock: got %v, %v", got, err)
	}

	cases := []struct {
		name   string
		mutate func(*Snapshot)
	}{
		{"digest", func(s *Snapshot) { s.Digest = "sha256:" + strings.Repeat("0", 64) }},
		{"duplicate message", func(s *Snapshot) { s.Messages = append(s.Messages, s.Messages[0]) }},
		{"duplicate method", func(s *Snapshot) { s.Service.Methods = append(s.Service.Methods, s.Service.Methods[0]) }},
		{"duplicate field number", func(s *Snapshot) {
			f := s.Messages[0].Fields[0]
			f.Name = "other"
			s.Messages[0].Fields = append(s.Messages[0].Fields, f)
		}},
		{"dangling request", func(s *Snapshot) { s.Service.Methods[0].Input = "test.Missing" }},
		{"dangling type", func(s *Snapshot) {
			s.Messages[0].Fields[0].Type = "TYPE_MESSAGE"
			s.Messages[0].Fields[0].TypeName = "test.Missing"
		}},
		{"invalid api", func(s *Snapshot) { s.API = "api@fork" }},
		{"noncanonical version", func(s *Snapshot) { s.Version = "+1.0.0" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copy := cloneSnapshot(t, base)
			tc.mutate(copy)
			if tc.name != "digest" {
				copy.Digest, _ = calculateDigest(copy)
			}
			b, err := json.Marshal(copy)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "invalid.json")
			if err := os.WriteFile(path, b, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(path); err == nil {
				t.Fatal("accepted malformed lock")
			}
		})
	}
}

func TestReadRejectsOversizedAndTrailingLocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "too-large.json")
	if err := os.WriteFile(path, make([]byte, MaxLockBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "maximum size") {
		t.Fatalf("oversized lock error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{} {}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil {
		t.Fatal("accepted trailing JSON data")
	}
}

func cloneSnapshot(t *testing.T, snapshot *Snapshot) *Snapshot {
	t.Helper()
	b, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var clone Snapshot
	if err := json.Unmarshal(b, &clone); err != nil {
		t.Fatal(err)
	}
	return &clone
}

func releaseHistory(snapshots ...*Snapshot) *ReleaseManifest {
	history := &ReleaseManifest{Format: 1, Releases: []Release{}}
	for _, snapshot := range snapshots {
		history.Releases = append(history.Releases, releaseFor(snapshot))
	}
	return history
}

func TestReleaseTransitionAndImmutableHistory(t *testing.T) {
	base := lineageSnapshot(t, "1.0.0", false)
	additive := lineageSnapshot(t, "1.1.0", true)
	baseHistory := releaseHistory(base)
	if err := CheckReleaseTransition(base, additive, baseHistory, releaseHistory(base, additive)); err != nil {
		t.Fatalf("legal additive release rejected: %v", err)
	}

	higher := lineageSnapshot(t, "4.0.0", true)
	if err := CheckReleaseTransition(base, higher, baseHistory, releaseHistory(base, higher)); err != nil {
		t.Fatalf("higher semantic bump rejected: %v", err)
	}
	fork := lineageSnapshot(t, "1.0.0", true)
	if err := CheckReleaseTransition(base, fork, baseHistory, releaseHistory(base)); err == nil {
		t.Fatal("accepted same-version schema fork")
	}
	rollback := lineageSnapshot(t, "0.9.0", false)
	if err := CheckReleaseTransition(base, rollback, baseHistory, releaseHistory(base)); err == nil {
		t.Fatal("accepted version rollback")
	}
	insufficient := lineageSnapshot(t, "1.0.1", true)
	if err := CheckReleaseTransition(base, insufficient, baseHistory, releaseHistory(base, insufficient)); err == nil {
		t.Fatal("accepted structural change with patch-only version bump")
	}
	if err := CheckReleaseTransition(base, additive, baseHistory, releaseHistory(base, higher)); err == nil {
		t.Fatal("accepted manifest assignment for a different proposed lock")
	}
}

func TestReleaseTransitionRejectsStaleTrustedBaselineAndUnrelatedAppend(t *testing.T) {
	initial := lineageSnapshot(t, "1.0.0", false)
	latest := lineageSnapshot(t, "1.1.0", true)
	staleCandidate := lineageSnapshot(t, "1.2.0", false)
	if err := CheckReleaseTransition(initial, staleCandidate, releaseHistory(initial, latest), releaseHistory(initial, latest, staleCandidate)); err == nil {
		t.Fatal("accepted stale base lock")
	}
	otherAPI := Release{API: "other.api", Version: "1.0.0", Digest: latest.Digest}
	proposedHistory := releaseHistory(latest)
	proposedHistory.Releases = append(proposedHistory.Releases, otherAPI)
	if err := CheckReleaseTransition(latest, latest, releaseHistory(latest), proposedHistory); err == nil {
		t.Fatal("accepted unrelated history append")
	}
}

func TestReleaseHistoryBootstrapsTrustedBase(t *testing.T) {
	base := lineageSnapshot(t, "1.0.0", false)
	proposed := lineageSnapshot(t, "1.1.0", true)
	empty := &ReleaseManifest{Format: 1, Releases: []Release{}}
	if err := CheckReleaseTransition(base, proposed, empty, releaseHistory(base, proposed)); err != nil {
		t.Fatalf("bootstrap release rejected: %v", err)
	}
	if err := CheckReleaseTransition(base, proposed, empty, releaseHistory(proposed)); err == nil {
		t.Fatal("bootstrap did not require trusted base assignment")
	}
}

func TestDescriptorDefaultsPresenceAndMapEntryValidation(t *testing.T) {
	validDefaults := []Field{
		{Number: 1, Name: "int_value", Type: "TYPE_INT32", Label: "LABEL_OPTIONAL", HasDefault: true, DefaultValue: "2147483647"},
		{Number: 2, Name: "bytes_value", Type: "TYPE_BYTES", Label: "LABEL_OPTIONAL", HasDefault: true, DefaultValue: `\377`},
	}
	snapshot := &Snapshot{Format: 2, API: "test.api", Version: "1.0.0", Service: Service{Name: "test.S", Methods: []Method{{Name: "Call", Input: "test.M", Output: "test.M"}}}, Messages: []Message{{Name: "test.M", Syntax: "proto2", Fields: validDefaults}}, Enums: []Enum{}}
	snapshot.Digest, _ = calculateDigest(snapshot)
	if err := Validate(snapshot); err != nil {
		t.Fatalf("valid proto defaults rejected: %v", err)
	}
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.Messages[0].Fields[0].DefaultValue = "2147483648" },
		func(s *Snapshot) { s.Messages[0].Fields[1].DefaultValue = `\xGZ` },
		func(s *Snapshot) { s.Messages[0].Fields[0].Label = "LABEL_REPEATED" },
		func(s *Snapshot) { s.Messages[0].Syntax = "proto3" },
		func(s *Snapshot) { s.Messages[0].Fields[0].Proto3Optional = true },
	} {
		candidate := cloneSnapshot(t, snapshot)
		mutate(candidate)
		candidate.Digest, _ = calculateDigest(candidate)
		if err := Validate(candidate); err == nil {
			t.Fatal("accepted inconsistent default/presence data")
		}
	}
	mapMessage := Message{Name: "test.M.Entry", Syntax: "proto3", MapEntry: true, Fields: []Field{{Number: 1, Name: "key", Type: "TYPE_FLOAT", Label: "LABEL_OPTIONAL"}, {Number: 2, Name: "value", Type: "TYPE_STRING", Label: "LABEL_OPTIONAL"}}}
	badMap := &Snapshot{Format: 2, API: "test.api", Version: "1.0.0", Service: Service{Name: "test.S", Methods: []Method{{Name: "Call", Input: "test.M", Output: "test.M"}}}, Messages: []Message{{Name: "test.M", Syntax: "proto3", Fields: []Field{}}, mapMessage}, Enums: []Enum{}}
	badMap.Digest, _ = calculateDigest(badMap)
	if err := Validate(badMap); err == nil {
		t.Fatal("accepted a malformed map-entry key type")
	}
	mapMessage.Fields[0].Type = "TYPE_INT32"
	validMap := &Snapshot{Format: 2, API: "test.api", Version: "1.0.0", Service: Service{Name: "test.S", Methods: []Method{{Name: "Call", Input: "test.M", Output: "test.M"}}}, Messages: []Message{{Name: "test.M", Syntax: "proto3", Fields: []Field{}}, mapMessage}, Enums: []Enum{}}
	validMap.Digest, _ = calculateDigest(validMap)
	if err := Validate(validMap); err != nil {
		t.Fatalf("valid map-entry descriptor rejected: %v", err)
	}
	validMap.Service.Methods[0].Input = mapMessage.Name
	validMap.Digest, _ = calculateDigest(validMap)
	if err := Validate(validMap); err == nil {
		t.Fatal("accepted a synthetic map-entry as an RPC input")
	}
}

func TestProto3EnumDefaultUsesDeclarationIdentityNotNumericSort(t *testing.T) {
	set := descriptorSet(nil, false, enumValue("ZERO", 0), enumValue("NEGATIVE", -1))
	set.File[0].Syntax = proto.String("proto3")
	snapshot, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatalf("valid proto3 enum with negative nondefault value rejected: %v", err)
	}
	if snapshot.Enums[0].Values[0].Name != "NEGATIVE" || snapshot.Enums[0].DefaultName != "ZERO" {
		t.Fatalf("unexpected sorted/default enum metadata: %+v", snapshot.Enums[0])
	}
}

func TestEnumRequiresExplicitFirstDeclaredDefaultIdentity(t *testing.T) {
	set := descriptorSet(nil, false, enumValue("ZERO", 0), enumValue("ALIAS", 0))
	snapshot, err := Build(set, "test.S", "test", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "ALIAS"} {
		candidate := cloneSnapshot(t, snapshot)
		candidate.Enums[0].DefaultName = name
		candidate.Digest, _ = calculateDigest(candidate)
		if err := Validate(candidate); err == nil {
			t.Fatalf("accepted enum default identity %q that is not the first declaration", name)
		}
	}
}

func TestStrictJSONRejectsDuplicateKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "duplicate.json")
	if err := os.WriteFile(path, []byte(`{"format":2,"format":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate lock key error = %v", err)
	}
	shadowPath := filepath.Join(t.TempDir(), "case-shadow.json")
	if err := os.WriteFile(shadowPath, []byte(`{"format":2,"API":"first.api","api":"review.api"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(shadowPath); err == nil || !strings.Contains(err.Error(), "canonical lowercase spelling") {
		t.Fatalf("case-variant shadow lock key error = %v", err)
	}
	manifestPath := filepath.Join(t.TempDir(), "releases.json")
	if err := os.WriteFile(manifestPath, []byte(`{"format":1,"format":1,"releases":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReleaseManifest(manifestPath); err == nil || !strings.Contains(err.Error(), "duplicate JSON object key") {
		t.Fatalf("duplicate manifest key error = %v", err)
	}
	manifestShadowPath := filepath.Join(t.TempDir(), "manifest-case-shadow.json")
	if err := os.WriteFile(manifestShadowPath, []byte(`{"format":1,"releases":[],"Releases":[{"api":"test.api","version":"1.0.0","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReleaseManifest(manifestShadowPath); err == nil || !strings.Contains(err.Error(), "canonical lowercase spelling") {
		t.Fatalf("case-variant shadow manifest key error = %v", err)
	}
}
