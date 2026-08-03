# ProjectStatus integration with NewA staging

ProjectStatus is developed and built in a separate repository. NewA owns the
shared `newa-staging` environment and is the only repository that deploys into
that environment.

Immutable releases are `registry/image@sha256:<digest>` (GHCR today; GitLab
Container Registry after cutover). Deploy scripts accept any registry host as
long as the digest form is immutable.

## Release flow

1. ProjectStatus CI tests the service and publishes an immutable GHCR image.
2. With `NEWA_PROMOTION_TOKEN`, ProjectStatus CI opens a pull request that
   writes the image digest to
   `deploy/environments/staging/project-status-image.txt`. Without the token,
   CI remains green and reports that manual promotion is required.
3. Merging the promotion pull request triggers `Deploy ProjectStatus staging`;
   alternatively run that workflow manually with `image@digest`.
4. The workflow deploys only `deployment/project-status`, runs UI, auth, and
   GraphQL smoke checks, and records `project-status-release-state`.

The main NAV workflow ignores a change that consists only of the ProjectStatus
promotion file. Both deploy workflows use the `newa-staging` concurrency group,
so NAV and ProjectStatus cannot mutate the namespace concurrently.

## One-time staging provisioning

After NAV has initialized the application schema, run with an administrative
kubeconfig:

```bash
bash scripts/provision-project-status-staging.sh
```

The script:

- creates or rotates PostgreSQL role `project_status_ro`;
- grants `SELECT` only on the ProjectStatus contract tables;
- creates `newa-staging/project-status-secrets`;
- copies the NAV HMAC key only into that dedicated Kubernetes secret.

The deploy identity cannot provision database roles and does not need
`pods/exec`.

## GitHub Environment settings

Add to NewA environment `staging`:

- secret `PROJECT_STATUS_GHCR_USERNAME`;
- secret `PROJECT_STATUS_GHCR_TOKEN` with package read access to the
  ProjectStatus image;
- existing smoke secrets `SMOKE_COMPANY_NAME`, `SMOKE_USER_NAME`,
  `SMOKE_PASSWORD`;
- existing variable `STAGING_BASE_URL`.

The ProjectStatus repository needs `NEWA_PROMOTION_TOKEN`, scoped to creating
branches and pull requests in NewA.

Current status (2026-07-31): ProjectStatus CI run
`30611794492` is green and publishes GHCR successfully. The promotion token is
not configured yet. NewA staging workflow files exist only in the local working
tree until they are committed and pushed.

## Runtime contract

- public routes remain on the common `newa-staging.local` origin;
- ProjectStatus reads PostgreSQL through `project_status_ro`;
- `companyId` comes only from a verified NAV JWT;
- recalculation goes to `http://nav-api:8090`;
- ProjectStatus does not access RabbitMQ, GSN/FGIS, outbox, or calc job tables.

## Database compatibility

NewA owns schema evolution. Database changes used by ProjectStatus follow:

1. add a backward-compatible schema change;
2. deploy NAV;
3. release and promote ProjectStatus;
4. remove the old contract only in a later NewA release.

Changes to the contract table list must also update
`scripts/provision-project-status-staging.sh`. A future improvement is to expose
versioned read-only views instead of granting the service direct table access.

## Rollback

Use the `Rollback ProjectStatus staging` workflow. With no input, it restores
the image stored in ConfigMap `project-status-release-state`. NAV deployments
are not changed.

The current per-estimate `calc?force=1` route remains an MVP limitation. It is
not a durable bulk orchestration contract; implementing that NAV API is a
separate application change.
