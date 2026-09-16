# GH actions bootstrap

This repo is a testing ground for GitHub Actions, isolated from real production code.

## Container build

The default workflow builds and pushes a multi-arch (amd64 + arm64) container image to GitHub Container Registry.

### How it is triggered

The workflow runs on:
- **Push to `main`** — builds and tags with the commit SHA and `latest`.
- **Push of a `v*` tag** — additionally tags with the semver version (e.g. `v1.2.3` → `1.2.3`).

### What is built

A small Go binary that prints its own architecture — used to validate that the multi-arch pipeline produces the correct image on each platform.

### Registry

Images are pushed to GHCR using the repository path:

```
ghcr.io/crowdsalat/bootstrap-gh-actions
```

With the following tag scheme:

| Trigger | Tags |
|---------|------|
| push to `main` | `latest`, `abc1234` (commit SHA) |
| push tag `v1.2.3` | `1.2.3`, `abc1234`, `latest` |

### Multi-arch build strategy

Both architectures are built in parallel on **native runners** (no QEMU emulation):

- `ubuntu-latest` → `linux/amd64`
- `ubuntu-24.04-arm` → `linux/arm64`

Each runner pushes a per-arch image. A final `manifest` job then uses `docker buildx imagetools create` to assemble a single multi-arch manifest list for each tag.

### Containerfile — OpenShift restricted-v3 compatibility

The Containerfile is written to be compatible with OpenShift's `restricted-v3` Security Context Constraint by default. Here is how each constraint is addressed:

| restricted-v3 requirement | How the Containerfile satisfies it |
|--------------------------|-------------------------------------|
| **Runs in a user namespace** (host UID mapped to unprivileged) | No hardcoded UIDs anywhere; no `chown` or `chmod` to fixed numeric IDs |
| **GID 0 is always assigned** | Base images already have GID 0 as a group — no explicit `chgrp` needed since nothing is written to group-writable paths |
| **Non-root default USER** | `USER 65534:0` — `nobody` with root group; the actual runtime UID is assigned by OpenShift and overrides this |
| **Exec-form ENTRYPOINT** | `ENTRYPOINT ["/app"]` — receives signals directly (PID 1 behavior) |
| **No secrets baked into image** | No credentials or environment secrets in the Containerfile or build args |
| **Lean image** | Multi-stage build: compile in `golang:1.24-alpine`, copy only the static binary to `scratch` |

> **Why `scratch` works here:** The Go binary is statically compiled (`CGO_ENABLED=0`), so it needs no runtime libraries. `scratch` is the smallest possible base. For images that need a shell or package manager, replace with `alpine:3.20` and add a `chmod 775` + `chgrp 0` on any writable directory the app needs.
