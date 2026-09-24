# GH actions bootstrap

This repo is a testing ground for GitHub Actions, isolated from real production code.

## Container build

The default workflow builds and pushes a multi-arch (amd64 + arm64) container image to GitHub Container Registry.

### How it is triggered

The workflow runs on these events, with different behavior per event:

| Event | `build` job | `manifest` job | Published tags |
|-------|-------------|----------------|----------------|
| **push to `main`** | builds amd64 + arm64, pushes per-arch images, writes GHA cache | merges per-arch images into multi-arch manifest lists | `latest`, commit SHA |
| **push of a `v*` tag** | same as above | same as above | semver version (e.g. `v1.2.3` → `1.2.3`), commit SHA, `latest` |
| **pull request targeting `main`** | builds amd64 + arm64, **no push**, **no cache write** | **skipped** | none |

On pull requests the push step is disabled (`push: false`) and the cache is read-only — the goal is purely to validate that both architectures build correctly before merging.

Action versions are kept up to date by Dependabot (`.github/dependabot.yml`, weekly updates).

### What is built

A small Go binary that prints its own architecture — used to validate that the multi-arch pipeline produces the correct image on each platform.

## RHEL Image Mode qcow2 (bootc)

The `image-bootc-qcow2` workflow builds an RHEL Image Mode (bootc) container from `image-bootc/Containerfile` and converts it into a bootable **qcow2 disk image** with `bootc-image-builder` — for **both amd64 and arm64**, on native runners.

- **Base image:** `quay.io/centos-bootc/centos-bootc:stream10` (no Red Hat subscription needed), with cloud-init installed and an `opc` default user via `99-default-user.cfg` (adapted from the `homelab/oracle-vls/image-bootc` experiment).
- **Architecture:** two lanes — `ubuntu-24.04` → amd64, `ubuntu-24.04-arm` → arm64.
- **Flow per lane:** build + push the bootc container to GHCR → `podman pull` into root storage → `bootc-image-builder --type qcow2 --rootfs xfs` → `qemu-img info` check → upload `disk-qcow2-<arch>` artifact.
- **Images on GHCR:** per-arch pushed as `bootc-<tag>-<arch>`, then the `manifest` job merges them into a multi-arch `bootc-<tag>` tag.

### How to trigger

Manual only (`workflow_dispatch`) — deliberately *not* tied to pushes or tags, so a tag never sets off several workflows in this shared repo.

Via the web UI:
**Actions → *Image Mode qcow2 (bootc)* → *Run workflow*** → set *image tag* (optional) → *Run workflow*.

Via the CLI:

```bash
gh workflow run image-bootc-qcow2.yml --field image_tag=stream10-20260924
gh run watch # or: gh run list --workflow=image-bootc-qcow2.yml
```

The `image_tag` input names both the GHCR tag and the tag baked into the disk image. Leave it empty to use `stream10-<date>` (UTC). The tag itself has no meaning to the OS — it only labels the bootc container — the image content always matches the current `image-bootc/Containerfile`.

Triggering twice on the same day pushes to the same GHCR tag (overwrite); artifacts get a new run id each time.

### What you get

Two workflow artifacts (GHCR packages are the bootc container, not the disk):

| Artifact | Content |
|----------|---------|
| `disk-qcow2-amd64` | `disk.qcow2` — 10 GiB virtual, qcow2, `xfs` rootfs, cloud-init `opc` user |
| `disk-qcow2-arm64` | same, for arm64 |

### How to download

Via the web UI: open the run → **Artifacts** section (right-hand column) → `disk-qcow2-<arch>`.

Via the CLI (artifacts expire after 14 days):

```bash
gh run download <run-id> --pattern 'disk-qcow2-*' -D ~/downloads
gh run download <run-id> -n disk-qcow2-arm64 -D ~/downloads   # single arch
```

Each artifact is a zip containing `disk.qcow2`. Boot it on a machine/VPS matching the architecture, e.g. with the OCI image-bootc setup: attach the cloud-init-capable disk and let it create the `opc` user on first boot.

### The bootc container on GHCR

The disk images are built from `ghcr.io/crowdsalat/bootstrap-gh-actions:bootc-<tag>[-<arch>]`:

```bash
podman pull ghcr.io/crowdsalat/bootstrap-gh-actions:bootc-stream10-20260924
podman run --rm --pull=never --entrypoint /usr/bin/bootc \
  ghcr.io/crowdsalat/bootstrap-gh-actions:bootc-stream10-20260924 status
```

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

- `ubuntu-24.04` → `linux/amd64`
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
