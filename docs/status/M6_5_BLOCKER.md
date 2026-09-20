# M6.5 external-state blocker

## Product state

The separate Windows controlled computer is intentionally offline at the
owner's request. PF Remote continues to show it as offline with actions disabled
and does not connect, wake, deploy to, or probe it.

The installed Windows controller, private Gateway, headless Linux computer and
five Linux actions remain available. The old Center remains running as the
independent fallback. The current Windows enrollment helper is built and its
local identity, enrollment, invitation, migration, and activation checks pass.

## Why work cannot continue

The next roadmap item requires the separate Windows computer to create and use
its own protected Device identity. The controller cannot substitute for that
computer without invalidating the identity and physical-Desktop acceptance
criteria. Repeating local tests would not advance the replacement claim.

This same external condition has held for three consecutive Goal turns: the
owner-directed offline turn and two automatic continuations. No authorized,
meaningful mainline action remains while the computer is intentionally offline.

## Resume condition

Resume when the owner explicitly reports the Windows computer online. Begin at
R0 by resolving the exact existing catalog Device and confirming the untouched
legacy fallback before sending any enrollment or diagnostic request.
