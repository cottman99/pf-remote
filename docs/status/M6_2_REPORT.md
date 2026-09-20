# M6.2 online-computer enrollment report

## Product result

The local Windows controller and one headless Linux controlled computer now
belong to the same durable private Fabric. The Linux computer publishes one
Shell action and four Desktop actions while running no PF Remote graphical
client. The Windows controller presents those actions through the same named
catalog used by its UI, CLI, and Agent interface.

The controlled-node service, encrypted private Gateway, and Windows controller
were restarted independently. The Fabric, Device, Capability, and Grant
identities remained unchanged, and all five Linux actions reappeared. The old
installed product stayed running and unchanged as the recovery path.

The second Windows controlled computer is intentionally absent from this
milestone because its owner took it offline. No connection, wake, deployment,
or probe targeted it. Its own-identity enrollment resumes only after the owner
reports that it is online again.

## Usability boundary

Enrollment is complete for the computers that are currently in scope. The
Shell action and all three VNC virtual desktops have completed real actions.
The additional RDP virtual desktop reaches the exact controlled computer, but
Windows still shows a first-use publisher confirmation. Removing that
technical-looking prompt remains part of the next action-parity item; it is not
presented as a finished one-click journey yet.

The internal Windows candidate can include the officially signed standalone
TigerVNC viewer as a separate protocol executor. The public repository keeps
only the source integration and strict input verification; it does not contain
the third-party binary or any private deployment value.

## Verification confidence

Targeted identity, enrollment, migration, Desktop, Shell, Gateway TLS, and
restart checks pass. Private names, addresses, credentials, certificates, and
runtime evidence remain outside the public source tree. Full repository checks
continue under M6.3 after the coherent online-action slice is complete.

## Migration impact

No old configuration, route, service, or entry point was changed. The new
controller and Gateway remain an internal side-by-side candidate. Default
cutover is deferred until online-action parity and the restored Windows
computer's physical Desktop checks both pass.
