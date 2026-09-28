package contract

import "testing"

func TestNextVersion(t *testing.T) {
	for _, tc := range []struct {
		in   string
		bump Bump
		want string
	}{{"1.2.3", None, "1.2.3"}, {"1.2.3", Minor, "1.3.0"}, {"1.2.3", Major, "2.0.0"}} {
		got, err := NextVersion(tc.in, tc.bump)
		if err != nil || got != tc.want {
			t.Fatalf("NextVersion(%q,%s)=%q,%v", tc.in, tc.bump, got, err)
		}
	}
}

func TestCompareFieldAddition(t *testing.T) {
	a := &Snapshot{Service: Service{Name: "x", Methods: []Method{{Name: "Call", Input: "Req", Output: "Res"}}}, Messages: []Message{{Name: "Req", Fields: []Field{{Number: 1, Name: "value"}}}, {Name: "Res"}}}
	b := &Snapshot{Service: a.Service, Messages: []Message{{Name: "Req", Fields: []Field{{Number: 1, Name: "value"}, {Number: 2, Name: "extra"}}}, {Name: "Res"}}}
	bump, _ := Compare(a, b)
	if bump != Minor {
		t.Fatalf("got %s", bump)
	}
	b.Messages[0].Fields[0].Type = "TYPE_INT32"
	bump, _ = Compare(a, b)
	if bump != Major {
		t.Fatalf("got %s", bump)
	}
}
