# Runbook

## Run and test locally

```bash
cd app
go test -race ./...
PORT=8080 go run .
curl -i localhost:8080/health      # 200 {"status":"ok","version":"dev"}
```

Build and run the container (from the repo root):

```bash
docker build --build-arg VERSION=$(git rev-parse --short HEAD) -t kubership:local .
docker run --rm -p 8080:8080 kubership:local
```

## Render the manifests

```bash
kubectl kustomize deploy/overlays/dev
kubectl kustomize deploy/overlays/staging
```

## Verify a deployment

```bash
kubectl -n kubership-dev rollout status deploy/kubership
kubectl -n kubership-dev port-forward svc/kubership 8080:80
curl -s localhost:8080/health      # "version" must equal the commit that was deployed
```

## Troubleshooting

| Symptom | Likely cause | Check |
|---------|--------------|-------|
| Pod `CrashLoopBackOff`, log `listen ... address already in use` | `PORT` clashes with another listener in the pod | `kubectl logs`, the `PORT` env var |
| Pod never Ready | `/health` not answering on the container port | `kubectl describe pod` (probe failures), `kubectl logs` |
| `ImagePullBackOff` | Overlay still points at the placeholder image or tag | `kubectl kustomize deploy/overlays/<env> \| grep image:` |
| Requests fail during a rollout | New pods not Ready yet or the termination grace period is too short | `kubectl rollout status`, the app's 10 s drain vs `terminationGracePeriodSeconds` (default 30 s) |
| `/health` reports an old `version` | The old image is still running | `kubectl get deploy -o wide`, rollout history |
