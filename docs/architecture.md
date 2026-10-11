# Architecture

Kubership is a deliberately small service whose purpose is to exercise a complete
Jenkins-based CI/CD pipeline on Kubernetes.

## Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Application | `app/` | Go HTTP API. One endpoint: `GET /health` returns `{"status":"ok","version":"<build>"}`. |
| Container image | `Dockerfile` | Multi-stage build; static binary on `distroless/static:nonroot` (uid 65532). |
| Pipeline | `Jenkinsfile` | Declarative Jenkins pipeline (stages added step by step in Phase 3). |
| Kubernetes manifests | `deploy/` | Kustomize `base` plus `dev` and `staging` overlays. |
| Jenkins config | `jenkins/` | Helm values and Configuration-as-Code (`casc/`) for the Jenkins controller. |

## Flow

```
commit -> Jenkins pipeline -> go test -> docker build (VERSION=<git sha>)
       -> push image -> kustomize edit set image -> deploy overlay (dev / staging)
```

Only the commit and test/build stages are defined so far; the rest is planned
for the following Phase 3 steps.

## Application design

- `GET /health` is the only route. Other methods return 405, unknown paths 404.
- Listens on `$PORT` (default `8080`).
- Handles SIGTERM/SIGINT by draining in-flight requests (10 s) before exiting, so
  Kubernetes rolling updates do not drop connections.
- The build version is injected with `-ldflags "-X main.version=<sha>"` and
  reported by `/health`, which lets a deployment be verified from outside.
- Logs are structured JSON on stdout.

## Deployment model

- `deploy/base`: Deployment and ClusterIP Service. Pods run non-root with a
  read-only root filesystem, all capabilities dropped, the `RuntimeDefault`
  seccomp profile, no service-account token, and resource requests and limits.
  Readiness and liveness probes call `/health`.
- `deploy/overlays/dev`: namespace `kubership-dev`, 1 replica.
- `deploy/overlays/staging`: namespace `kubership-staging`, 2 replicas.
- The base image name is the placeholder `kubership`. The pipeline sets the real
  registry and tag through each overlay's `images:` section.
