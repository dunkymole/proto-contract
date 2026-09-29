package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCheckReportJSONIsStableForCIConsumers(t *testing.T) {
	want := contractCheckReport{
		API: "demo.echo", Status: "changed", Version: "1.0.0",
		NextVersion: "1.1.0", Bump: "minor", Changes: []string{"added rpc EchoService.Watch"},
	}
	encoded, err := encodeCheckReport(want)
	if err != nil {
		t.Fatal(err)
	}
	var got contractCheckReport
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON round-trip = %#v, want %#v", got, want)
	}
}
