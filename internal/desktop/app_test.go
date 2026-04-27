package desktop

import (
	"strings"
	"testing"
)

func TestDecodeActionTrimsValues(t *testing.T) {
	request, err := decodeAction(strings.NewReader(`{"endpoint_ip":" 203.0.113.10 ","wstunnel_binary":" /usr/bin/wstunnel "}`))
	if err != nil {
		t.Fatal(err)
	}
	if request.EndpointIP != "203.0.113.10" {
		t.Fatalf("endpoint = %q", request.EndpointIP)
	}
	if request.WSTunnelBinary != "/usr/bin/wstunnel" {
		t.Fatalf("wstunnel binary = %q", request.WSTunnelBinary)
	}
}

func TestDecodeActionAllowsEmptyBody(t *testing.T) {
	request, err := decodeAction(strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if request.EndpointIP != "" || request.WSTunnelBinary != "" {
		t.Fatalf("request = %#v", request)
	}
}
