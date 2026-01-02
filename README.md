# TF2Chart

Helm chart for deploying Team Fortress 2 servers in Kubernetes with dynamic content merging from multiple overlay sources.

## Overview

TF2Chart deploys TF2 servers in Kubernetes using a layered filesystem approach. It merges immutable base installations, additive overlays (like git-synced configs), and writable runtime directories into a unified `/tf` tree.

**Key Features:**

- Compiled Go utilities for permissions, stitching, and file watching
- Real-time overlay updates via filesystem watcher sidecar
- Flexible workload types (Deployment or StatefulSet)
- Advanced overlay system with runtime templates and copy-on-start support
- Dual-phase permission enforcement for proper file ownership

## How It Works

1. **Permissions Init**: Normalizes file ownership on base directories
2. **Decompressor Init**: Scans overlays for .bz2 files (like maps), decompresses them, and removes archives
3. **Stitcher**: Merges base + overlay layers into `/tf` using symlinks
4. **Watcher Sidecar**: Monitors overlays for changes and re-runs merge automatically
5. **Application**: TF2 server runs with merged view

The watcher uses filesystem events (inotify) and polling to detect changes from git-sync or other overlay sources, automatically refreshing the merged view without pod restarts.

## Prerequisites

- Kubernetes v1.25+
- Helm 3.12+
- TF2 base installation at `/tf/standalone` (or custom path)
- Steam Game Server Login Token (GSLT)
- UDP port access for game traffic (default: 28015)

## Installation

```bash
helm upgrade --install tf2chart ./TF2Chart \
  --namespace gameservers --create-namespace \
  -f my-values.yaml
```

## Configuration

### Core Environment Variables

| Variable            | Description                   | Default              | Required |
| ------------------- | ----------------------------- | -------------------- | -------- |
| `SRCDS_TOKEN`       | Steam Game Server Login Token | ``                   | Yes      |
| `SRCDS_PW`          | Server password               | ``                   | No       |
| `SRCDS_MAXPLAYERS`  | Maximum player slots          | `24`                 | No       |
| `SRCDS_REGION`      | Region code for matchmaking   | `255`                | No       |
| `TF2_CUSTOM_CONFIG` | Path to custom config file    | `/tf/cfg/server.cfg` | No       |

### Overlays & Layers

Define multiple overlay sources with ordered precedence:

```yaml
overlays:
  # Read-only git-synced base
  - name: serverfiles-base
    type: hostPath
    path: /mnt/serverfiles/serverfiles/base
    hostPathType: Directory
    readOnly: true

  # Writable runtime overlay
  - name: serverfiles-runtime
    type: hostPath
    path: /mnt/serverfiles/runtime-overlays/advanced-one
    hostPathType: DirectoryOrCreate
    readOnly: false
```

**Advanced:** Use `subPath` for nested mounts or `sourcePath` to stitch only a subdirectory:

```yaml
overlays:
  - name: serverfiles-base
    type: hostPath
    path: /mnt/serverfiles # parent on host
    subPath: serverfiles/base # child to mount
    readOnly: true
```

### Writable Paths

Map writable directories to specific overlay volumes:

```yaml
writablePaths:
  - path: tf/logs
    overlay: serverfiles-runtime
  - path: tf/uploads
    overlay: serverfiles-runtime
```

**Template-based Writable Paths:** Start from a tracked template with runtime edits:

```yaml
writablePaths:
  - path: tf/tf/cfg
    overlay: serverfiles-runtime
    template:
      overlay: serverfiles-base
      sourcePath: tf/tf/cfg
      clean: true # wipe destination between merges
```

### Copy-on-Start Templates

Copy pristine templates on each pod restart while keeping them writable:

```yaml
copyTemplates:
  - targetPath: tf/tf/addons/sourcemod/configs/sourcebans
    overlay: serverfiles-base
    sourcePath: serverfiles/base/tf/addons/sourcemod/configs/sourcebans
    cleanTarget: true # remove destination before copying
```

Perfect for SourceBans configs that need pristine templates on each rollout.

### Decompressor

Automatically decompress .bz2 files before merging. Useful for TF2 map files that are often distributed as compressed archives:

```yaml
decompressor:
  enabled: true
  cachePath: /cache/decompression.cache # optional: enable cache to prevent git-lfs pointer overwrites
  image:
    repository: ghcr.io/udl-tf/tf2chart-decompressor
    tag: latest
  scanBase: true # scan base path for .bz2 files
  scanOverlays:
    - maps # scan specific overlay layers
    - custom

# Runtime decompression with watcher
merger:
  decompressionCachePath: /cache/decompression.cache # optional: must match decompressor.cachePath
  decompressPaths:
    - /mnt/overlays/maps # paths to scan for .bz2 files when watcher detects changes
  watcher:
    enabled: true
```

The decompressor runs as an init container before the stitcher, scanning specified paths for `.bz2` files, decompressing them in-place, and removing the compressed archives. This ensures map files and other compressed content are ready before the merge process begins.

**Git-LFS Protection with Cache:**

When using git-sync with git-lfs to distribute maps, git-sync periodically overwrites decompressed `.bsp` files with small git-lfs pointer files. The cache prevents this:

1. Tracks SHA256 hash of every decompressed file
2. Detects when git-lfs overwrites a file with a pointer
3. Automatically re-decompresses the file
4. Ensures FastDL always serves actual maps, not pointers

**Quick Enable:** Just set `cachePath` in both `decompressor` and `merger` to the same path. See [ENABLE_CACHE.md](ENABLE_CACHE.md) for details.

**Split Map Support:**

The decompressor also handles large maps that have been split into multiple compressed parts. These files are created by splitting a single `.bsp.bz2` file into chunks. Two folder naming patterns are supported:

- `tfdb_map_name.bsp/` - containing parts
- `map_name.bsp.bz2.parts/` - containing parts

Inside the folder, you'll find chronologically ordered parts:

```
bhop_arcane2_a06.bsp.bz2.parts/
  bhop_arcane2_a06.bsp.bz2.part.000
  bhop_arcane2_a06.bsp.bz2.part.001
  bhop_arcane2_a06.bsp.bz2.part.002
  bhop_arcane2_a06.bsp.bz2.part.003
  bhop_arcane2_a06.bsp.bz2.part.004
```

The decompressor will:

1. Detect the folder (ending with `.bsp` or `.bsp.bz2.parts`)
2. Concatenate all `.bz2.part.*` files in order
3. Decompress the combined bz2 stream
4. Save as a single `.bsp` file (e.g., `bhop_arcane2_a06.bsp`)
5. Place the final file where the folder was located
6. Remove the folder and all parts

This is particularly useful for very large TF2 maps that exceed typical file size limits.

### Permissions

**Important:** Permission containers must run as root (UID 0) to execute `chown`. Configured by default.

```yaml
permissionsInit:
  runFirst: true # fix /mnt/base before stitching
  runLast: true # fix /tf before app starts
  applyDuringMerge: true # re-run on every merge cycle
```

### Watcher Sidecar

Real-time overlay monitoring with automatic merge on changes:

```yaml
merger:
  watcher:
    enabled: true
    image:
      repository: ghcr.io/udl-tf/tf2chart-watcher
      tag: latest
    debounceSeconds: 2
    pollIntervalSeconds: 300 # fallback for filesystems without inotify
    watchBase: false # set true to monitor /mnt/base changes
    watchParentDepth: 1 # watch parent directories for git-sync atomic swaps
    extraWatchPaths: [] # additional paths to monitor beyond overlays
```

The watcher uses inotify events + polling to detect changes, automatically re-merging without pod restarts.

**Runtime Decompression:** When new files are synced (e.g., via git-sync), the watcher triggers a merge operation that includes automatic decompression of any `.bz2` files found in the configured `decompressPaths`. This ensures that newly added compressed maps are automatically decompressed and made available without manual intervention or pod restarts.

Configure decompression paths in your merge configuration:

```yaml
merger:
  config:
    decompressPaths:
      - /mnt/overlays/maps
      - /mnt/overlays/custom
```

**Example: Git-sync with atomic swaps**

When using git-sync, updates happen via atomic directory swaps (symlink changes). Set `watchParentDepth: 1` to detect these:

```yaml
merger:
  watcher:
    watchParentDepth: 1

overlays:
  - name: serverfiles-base
    path: /mnt/serverfiles # git-sync syncs here, swaps symlinks
    sourcePath: serverfiles/base
```

Without `watchParentDepth`, the watcher only monitors `/mnt/overlays/serverfiles-base` (the final mount). With `watchParentDepth: 1`, it also watches `/mnt/overlays`, detecting when git-sync creates a new directory and swaps the symlink.

**Example: Custom config directories**

Monitor additional paths outside your overlays:

```yaml
merger:
  watcher:
    extraWatchPaths:
      - /mnt/config-server/tf-configs
      - /mnt/shared/plugins

overlays:
  - name: serverfiles
    path: /mnt/serverfiles
```

Now changes to `/mnt/config-server/tf-configs` or `/mnt/shared/plugins` will trigger automatic re-merges, even though they're not defined as overlays.

## Development

### Project Structure

```
TF2Chart/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── _helpers.tpl
│   ├── _podtemplate.tpl
│   ├── deployment.yaml
│   ├── service.yaml
│   └── statefulset.yaml
└── src/
    ├── go.mod
    ├── cmd/
    │   ├── merger/      # Stitcher binary
    │   ├── permissions/ # Permission fixer binary
    │   └── watcher/     # Filesystem watcher binary
    └── internal/
        ├── config/
        ├── merge/
        └── watch/
```

### Building Go Utilities

```bash
cd src
go test ./...
docker build -f cmd/merger/Dockerfile -t ghcr.io/udl-tf/tf2chart-merger:latest .
docker build -f cmd/permissions/Dockerfile -t ghcr.io/udl-tf/tf2chart-permissions:latest .
docker build -f cmd/watcher/Dockerfile -t ghcr.io/udl-tf/tf2chart-watcher:latest .
docker build -f cmd/decompressor/Dockerfile -t ghcr.io/udl-tf/tf2chart-decompressor:latest .
```

## License

See [LICENSE](LICENSE).

## Dependencies

- [tf2-image](https://github.com/UDL-TF/TF2Image) – Base TF2 container with SteamCMD
- [Helm](https://helm.sh/) – Chart deployment
- [Kubernetes](https://kubernetes.io/) – Target platform
