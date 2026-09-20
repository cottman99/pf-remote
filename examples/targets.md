# Synthetic targets

All examples are intentionally fictional. The bootstrap catalog provides:

- `compute-node/shell`
- `compute-node/desktop`
- `workstation/shell`

Example:

```powershell
pfremote inspect compute-node/shell --json
pfremote context compute-node/shell --task "Inspect the repository" --constraint "Do not modify files"
```

The constraint is an instruction to the agent, not a technical sandbox.

