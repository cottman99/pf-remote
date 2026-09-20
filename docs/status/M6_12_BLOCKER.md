# M6.12 blocker — Windows Home client intentionally offline

The owner reported more than once that Windows Home client was still offline and explicitly
asked PF Remote to leave it alone until they report it online. Earlier rollout
work therefore completed the local Windows and headless Linux paths without
waking, probing, configuring, or deploying to Windows Home client.

This is an external availability condition, not a software failure. Repeating
network probes cannot advance identity enrollment and would contradict the
owner's instruction. Resume when the owner reports Windows Home client online, then begin
enrollment from that computer's own identity while preserving the installed
`0.1.0-alpha.27` rollback candidate.
