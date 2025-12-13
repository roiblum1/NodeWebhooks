# ✅ Air-Gapped / Disconnected Environment Ready

This project is **fully compatible** with air-gapped and disconnected environments where there is no internet access.

## What Was Done

### 1. ✅ Removed golangci-lint Dependency
- **Removed**: `.golangci.yml` configuration file
- **Removed**: `make install-lint`, `make lint`, `make lint-fix` targets
- **Replaced with**: Built-in Go tools (`go vet`, `go fmt`)
- **Reason**: golangci-lint requires internet to download, not suitable for air-gapped environments

### 2. ✅ Vendored All Dependencies
- **Created**: `vendor/` directory with all Go dependencies (46 MB)
- **Committed**: vendor/ to git repository
- **Benefit**: No internet required during build

### 3. ✅ Updated Build Process
- **Dockerfile**: Now uses `-mod=vendor` flag
- **Makefile**: Added `make vendor` target
- **Build command**: `go build -mod=vendor ./cmd/webhook`

### 4. ✅ Updated Configuration
- **.gitignore**: vendor/ is now committed (not ignored)
- **Dockerfile**: Copies vendor/ directory before building
- **Build flag**: All builds use `-mod=vendor`

### 5. ✅ Created Documentation
- **[docs/AIR_GAPPED_DEPLOYMENT.md](docs/AIR_GAPPED_DEPLOYMENT.md)**: Complete air-gapped deployment guide
- Step-by-step instructions for connected and disconnected environments
- Troubleshooting section
- Testing procedures

## Verification

### ✅ Local Build Works
```bash
go build -mod=vendor -o bin/webhook ./cmd/webhook
# ✅ Builds successfully without internet
```

### ✅ Container Build Works
```bash
podman build -t node-cleanup-webhook:test .
# ✅ Builds successfully using vendor directory
```

### ✅ All Dependencies Vendored
```bash
du -sh vendor/
# 46M vendor/

ls vendor/
# github.com  golang.org  google.golang.org  gopkg.in  k8s.io  modules.txt  sigs.k8s.io
```

## How to Use in Air-Gapped Environment

### Quick Start

**Connected Environment (preparation):**
```bash
# 1. Clone repository (includes vendor/)
git clone https://github.com/your-org/node-cleanup-webhook.git

# 2. Build image
podman build -t node-cleanup-webhook:v1.0.0 .

# 3. Export image
podman save node-cleanup-webhook:v1.0.0 -o webhook.tar

# 4. Transfer webhook.tar to air-gapped environment
```

**Air-Gapped Environment (deployment):**
```bash
# 1. Load image
podman load -i webhook.tar

# 2. Tag for internal registry
podman tag node-cleanup-webhook:v1.0.0 internal-registry/webhook:v1.0.0

# 3. Push to internal registry
podman push internal-registry/webhook:v1.0.0

# 4. Deploy to Kubernetes
helm install webhook ./deploy/helm/... \
  --set image.repository=internal-registry/webhook \
  --set image.tag=v1.0.0
```

## Available Make Targets (No Internet Required)

```bash
make build-local   # Build binary using vendor
make build         # Build container image using vendor
make fmt           # Format code (built-in go fmt)
make vet           # Static analysis (built-in go vet)
make check         # Run fmt + vet + test
make test          # Run tests
make vendor        # Re-vendor dependencies (requires internet)
```

## What Doesn't Require Internet

✅ **Building binary**: `go build -mod=vendor`
✅ **Building container**: `podman build`
✅ **Running tests**: `go test -mod=vendor`
✅ **Formatting code**: `go fmt`
✅ **Static analysis**: `go vet`
✅ **Running locally**: `make run-local`

## What Requires Internet (Only Initial Setup)

❌ **Cloning repository**: `git clone` (one-time)
❌ **Updating vendor**: `go mod vendor` (only when updating dependencies)
❌ **Pulling base images**: `docker pull golang:1.21-alpine` (cached after first pull)

## File Sizes for Transfer

```
Container image tar:  ~50-80 MB
Vendor directory:     46 MB
Total repository:     ~100 MB
```

## Code Quality Without golangci-lint

We use built-in Go tools for code quality:

```bash
# Format code
make fmt

# Static analysis
make vet

# Run all checks
make check
```

These tools are:
- ✅ Part of standard Go installation
- ✅ No internet required
- ✅ Work offline
- ✅ Production-ready

## Security Benefits

1. **No external dependencies during build**
   - All dependencies are vendored
   - Reproducible builds
   - No supply chain attacks via download

2. **Minimal attack surface**
   - Distroless base image
   - No package manager
   - No shell

3. **Verified dependencies**
   - Vendor directory can be audited
   - Dependencies are version-locked
   - SHA checksums in go.sum

## Testing Air-Gapped Compatibility

Test that everything works without internet:

```bash
# Disable network (Linux)
sudo iptables -A OUTPUT -j DROP

# Build should still work
go build -mod=vendor ./cmd/webhook

# Container build should work
podman build -t test .

# Re-enable network
sudo iptables -F OUTPUT
```

## Updating Dependencies in the Future

When you need to update dependencies:

```bash
# In connected environment:
go get -u ./...           # Update dependencies
go mod tidy               # Clean up go.mod
go mod vendor             # Re-vendor dependencies
git add vendor/ go.mod go.sum
git commit -m "Update dependencies"

# Transfer updated repo to air-gapped environment
```

## Documentation

- **[docs/AIR_GAPPED_DEPLOYMENT.md](docs/AIR_GAPPED_DEPLOYMENT.md)** - Complete deployment guide
- **[README.md](README.md)** - Main documentation
- **[Makefile](Makefile)** - Build targets
- **[Dockerfile](Dockerfile)** - Container build

## Support

For air-gapped deployment issues:
1. Check [docs/AIR_GAPPED_DEPLOYMENT.md](docs/AIR_GAPPED_DEPLOYMENT.md)
2. Verify vendor/ directory exists: `ls vendor/`
3. Ensure using vendor mode: `go build -mod=vendor`
4. Check Dockerfile uses vendor: `grep "mod=vendor" Dockerfile`

---

**Summary**: This project is production-ready for air-gapped and disconnected environments. All dependencies are vendored, no external linters required, and builds work completely offline.
