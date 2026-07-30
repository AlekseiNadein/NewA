# Rollback Runbook (Staging)

## Trigger conditions

Use rollback when deploy has already changed cluster state and one of the following is observed:

- rollout timeout;
- startup/readiness failures;
- `CrashLoopBackOff` or unrecoverable pod failures;
- mandatory smoke checks fail.

## Inputs

- `KUBE_CONTEXT` CI variable.
- `LAST_KNOWN_GOOD_IMAGE` (format: `registry/repository@sha256:<digest>`).

## Procedure (GitLab manual job)

1. Start `rollback:staging` manual job in pipeline for `main`.
2. Job updates `nav-api`, `nav-auth`, `nav-calc-worker` to `LAST_KNOWN_GOOD_IMAGE`.
3. Job waits for rollout completion in `newa-staging`.
4. Job runs `scripts/verify-staging.sh`.

## Post-rollback actions

1. Open incident/task with rollback reason and affected commit.
2. Create fix/revert MR to align source of truth with stable state.
3. Re-run pipeline for corrected commit.
4. Attach rollout and smoke evidence.

## Safety constraints

Do not run automatic rollback for known non-reversible data migrations without owner-approved runbook.
