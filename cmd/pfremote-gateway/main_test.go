package main

import "testing"

func TestValidateTLSOptions(t *testing.T) {
	tests := []struct {
		name, listen, certificate, key string
		wantErr                        bool
	}{
		{name: "loopback plaintext", listen: "127.0.0.1:47832"},
		{name: "IPv6 loopback plaintext", listen: "[::1]:47832"},
		{name: "remote TLS", listen: "100.64.0.2:47832", certificate: "gateway.crt", key: "gateway.key"},
		{name: "remote plaintext", listen: "100.64.0.2:47832", wantErr: true},
		{name: "certificate only", listen: "127.0.0.1:47832", certificate: "gateway.crt", wantErr: true},
		{name: "key only", listen: "127.0.0.1:47832", key: "gateway.key", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateTLSOptions(test.listen, test.certificate, test.key); (err != nil) != test.wantErr {
				t.Fatalf("validateTLSOptions() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
