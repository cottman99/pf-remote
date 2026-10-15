package desktop

import "context"

// PrepareLocalRDPTrust configures only this controller's local RDP publisher.
// It never opens a viewer or makes a network connection.
func PrepareLocalRDPTrust(ctx context.Context) error {
	p, cleanup, err := writeRDPProfile(RDPOptions{Endpoint: "192.0.2.1:3389"})
	if err != nil {
		return err
	}
	defer cleanup()
	return signRDPProfile(ctx, p)
}
