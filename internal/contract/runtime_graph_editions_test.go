package contract

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Protobuf-ES retains resolved runtime behavior. In particular, edition defaults
// must not accidentally inherit proto2 behavior merely because syntax != proto3.
func TestRuntimeGraphEdition2023DefaultsAndInheritance(t *testing.T) {
	for _, mode := range []string{"defaults", "file overrides", "nearest overrides"} {
		t.Run(mode, func(t *testing.T) {
			set := descriptorSet(nil, false, enumValue("ZERO", 0))
			file := set.File[0]
			file.Syntax = proto.String("editions")
			file.Edition = descriptorpb.Edition_EDITION_2023.Enum()
			file.EnumType[0].Options = nil
			request := file.MessageType[0]
			request.Field = append(request.Field,
				&descriptorpb.FieldDescriptorProto{Name: proto.String("text"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()},
				&descriptorpb.FieldDescriptorProto{Name: proto.String("numbers"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()})
			wantUTF8, wantOpen, wantPacked, wantPresence := true, true, true, "EXPLICIT"
			if mode != "defaults" {
				file.Options = &descriptorpb.FileOptions{Features: &descriptorpb.FeatureSet{
					Utf8Validation: descriptorpb.FeatureSet_NONE.Enum(), EnumType: descriptorpb.FeatureSet_CLOSED.Enum(),
					RepeatedFieldEncoding: descriptorpb.FeatureSet_EXPANDED.Enum(), FieldPresence: descriptorpb.FeatureSet_IMPLICIT.Enum(),
				}}
				wantUTF8, wantOpen, wantPacked, wantPresence = false, false, false, "IMPLICIT"
				// Closed enums require explicit field presence even when the file
				// sets implicit presence for ordinary scalar fields.
				request.Field[0].Options = &descriptorpb.FieldOptions{Features: &descriptorpb.FeatureSet{FieldPresence: descriptorpb.FeatureSet_EXPLICIT.Enum()}}
			}
			if mode == "nearest overrides" {
				request.Options = &descriptorpb.MessageOptions{Features: &descriptorpb.FeatureSet{
					Utf8Validation: descriptorpb.FeatureSet_VERIFY.Enum(), RepeatedFieldEncoding: descriptorpb.FeatureSet_PACKED.Enum(), FieldPresence: descriptorpb.FeatureSet_EXPLICIT.Enum(),
				}}
				file.EnumType[0].Options = &descriptorpb.EnumOptions{Features: &descriptorpb.FeatureSet{EnumType: descriptorpb.FeatureSet_OPEN.Enum()}}
				wantUTF8, wantOpen, wantPacked, wantPresence = true, true, true, "EXPLICIT"
			}
			graph, err := RuntimeGraph(set, "test.S")
			if err != nil {
				t.Fatal(err)
			}
			var fields []map[string]any
			for _, message := range graph["messages"].([]map[string]any) {
				if message["typeName"] == "test.Request" {
					fields = message["fields"].([]map[string]any)
				}
			}
			if len(fields) != 3 {
				t.Fatalf("request fields = %#v", fields)
			}
			if fields[1]["utf8Validation"] != wantUTF8 || fields[1]["presence"] != wantPresence {
				t.Errorf("string field = %#v; want UTF8 %v, presence %s", fields[1], wantUTF8, wantPresence)
			}
			if shape := fields[2]["shape"].(map[string]any); shape["packed"] != wantPacked {
				t.Errorf("repeated shape = %#v; want packed %v", shape, wantPacked)
			}
			if enums := graph["enums"].([]map[string]any); len(enums) != 1 || enums[0]["open"] != wantOpen {
				t.Errorf("enum graph = %#v; want open %v", enums, wantOpen)
			}
		})
	}
}
