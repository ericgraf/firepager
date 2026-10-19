# Deploying the FirePager web UI

Production is deployed by Flux from the private `flux-cloud` repository. Do
not use `kubectl apply` for persistent application changes.

## Build and publish source

```sh
cd ~/scripts/firepager-v2
cd backend && go test ./... && cd ..
cd frontend && npm ci && npm run build && cd ..
docker build -t firepager-frontend:check frontend
git push origin main
```

GitHub Actions publishes images tagged with the source SHA:

```text
ghcr.io/ericgraf/firepager/frontend:sha-<source-git-sha>
ghcr.io/ericgraf/firepager/backend:sha-<source-git-sha>
```

Wait for the workflow:

```sh
gh run list -R ericgraf/firepager --limit 5
gh run watch <run-id> --exit-status
```

Retrieve the immutable manifest digest:

```sh
SHA=$(git rev-parse HEAD)
docker buildx imagetools inspect \
  "ghcr.io/ericgraf/firepager/frontend:sha-$SHA"
docker buildx imagetools inspect \
  "ghcr.io/ericgraf/firepager/backend:sha-$SHA"
```

Use the top-level `Digest` value, not a platform-specific manifest digest.

## Update the fleet repository

```sh
git clone https://github.com/ericgraf/flux-cloud.git /tmp/opencode/flux-cloud
cd /tmp/opencode/flux-cloud
```

Update the frontend digest in:

```text
apps/prod/firepager/workloads/frontend.yaml
```

If backend code changed, update both:

```text
apps/prod/firepager/workloads/backend.yaml
apps/prod/firepager/workloads/processor.yaml
```

Keep images digest-pinned; never use `:latest`. Validate before pushing:

```sh
kubectl kustomize apps/prod/firepager >/tmp/firepager-render.yaml
kubectl kustomize apps/prod/firepager/workloads >/tmp/firepager-workloads.yaml
git diff --check
python3 bootstrap/test-safety.py
python3 bootstrap/check-gitops-safety.py
```

Commit and push the scoped fleet change:

```sh
git add apps/prod/firepager/workloads/frontend.yaml
git commit -m "Deploy FirePager operator dashboard"
git push origin main
```

## Reconcile and verify production

Read `CLUSTER_ACCESS.md` first. Use the workload kubeconfig explicitly:

```sh
export PATH=/tmp/opencode/flux-cloud-tools:$PATH
export KUBECONFIG=/tmp/opencode/flux-cloud-bootstrap/workload.kubeconfig
export KUBE_CONTEXT=prod-admin@prod

flux --context "$KUBE_CONTEXT" reconcile kustomization \
  firepager-workloads -n flux-system --with-source

kubectl --context "$KUBE_CONTEXT" -n flux-system \
  get kustomization firepager-workloads -o wide
kubectl --context "$KUBE_CONTEXT" -n firepager \
  rollout status deployment/firepager-frontend --timeout=180s
kubectl --context "$KUBE_CONTEXT" -n firepager \
  get deploy,pods -l app=firepager-frontend -o wide
kubectl --context "$KUBE_CONTEXT" -n firepager get deploy firepager-frontend \
  -o 'jsonpath={.spec.template.spec.containers[0].image}{"\n"}'
```

Verify public endpoints without disabling TLS verification:

```sh
curl --fail --silent --show-error \
  https://firepager.cloud.graffamily.ca/healthz
curl --fail --silent --show-error -I \
  https://firepager.cloud.graffamily.ca/
curl --fail --silent --show-error \
  https://rancher.cloud.graffamily.ca/ping
curl --fail --silent --show-error \
  https://mealie.cloud.graffamily.ca/api/app/about
```

`/healthz` should return `200`; `index.html` should be `no-cache`; hashed
`/assets/` files should be long-lived and immutable.

## Rollback

Revert the fleet manifest to the previous known-good digest, commit, push, and
reconcile `firepager-workloads`. Keep the previous GHCR image available until
the new rollout is validated.

The UI rollout does not require database migrations or PVC changes. If the UI
requires a new backend endpoint, deploy a backward-compatible backend first.
