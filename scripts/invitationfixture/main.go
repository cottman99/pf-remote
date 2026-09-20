// Command invitationfixture creates a short-lived invitation only for the
// isolated live-reload verification. It is not a product setup surface.
package main

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/cottman99/pf-remote/internal/capabilitysync"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/localapi"
)

func main() {
	if len(os.Args) != 2 {
		panic("output path required")
	}
	identityPath, err := identity.DefaultPath()
	if err != nil {
		panic(err)
	}
	store, err := identity.NewStore(identityPath)
	if err != nil {
		panic(err)
	}
	signer, err := store.LoadOrCreate()
	if err != nil {
		panic(err)
	}
	// Listing may include bounded route-health checks. Keep this fixture above
	// that product budget so scheduler jitter cannot turn a healthy daemon into
	// an invitation failure.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	catalog, err := localapi.NewClient().List(ctx)
	if err != nil {
		panic(err)
	}
	invitation, err := capabilitysync.SignInvitation("http://127.0.0.1:47839", catalog.FabricID, "owner", "isolated-live-reload-check", time.Now().Add(time.Hour), signer)
	if err != nil {
		panic(err)
	}
	payload, err := json.Marshal(invitation)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Args[1], payload, 0o600); err != nil {
		panic(err)
	}
}
