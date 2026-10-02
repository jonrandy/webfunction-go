package webfunction

import "testing"

func TestPrivateFlag(t *testing.T) {
	if !(&Endpoint{Flags: []string{"private"}}).Private() || (&Endpoint{}).Private() {
		t.Error("Endpoint.Private() wrong")
	}
	if !(&Argument{Flags: []string{"required", "private"}}).Private() || (&Argument{Flags: []string{"required"}}).Private() {
		t.Error("Argument.Private() wrong")
	}
	if !(&Attribute{Flags: []string{"private"}}).Private() || (&Attribute{Flags: []string{"nullable"}}).Private() {
		t.Error("Attribute.Private() wrong")
	}
}