// Package tailscale verifies that a discovered endpoint still belongs to the
// expected Tailscale node before a route candidate is exposed.
package tailscale

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os/exec"
	"strings"
)

type StatusCommand interface {
	StatusJSON(context.Context) ([]byte, error)
}

type CLI struct {
	Executable string
}

func (c CLI) StatusJSON(ctx context.Context) ([]byte, error) {
	executable := c.Executable
	if executable == "" {
		executable = "tailscale"
	}
	output, err := exec.CommandContext(ctx, executable, "status", "--json").Output()
	if err != nil || len(output) > 1<<20 {
		return nil, errors.New("Tailscale status is unavailable")
	}
	return output, nil
}

type Verifier struct {
	Command StatusCommand
}

// Directory is one bounded, read-only Tailscale identity snapshot. Its peer
// details remain private and cannot be serialized through exported fields.
type Directory struct {
	peers []peer
}

type status struct {
	Peer map[string]peer `json:"Peer"`
}

type peer struct {
	ID           string   `json:"ID"`
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Online       bool     `json:"Online"`
}

func (v Verifier) VerifyPeer(ctx context.Context, address, expectedNodeID string) error {
	directory, err := LoadDirectory(ctx, v.Command)
	if err != nil {
		return err
	}
	return directory.VerifyPeer(ctx, address, expectedNodeID)
}

func (v Verifier) ResolvePeer(ctx context.Context, address string) (string, error) {
	directory, err := LoadDirectory(ctx, v.Command)
	if err != nil {
		return "", err
	}
	return directory.ResolvePeer(ctx, address)
}

func LoadDirectory(ctx context.Context, command StatusCommand) (Directory, error) {
	if command == nil {
		command = CLI{}
	}
	raw, err := command.StatusJSON(ctx)
	if err != nil {
		return Directory{}, err
	}
	var value status
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	// Tailscale status adds fields over time, so decode without strict unknown
	// rejection after checking the bounded payload.
	if err := decoder.Decode(&value); err != nil {
		return Directory{}, errors.New("Tailscale status is invalid")
	}
	result := Directory{peers: make([]peer, 0, len(value.Peer))}
	for _, candidate := range value.Peer {
		result.peers = append(result.peers, candidate)
	}
	return result, nil
}

func (d Directory) VerifyPeer(_ context.Context, address, expectedNodeID string) error {
	for _, candidate := range d.peers {
		if candidate.Online && candidate.ID == expectedNodeID && matchesAddress(candidate, address) {
			return nil
		}
	}
	return errors.New("Tailscale node identity does not match")
}

func (d Directory) ResolvePeer(_ context.Context, address string) (string, error) {
	matched := ""
	for _, candidate := range d.peers {
		if !candidate.Online || candidate.ID == "" || !matchesAddress(candidate, address) {
			continue
		}
		if matched != "" && matched != candidate.ID {
			return "", errors.New("Tailscale endpoint identity is ambiguous")
		}
		matched = candidate.ID
	}
	if matched == "" {
		return "", errors.New("Tailscale endpoint identity is unavailable")
	}
	return matched, nil
}

func matchesAddress(value peer, address string) bool {
	want := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(address)), ".")
	if strings.TrimSuffix(strings.ToLower(value.DNSName), ".") == want || strings.ToLower(value.HostName) == want {
		return true
	}
	wantIP := net.ParseIP(address)
	for _, candidate := range value.TailscaleIPs {
		if parsed := net.ParseIP(candidate); wantIP != nil && parsed != nil && wantIP.Equal(parsed) {
			return true
		}
	}
	return false
}
