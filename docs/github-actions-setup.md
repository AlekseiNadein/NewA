# GitHub Actions (mirror after GitLab cutover)

GitLab is the **active** staging CD (`docs/ci-cd-decision.md`,
`docs/gitlab-cicd-setup.md`). GitHub Actions may still run
validate / test / security / publish to GHCR as a mirror.

**Do not re-enable** `deploy` / `verify` jobs on GitHub while
`GITLAB_CD_ENABLED=true` on GitLab.

## What still runs on GitHub

- `validate`, `test`, `security` on PR and `main`
- `publish` to `ghcr.io/.../nav-saas` on `main` (optional mirror artifact)
- `deploy` / `verify`: `if: false`
- ProjectStatus / rollback staging workflows: disabled or unused for CD

## Runner note

Self-hosted labels `self-hosted`, `Linux`, `newa-staging` historically served
GitHub deploy. After cutover the **same WSL host** runs the GitLab runner with
tag `newa-staging` for deploy/smoke. Keep WSL/k3s up via
`deploy/k3s/start-wsl-k3s.ps1`.

## Historical setup

Environment `staging`, GHCR publish, and smoke secrets were configured for the
temporary GitHub CD path. Prefer GitLab CI variables for active smoke/registry
credentials. Do not delete GitHub Environment blindly until ProjectStatus
promotion fully leaves GHCR.

Details of the old runner install remain useful for host maintenance; see git
history of this file before cutover and `scripts/install-github-runner.sh`.
