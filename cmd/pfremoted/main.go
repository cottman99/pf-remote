package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cottman99/pf-remote/internal/actions"
	"github.com/cottman99/pf-remote/internal/activity"
	"github.com/cottman99/pf-remote/internal/capabilitysync"
	"github.com/cottman99/pf-remote/internal/catalog"
	"github.com/cottman99/pf-remote/internal/desktop"
	"github.com/cottman99/pf-remote/internal/desktopcredential"
	"github.com/cottman99/pf-remote/internal/desktopruntime"
	"github.com/cottman99/pf-remote/internal/enrollment"
	desktopexecutor "github.com/cottman99/pf-remote/internal/executor/desktop"
	"github.com/cottman99/pf-remote/internal/identity"
	"github.com/cottman99/pf-remote/internal/localapi"
	"github.com/cottman99/pf-remote/internal/migration"
	"github.com/cottman99/pf-remote/internal/powerresume"
	"github.com/cottman99/pf-remote/internal/releaseauth"
	"github.com/cottman99/pf-remote/internal/route"
	tailscaleroute "github.com/cottman99/pf-remote/internal/route/tailscale"
	"github.com/cottman99/pf-remote/internal/shellbinding"
	"github.com/cottman99/pf-remote/internal/state"
	"github.com/cottman99/pf-remote/pkg/contracts"
)

type connectionSettings struct {
	GatewayURL    string
	GatewayCAPEM  string
	Pull          bool
	FabricID      string
	OwnerDeviceID string
}

func loadConnectionSettings() (connectionSettings, error) {
	settings := connectionSettings{
		GatewayURL: strings.TrimSpace(os.Getenv("PFREMOTE_GATEWAY_URL")),
		Pull:       strings.EqualFold(strings.TrimSpace(os.Getenv("PFREMOTE_GATEWAY_OWNER_SYNC")), "true"),
	}
	configPath, err := capabilitysync.DefaultConfigPath()
	if err != nil {
		return connectionSettings{}, err
	}
	connectionConfig, err := capabilitysync.LoadConfig(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return connectionSettings{}, err
	}
	return connectionSettings{
		GatewayURL:    connectionConfig.GatewayURL,
		GatewayCAPEM:  connectionConfig.GatewayCAPEM,
		Pull:          connectionConfig.Pull,
		FabricID:      connectionConfig.FabricID,
		OwnerDeviceID: connectionConfig.OwnerDeviceID,
	}, nil
}

func (s connectionSettings) fingerprint() string {
	caDigest := sha256.Sum256([]byte(s.GatewayCAPEM))
	return fmt.Sprintf("%s\x00%t\x00%s\x00%s\x00%x", s.GatewayURL, s.Pull, s.FabricID, s.OwnerDeviceID, caDigest)
}

func (s connectionSettings) synchronizer() capabilitysync.Synchronizer {
	return capabilitysync.Synchronizer{
		Gateway:               s.gatewayClient(),
		Pull:                  s.Pull,
		ExpectedFabricID:      s.FabricID,
		ExpectedOwnerDeviceID: s.OwnerDeviceID,
	}
}

func (s connectionSettings) gatewayClient() enrollment.Client {
	return enrollment.Client{BaseURL: s.GatewayURL, HTTPClient: capabilitysync.GatewayHTTPClient(s.GatewayCAPEM)}
}

func connectionSyncDue(settingsChanged, pull, retry bool, lastPull, now time.Time) bool {
	return settingsChanged || (pull && (retry || now.Sub(lastPull) >= 30*time.Second))
}

type snapshotCommitter interface {
	CommitSnapshot(context.Context, state.Snapshot) error
}

func commitImportedSnapshot(ctx context.Context, store snapshotCommitter, snapshot state.Snapshot, imported int) error {
	if imported == 0 {
		return nil
	}
	return store.CommitSnapshot(ctx, snapshot)
}

func main() {
	identityPath, err := identity.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not locate protected identity storage:", err)
		os.Exit(1)
	}
	identityStore, err := identity.NewStore(identityPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not initialize identity protection:", err)
		os.Exit(1)
	}
	deviceIdentity, err := identityStore.LoadOrCreate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not load its protected Device identity:", err)
		os.Exit(1)
	}
	statePath, err := state.DefaultPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not locate protected state storage:", err)
		os.Exit(1)
	}
	stateStore, err := state.Open(statePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not initialize protected state storage:", err)
		os.Exit(1)
	}
	defer stateStore.Close()
	var desktopCredentials *desktopcredential.Store
	if runtime.GOOS == "windows" {
		credentialRoot, credentialPathErr := desktopcredential.DefaultRoot()
		if credentialPathErr != nil {
			fmt.Fprintln(os.Stderr, "PF Remote daemon could not locate protected Desktop credential storage.")
			os.Exit(1)
		}
		desktopCredentials, err = desktopcredential.New(credentialRoot)
		if err != nil {
			fmt.Fprintln(os.Stderr, "PF Remote daemon could not initialize protected Desktop credential storage.")
			os.Exit(1)
		}
	}
	var desktopConfig *desktopruntime.Config
	loadedDesktopConfig, loadDesktopErr := desktopruntime.Load(desktopruntime.PathForState(statePath))
	if loadDesktopErr == nil {
		loadedDesktopConfig = loadedDesktopConfig.WithTailscaleVerifier(tailscaleroute.Verifier{})
		desktopConfig = &loadedDesktopConfig
	} else if !errors.Is(loadDesktopErr, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not load Desktop settings:", loadDesktopErr)
		os.Exit(1)
	}
	var desktopProfiles *desktopruntime.Profiles
	loadedDesktopProfiles, loadProfilesErr := desktopruntime.LoadProfiles(desktopruntime.ProfilesPathForState(statePath))
	if loadProfilesErr == nil {
		desktopProfiles = &loadedDesktopProfiles
	} else if !errors.Is(loadProfilesErr, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not load Desktop profiles:", loadProfilesErr)
		os.Exit(1)
	}
	currentSnapshot, recovered, err := stateStore.LoadLatestValidSnapshot(context.Background())
	if errors.Is(err, state.ErrNoSnapshot) {
		if err := stateStore.CommitSnapshot(context.Background(), state.SyntheticSnapshot(deviceIdentity.DeviceID(), time.Now())); err != nil {
			fmt.Fprintln(os.Stderr, "PF Remote daemon could not initialize its state snapshot:", err)
			os.Exit(1)
		}
		currentSnapshot, recovered, err = stateStore.LoadLatestValidSnapshot(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not load a valid state snapshot:", err)
		os.Exit(1)
	}
	if recovered {
		fmt.Fprintln(os.Stderr, "PF Remote daemon recovered the last valid committed state snapshot.")
	}
	if hostKeys, discoverErr := shellbinding.DiscoverSystemHostKeys(); discoverErr == nil && len(hostKeys) > 0 {
		updated, bound, bindErr := state.BindLocalShellCapabilities(currentSnapshot, deviceIdentity, hostKeys)
		if bindErr != nil {
			fmt.Fprintln(os.Stderr, "PF Remote daemon could not confirm the local Shell identity.")
			os.Exit(1)
		}
		if bound > 0 {
			if commitErr := stateStore.CommitSnapshot(context.Background(), updated); commitErr != nil {
				fmt.Fprintln(os.Stderr, "PF Remote daemon could not save the confirmed local Shell identity.")
				os.Exit(1)
			}
			currentSnapshot = updated
		}
	}
	var capabilitySynchronizer *capabilitysync.Synchronizer
	var capabilitySyncMu sync.RWMutex
	capabilitySyncCheck := contracts.Check{Name: "connection-service", Status: "skip", Summary: "Connection service is not configured"}
	setCapabilitySyncCheck := func(status, summary string) {
		capabilitySyncMu.Lock()
		capabilitySyncCheck = contracts.Check{Name: "connection-service", Status: status, Summary: summary}
		capabilitySyncMu.Unlock()
	}
	getCapabilitySyncCheck := func() contracts.Check {
		capabilitySyncMu.RLock()
		defer capabilitySyncMu.RUnlock()
		return capabilitySyncCheck
	}
	initialConnectionSettings, connectionSettingsErr := loadConnectionSettings()
	if connectionSettingsErr != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not load protected connection service settings.")
		os.Exit(1)
	}
	if initialConnectionSettings.GatewayURL != "" {
		configuredSynchronizer := initialConnectionSettings.synchronizer()
		capabilitySynchronizer = &configuredSynchronizer
		syncContext, cancelSync := context.WithTimeout(context.Background(), 5*time.Second)
		if synced, result, syncErr := capabilitySynchronizer.Sync(syncContext, currentSnapshot, deviceIdentity); syncErr == nil {
			if commitErr := commitImportedSnapshot(context.Background(), stateStore, synced, result.Imported); commitErr != nil {
				setCapabilitySyncCheck("pending", "Connection service will retry after local state is writable")
			} else {
				setCapabilitySyncCheck("pass", "Connection service synchronized signed capabilities")
				if result.Imported > 0 {
					currentSnapshot = synced
				}
			}
		} else {
			setCapabilitySyncCheck("pending", "Connection service will retry in the background")
		}
		cancelSync()
	}
	explicitLegacyPath := strings.TrimSpace(os.Getenv("PFREMOTE_LEGACY_CENTER_CATALOG"))
	activationPath := migration.LegacyActivationPathForState(statePath)
	legacyBindingsPath := migration.LegacyDeviceBindingsPathForState(statePath)
	legacyDeviceStatePath := migration.LegacyDeviceStatePathForState(statePath)
	managedTargetsPath := filepath.Join(filepath.Dir(statePath), "managed-targets-v1.json")
	managedShellClaimsPath := filepath.Join(filepath.Dir(statePath), "managed-shell-claims-v1.json")
	var legacyMu sync.Mutex
	var legacyCandidate *migration.LegacyCandidate
	loadLegacyCandidate := func() (*migration.LegacyCandidate, error) {
		legacyMu.Lock()
		defer legacyMu.Unlock()
		legacyPath := explicitLegacyPath
		if legacyPath == "" {
			enabled, enableErr := migration.LegacyCenterEnabled(activationPath)
			if errors.Is(enableErr, os.ErrNotExist) {
				legacyCandidate = nil
				return nil, nil
			}
			if enableErr != nil {
				return nil, errors.New("existing setup activation is invalid")
			}
			if !enabled {
				legacyCandidate = nil
				return nil, nil
			}
			var pathErr error
			legacyPath, pathErr = migration.DefaultLegacyCenterCatalogPath()
			if pathErr != nil {
				return nil, pathErr
			}
		}
		if legacyCandidate != nil {
			return legacyCandidate, nil
		}
		legacySource, loadErr := migration.LoadLegacyCenterFile(legacyPath)
		if loadErr != nil {
			return nil, loadErr
		}
		var tailscaleIdentityResolved bool
		if directory, directoryErr := tailscaleroute.LoadDirectory(context.Background(), nil); directoryErr == nil {
			tailscaleIdentityResolved = migration.ResolveLegacyTailscaleIdentities(context.Background(), &legacySource, directory) > 0
		}
		var candidate migration.LegacyCandidate
		var projectErr error
		if tailscaleIdentityResolved {
			candidate, projectErr = migration.ProjectLegacyCenterCandidateWithTailscale(legacySource, deviceIdentity.DeviceID(), time.Now(), tailscaleroute.Verifier{})
		} else {
			candidate, projectErr = migration.ProjectLegacyCenterCandidate(legacySource, deviceIdentity.DeviceID(), time.Now())
		}
		if projectErr != nil {
			return nil, projectErr
		}
		if info, statErr := os.Lstat(managedTargetsPath); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return nil, errors.New("managed computer settings are unsafe")
			}
			overlaySource, overlayErr := migration.LoadLegacyCenterFile(managedTargetsPath)
			if overlayErr != nil {
				return nil, errors.New("managed computer settings are invalid")
			}
			if directory, directoryErr := tailscaleroute.LoadDirectory(context.Background(), nil); directoryErr == nil {
				migration.ResolveLegacyTailscaleIdentities(context.Background(), &overlaySource, directory)
			}
			candidate, projectErr = migration.ProjectLegacyCenterOverlay(candidate, overlaySource, deviceIdentity.DeviceID(), time.Now(), tailscaleroute.Verifier{})
			if projectErr != nil {
				return nil, errors.New("managed computer settings conflict with the existing list")
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, errors.New("managed computer settings could not be inspected")
		}
		if overrides, stateErr := migration.LoadLegacyDeviceStateOverrides(legacyDeviceStatePath); stateErr == nil {
			candidate, projectErr = migration.ApplyLegacyDeviceStateOverrides(candidate, overrides)
			if projectErr != nil {
				return nil, projectErr
			}
		} else if !errors.Is(stateErr, os.ErrNotExist) {
			return nil, errors.New("existing computer availability state is invalid")
		}
		if bindings, bindingsErr := migration.LoadLegacyDeviceBindings(legacyBindingsPath); bindingsErr == nil {
			candidate, projectErr = migration.ApplyLegacyDeviceBindings(candidate, bindings)
			if projectErr != nil {
				return nil, projectErr
			}
		} else if !errors.Is(bindingsErr, os.ErrNotExist) {
			return nil, errors.New("existing computer identity bindings are invalid")
		}
		if claims, claimsErr := capabilitysync.LoadClaimsCache(managedShellClaimsPath); claimsErr == nil {
			candidate.Snapshot, _, projectErr = state.ApplyShellCapabilityClaims(candidate.Snapshot, claims.FabricID, claims.DirectoryVersion, claims.Capabilities)
			if projectErr != nil {
				return nil, errors.New("managed Shell confirmations do not match the existing list")
			}
		} else if !errors.Is(claimsErr, os.ErrNotExist) {
			return nil, errors.New("managed Shell confirmations are invalid")
		}
		if capabilitySynchronizer != nil && capabilitySynchronizer.Pull {
			syncContext, cancelSync := context.WithTimeout(context.Background(), 5*time.Second)
			synced, _, syncErr := capabilitySynchronizer.Sync(syncContext, candidate.Snapshot, deviceIdentity)
			cancelSync()
			if syncErr == nil {
				candidate.Snapshot = synced
			}
		}
		legacyCandidate = &candidate
		return legacyCandidate, nil
	}
	if explicitLegacyPath != "" {
		if _, loadErr := loadLegacyCandidate(); loadErr != nil {
			fmt.Fprintln(os.Stderr, "PF Remote daemon could not align the selected existing setup:", loadErr)
			os.Exit(1)
		}
	}

	listener, endpoint, err := localapi.Listen()
	if err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon could not open protected local IPC:", err)
		os.Exit(1)
	}
	fmt.Println("PF Remote daemon development slice listening on protected IPC:", endpoint)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	updates := backgroundUpdates(statePath)
	updateGate := &localapi.ActionGate{}
	updates.Apply = func(ctx context.Context, r releaseauth.Release) string {
		return applyBackgroundUpdate(ctx, r, updateGate)
	}
	go updates.Run(ctx)
	fleet := &fleetUpdates{store: stateStore, signer: deviceIdentity, monitor: updates}
	go fleet.run(ctx)
	resumeMonitor := powerresume.NewMonitor(time.Now())
	go func(initialSettings connectionSettings) {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		activeSettings := initialSettings.fingerprint()
		lastPull := time.Now()
		retrySync := getCapabilitySyncCheck().Status == "pending"
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if resumeMonitor.Observe(time.Now()) {
					_ = powerresume.Refresh()
				}
				current, settingsErr := loadConnectionSettings()
				if settingsErr != nil {
					setCapabilitySyncCheck("fail", "Connection service settings need repair")
					retrySync = true
					continue
				}
				if current.GatewayURL == "" {
					activeSettings = ""
					setCapabilitySyncCheck("skip", "Connection service is not configured")
					continue
				}
				currentSettings := current.fingerprint()
				settingsChanged := currentSettings != activeSettings
				if !settingsChanged && current.Pull {
					healthContext, cancelHealth := context.WithTimeout(ctx, 2*time.Second)
					healthErr := current.gatewayClient().Health(healthContext)
					cancelHealth()
					if healthErr != nil {
						setCapabilitySyncCheck("pending", "Connection service will retry in the background")
						retrySync = true
						continue
					}
				}
				if !connectionSyncDue(settingsChanged, current.Pull, retrySync, lastPull, time.Now()) {
					continue
				}
				synchronizer := current.synchronizer()
				var snapshot state.Snapshot
				legacyMu.Lock()
				hasLegacy := legacyCandidate != nil
				if hasLegacy {
					snapshot = legacyCandidate.Snapshot
				}
				legacyMu.Unlock()
				if !hasLegacy {
					loaded, _, loadErr := stateStore.LoadLatestValidSnapshot(context.Background())
					if loadErr != nil {
						continue
					}
					snapshot = loaded
				}
				syncContext, cancelSync := context.WithTimeout(ctx, 5*time.Second)
				var synced state.Snapshot
				var imported int
				var syncErr error
				if settingsChanged {
					var result capabilitysync.Result
					synced, result, syncErr = synchronizer.Sync(syncContext, snapshot, deviceIdentity)
					imported = result.Imported
				} else {
					synced, imported, syncErr = synchronizer.PullClaims(syncContext, snapshot, deviceIdentity)
				}
				cancelSync()
				if syncErr == nil {
					activeSettings = currentSettings
					lastPull = time.Now()
					retrySync = false
				}
				if syncErr != nil || imported == 0 {
					if syncErr != nil {
						setCapabilitySyncCheck("pending", "Connection service will retry in the background")
						retrySync = true
					} else {
						setCapabilitySyncCheck("pass", "Connection service synchronized signed capabilities")
					}
					continue
				}
				if hasLegacy {
					legacyMu.Lock()
					if legacyCandidate != nil && legacyCandidate.Snapshot.FabricID == snapshot.FabricID && legacyCandidate.Snapshot.DirectoryVersion == snapshot.DirectoryVersion {
						legacyCandidate.Snapshot = synced
					}
					legacyMu.Unlock()
				} else {
					if commitErr := commitImportedSnapshot(context.Background(), stateStore, synced, imported); commitErr != nil {
						setCapabilitySyncCheck("pending", "Connection service will retry after local state is writable")
						retrySync = true
						continue
					}
				}
				setCapabilitySyncCheck("pass", "Connection service synchronized signed capabilities")
			}
		}
	}(initialConnectionSettings)
	activityStore := activity.NewPersistent(50, stateStore)
	smartRouteHealth := route.NewHealthTracker(12)
	provider := func() (actions.Service, error) {
		activeLegacy, legacyErr := loadLegacyCandidate()
		if legacyErr != nil {
			return actions.Service{}, legacyErr
		}
		current, _, loadErr := stateStore.LoadLatestValidSnapshot(context.Background())
		if loadErr != nil {
			return actions.Service{}, loadErr
		}
		var activeLegacyRoutes migration.LegacyEndpointRoutes
		var activeLegacyExternal migration.LegacyExternalDesktop
		if activeLegacy != nil {
			legacyMu.Lock()
			current = activeLegacy.Snapshot
			activeLegacyRoutes = activeLegacy.Routes
			activeLegacyExternal = activeLegacy.External
			legacyMu.Unlock()
			if desktopProfiles != nil {
				overlaid, overlayErr := desktopProfiles.Apply(current)
				if overlayErr != nil {
					return actions.Service{}, overlayErr
				}
				current = overlaid
				activeLegacyExternal = activeLegacyExternal.WithoutTargets(desktopProfiles.Targets())
			}
		}
		service := actions.Service{DeviceID: deviceIdentity.DeviceID()}
		service.Activity = activityStore
		service.DesktopCredentials = desktopCredentials
		service.AdditionalChecks = []contracts.Check{getCapabilitySyncCheck(), updates.Status()}
		service.Catalog = catalog.New(current.FabricID, current.Devices, current.Capabilities, current.Grants, service.DeviceID, current.CapturedAt, func() time.Time {
			effective, err := stateStore.ObserveAuthorizationTime(context.Background(), time.Now())
			if err != nil {
				// A storage failure must not silently extend cached authority.
				return current.CapturedAt.Add(catalog.DefaultAuthorizationTTL)
			}
			return effective
		})
		if activeLegacy != nil {
			service.Shell = activeLegacy.Shell.Runner(service.DeviceID, service.Catalog, activeLegacyRoutes, defaultSSHIdentityFile())
			desktopExecutor := desktopexecutor.Executor{Credentials: desktopCredentials}
			if desktopConfig != nil {
				desktopExecutor.Trust = *desktopConfig
			}
			lanProviders := []route.NamedProvider{{Adapter: "lan", Provider: route.ReachableProvider{Provider: activeLegacyRoutes.Provider("lan"), Timeout: 750 * time.Millisecond, Samples: 3}}}
			tailscaleProviders := []route.NamedProvider{{Adapter: "tailscale", Provider: route.ReachableProvider{Provider: activeLegacyRoutes.Provider("tailscale"), Timeout: 2 * time.Second, Samples: 3}}}
			if desktopConfig != nil {
				lanProviders = append([]route.NamedProvider{{Adapter: "lan", Provider: route.ReachableProvider{Provider: desktopConfig.Provider("lan"), Timeout: 750 * time.Millisecond, Samples: 3}}}, lanProviders...)
				tailscaleProviders = append([]route.NamedProvider{{Adapter: "tailscale", Provider: route.ReachableProvider{Provider: desktopConfig.Provider("tailscale"), Timeout: 2 * time.Second, Samples: 3}}}, tailscaleProviders...)
			}
			lanProvider := route.Selector{Providers: lanProviders}
			tailscaleProvider := route.Selector{Providers: tailscaleProviders}
			var gatewayProvider route.Provider
			managedGatewayConfig, managedGatewayErr := migration.DefaultLegacyManagedFRPConfigPath()
			if managedGatewayErr == nil {
				if info, statErr := os.Stat(managedGatewayConfig); statErr == nil && info.Mode().IsRegular() {
					gatewayProvider = route.ReachableProvider{Provider: activeLegacy.Gateway.ManagedProvider(managedGatewayConfig), Samples: 3}
				}
			}
			if gatewayProvider == nil {
				frpcPath := strings.TrimSpace(os.Getenv("PFREMOTE_FRPC_PATH"))
				if info, statErr := os.Stat(frpcPath); frpcPath != "" && statErr == nil && info.Mode().IsRegular() {
					gatewayProvider = activeLegacy.Gateway.Provider(frpcPath, filepath.Join(filepath.Dir(statePath), "route-sessions"))
				}
			}
			if gatewayProvider != nil {
				gatewayProvider = legacyGatewayDesktopProvider{
					Provider: gatewayProvider,
					Protocol: func(target string) string {
						resolved, resolveErr := service.Catalog.Resolve(target)
						if resolveErr != nil || resolved.Capability.DesktopProfile == nil {
							return ""
						}
						return resolved.Capability.DesktopProfile.Protocol
					},
				}
			}
			tailscaleCoordinator := desktop.Coordinator{
				SubjectDeviceID: service.DeviceID, Resolver: service.Catalog,
				RouteProvider: tailscaleProvider, Executor: desktopExecutor,
			}
			lanCoordinator := desktop.Coordinator{
				SubjectDeviceID: service.DeviceID, Resolver: service.Catalog,
				RouteProvider: lanProvider, Executor: desktopExecutor,
			}
			smartProviders := []route.NamedProvider{
				{Adapter: "lan", Provider: lanProvider},
				{Adapter: "tailscale", Provider: tailscaleProvider},
			}
			routeCoordinators := map[string]actions.DesktopCoordinator{"tailscale": tailscaleCoordinator, "lan": lanCoordinator}
			if gatewayProvider != nil {
				smartProviders = append(smartProviders, route.NamedProvider{Adapter: "frp", Provider: gatewayProvider})
				routeCoordinators["frp"] = desktop.Coordinator{SubjectDeviceID: service.DeviceID, Resolver: service.Catalog, RouteProvider: gatewayProvider, Executor: desktopExecutor}
			}
			routedDesktop := actions.SessionDesktop{
				Coordinator: desktop.Coordinator{
					SubjectDeviceID: service.DeviceID, Resolver: service.Catalog,
					RouteProvider: route.Selector{Providers: smartProviders, EvaluateAll: true, Health: smartRouteHealth}, Executor: desktopExecutor,
				},
				RouteCoordinators: routeCoordinators,
			}
			service.Desktop = actions.DesktopMux{Override: activeLegacyExternal, Fallback: routedDesktop}
			service.RouteOptions = func(target string) []contracts.RouteOption {
				if activeLegacyExternal.HasTarget(target) {
					return []contracts.RouteOption{{Adapter: "legacy-external", Status: "available", Order: 1}}
				}
				var options []contracts.RouteOption
				if desktopConfig != nil && desktopConfig.HasTarget(target) {
					for _, adapter := range desktopConfig.Adapters(target) {
						provider := lanProvider
						if adapter == "tailscale" {
							provider = tailscaleProvider
						}
						probeContext, cancelProbe := context.WithTimeout(context.Background(), 2200*time.Millisecond)
						acquired, acquireErr := provider.Acquire(probeContext, route.Request{CanonicalTarget: target})
						cancelProbe()
						if acquired != nil {
							_ = acquired.Close()
						}
						status := "unavailable"
						if acquireErr == nil {
							status = "available"
						}
						options = append(options, contracts.RouteOption{Adapter: adapter, Status: status, Order: len(options) + 1})
					}
					return options
				}
				probeContext, cancelProbe := context.WithTimeout(context.Background(), 1500*time.Millisecond)
				available := activeLegacyRoutes.AvailableAdapters(probeContext, target, 600*time.Millisecond)
				cancelProbe()
				for _, adapter := range activeLegacyRoutes.Adapters(target) {
					status := "unavailable"
					if available[adapter] {
						status = "available"
					}
					options = append(options, contracts.RouteOption{Adapter: adapter, Status: status, Order: len(options) + 1})
				}
				if gatewayProvider != nil && activeLegacy.Gateway.HasTarget(target) {
					options = append(options, contracts.RouteOption{Adapter: "frp", Status: probeRouteStatus(gatewayProvider, target, 1500*time.Millisecond), Order: len(options) + 1})
				}
				return options
			}
		} else if desktopConfig != nil {
			desktopExecutor := desktopexecutor.Executor{Trust: *desktopConfig, Credentials: desktopCredentials}
			tailscaleProvider := route.ReachableProvider{Provider: desktopConfig.Provider("tailscale"), Timeout: 2 * time.Second, Samples: 3}
			lanProvider := route.ReachableProvider{Provider: desktopConfig.Provider("lan"), Samples: 3}
			service.Desktop = actions.SessionDesktop{Coordinator: desktop.Coordinator{
				SubjectDeviceID: service.DeviceID, Resolver: service.Catalog,
				RouteProvider: route.Selector{Providers: []route.NamedProvider{
					{Adapter: "lan", Provider: lanProvider},
					{Adapter: "tailscale", Provider: tailscaleProvider},
				}, EvaluateAll: true, Health: smartRouteHealth}, Executor: desktopExecutor,
			}, RouteCoordinators: map[string]actions.DesktopCoordinator{
				"tailscale": desktop.Coordinator{SubjectDeviceID: service.DeviceID, Resolver: service.Catalog, RouteProvider: tailscaleProvider, Executor: desktopExecutor},
				"lan":       desktop.Coordinator{SubjectDeviceID: service.DeviceID, Resolver: service.Catalog, RouteProvider: lanProvider, Executor: desktopExecutor},
			}}
			service.RouteOptions = func(target string) []contracts.RouteOption {
				adapters := desktopConfig.Adapters(target)
				options := make([]contracts.RouteOption, 0, len(adapters))
				for index, adapter := range adapters {
					options = append(options, contracts.RouteOption{Adapter: adapter, Status: "available", Order: index + 1})
				}
				return options
			}
		}
		return service, nil
	}
	server := localapi.NewServerWithProvider(provider)
	server.Handler.Gate = updateGate
	server.Handler.Updates = fleet.action
	if err := server.Serve(ctx, listener); err != nil {
		fmt.Fprintln(os.Stderr, "PF Remote daemon stopped:", err)
		os.Exit(1)
	}
}

func probeRouteStatus(provider route.Provider, target string, timeout time.Duration) string {
	if provider == nil {
		return "unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	acquired, err := provider.Acquire(ctx, route.Request{CanonicalTarget: target})
	if acquired != nil {
		_ = acquired.Close()
	}
	if err != nil {
		return "unavailable"
	}
	return "available"
}

func defaultSSHIdentityFile() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".ssh", "id_ed25519")
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ""
	}
	return path
}
