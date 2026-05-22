package controlplane

import "testing"

func TestBaseURLFromWebSocketURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{
			"ws://fluid-controlplane.fluid.svc.cluster.local:4000/v1/probes/websocket",
			"http://fluid-controlplane.fluid.svc.cluster.local:4000",
		},
		{
			"wss://dev.fluid.pub/v1/probes/websocket",
			"https://dev.fluid.pub",
		},
	}
	for _, tc := range tests {
		got, err := BaseURLFromWebSocketURL(tc.in)
		if err != nil {
			t.Fatalf("BaseURLFromWebSocketURL(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("BaseURLFromWebSocketURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
