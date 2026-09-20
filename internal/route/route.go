// Package route defines internal, address-bearing route candidates.
package route

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"time"
)

// Request binds route acquisition to an already authorized immutable target.
type Request struct {
	SubjectDeviceID     string
	CanonicalTarget     string
	AuthorizationExpiry time.Time
}

// Acquisition owns one temporary candidate and its adapter resources.
type Acquisition interface {
	Candidate() Candidate
	Close() error
}

// Provider acquires one route. It must not silently select another adapter.
type Provider interface {
	Acquire(context.Context, Request) (Acquisition, error)
}

var (
	identifierPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,94}[a-z0-9])?$`)
	addressPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.:-]{0,252}$`)
)

// Candidate is one process-scoped way to reach a Capability. Its endpoint is
// mutable transport state and must never enter a TargetReference.
type Candidate struct {
	ID      string
	Adapter string
	Network string
	Address string
	Port    uint16
}

func (c Candidate) Validate() error {
	if !identifierPattern.MatchString(c.ID) || !identifierPattern.MatchString(c.Adapter) {
		return errors.New("route candidate has an invalid identifier")
	}
	if c.Network != "tcp" {
		return errors.New("route candidate uses an unsupported network")
	}
	validAddress := net.ParseIP(c.Address) != nil ||
		addressPattern.MatchString(c.Address) && !strings.Contains(c.Address, "..")
	if !validAddress || c.Port == 0 {
		return errors.New("route candidate has an invalid endpoint")
	}
	return nil
}
