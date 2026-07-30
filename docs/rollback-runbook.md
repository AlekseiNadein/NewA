# Rollback Runbook (Staging)

## Trigger conditions

Use rollback when deploy has already changed cluster state and one of the following is observed:

- rollout timeout;
- startup/readiness failures;
- `CrashLoopBackOff` or unrecoverable pod failures;
- mandatory smoke checks fail.

## Inputs

- Optional explicit image in format `registry/repository@sha256:<digest>`.
- Without an explicit image, ConfigMap `newa-release-state` supplies the last
  successfully verified digest.

## Procedure (GitHub Actions)

1. Open `Actions → Rollback staging → Run workflow`.
2. Optionally enter a specific image digest.
3. Job updates `nav-api`, `nav-auth`, `nav-calc-worker` to the selected image.
4. Job waits for rollout completion in `newa-staging`.
5. Job runs health and write-scenario verification.

## Controlled rollback exercise

Enable input `exercise` in the manual workflow to validate rollback mechanics:

1. sets intentionally invalid image for `nav-api`;
2. confirms rollout does not become ready;
3. restores `LAST_KNOWN_GOOD_IMAGE`;
4. reruns verification.

Optional variable: `EXERCISE_BAD_IMAGE` (defaults to a non-existent registry image).

## Post-rollback actions

1. Open incident/task with rollback reason and affected commit.
2. Create fix/revert MR to align source of truth with stable state.
3. Re-run pipeline for corrected commit.
4. Attach rollout and smoke evidence.

## Safety constraints

Do not run automatic rollback for known non-reversible data migrations without owner-approved runbook.
