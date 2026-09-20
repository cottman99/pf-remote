package route

import "testing"

func TestCandidateValidate(t *testing.T) {
	valid := Candidate{ID: "route-local-1", Adapter: "local", Network: "tcp", Address: "127.0.0.1", Port: 2222}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	ipv6 := valid
	ipv6.Address = "::1"
	if err := ipv6.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Candidate){
		func(value *Candidate) { value.ID = "../route" },
		func(value *Candidate) { value.Adapter = "FRP" },
		func(value *Candidate) { value.Network = "udp" },
		func(value *Candidate) { value.Address = "-oProxyCommand=bad" },
		func(value *Candidate) { value.Address = "example..com" },
		func(value *Candidate) { value.Port = 0 },
	} {
		value := valid
		mutate(&value)
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid candidate passed: %#v", value)
		}
	}
}
