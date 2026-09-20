# Add a computer

PF Remote treats a computer as a named, authorized Device rather than an IP
address. The public repository contains the product and this procedure, but it
never contains a customer's computer names, addresses, accounts, credentials,
Gateway configuration, Tailscale state, or cloud metadata.

## From the Windows Center

1. Open **Computers** and choose **Add computer**.
2. Enter only what you already know. Every field is optional.
3. Choose **Copy task for Agent** and paste the result into Codex or another
   Agent that can work on the new computer.
4. Keep the new computer online while the Agent installs the matching client,
   enrolls its independent identity, publishes the requested Desktop or Shell
   capabilities, and validates an authorized connection.

Do not paste passwords, private keys, access tokens, recovery files, or cloud
credentials into the form or Agent conversation. The Agent should request an
operating-system or provider authorization prompt only when it is actually
needed.

## Route expectations

- **Automatic** asks PF Remote to inspect every eligible independent path.
- **Local network** is appropriate when the computers share a trusted network.
- **Tailscale** requires the customer to install and sign in to Tailscale; PF
  Remote detects and uses that existing private network without changing its
  account or global VPN configuration.
- **PF Remote Gateway** uses the owner's self-hosted relay. Alibaba Cloud is one
  possible host, not a special PF Remote protocol. Deployment-specific endpoint
  and relay material stays in protected private configuration outside source
  control and outside Agent context.

PF Remote may diagnose these paths and create process-scoped connections. It
must not change the host's proxy, DNS, default route, VPN, TUN device, or another
network product's configuration.

## Completion standard

The task is complete only when the new computer appears by its user-facing name
on the authorized controllers, its intended Desktop or Agent action works, its
eligible routes show truthful availability, and existing computers and rollback
paths remain usable.
