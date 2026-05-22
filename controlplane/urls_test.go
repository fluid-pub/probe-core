package controlplane

import "testing"

func TestParseBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"http://fluid-controlplane.fluid.svc.cluster.local:4000", "http://fluid-controlplane.fluid.svc.cluster.local:4000"},
		{"https://dev.fluid.pub/", "https://dev.fluid.pub"},
	}
	for _, tc := range tests {
		got, err := ParseBaseURL(tc.in)
		if err != nil {
			t.Fatalf("ParseBaseURL(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseBaseURL_rejectsWebSocketScheme(t *testing.T) {
	_, err := ParseBaseURL("wss://dev.fluid.pub/v1/probes/websocket")
	if err == nil {
		t.Fatal("expected error for ws/wss scheme")
	}
}
