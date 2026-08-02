# GitLab access bootstrap (one-time)

GitLab projects are private. Create a Personal Access Token, then authenticate
`glab` in WSL. Do not paste the token into chat.

## 1. Create token

1. Open https://gitlab.com/-/user_settings/personal_access_tokens
2. Name: `newa-ci-bootstrap`
3. Scopes (minimum for bootstrap):
   - `api`
   - `read_repository`
   - `write_repository`
   - `read_registry`
   - `write_registry`
4. Create token and copy it once.

Optional later: replace with project/group access tokens with narrower scopes.

## 2. Authenticate in WSL

```bash
wsl.exe -d Ubuntu-24.04 -u alexey -- bash -lc 'export PATH="$HOME/go/bin:$HOME/.local/go/bin:$PATH"; glab auth login --hostname gitlab.com --stdin'
```

When prompted (or via stdin), paste the token. Confirm hostname `gitlab.com`.

Or:

```bash
export GITLAB_TOKEN='glpat-...'   # only in your local shell, not chat
glab auth login --hostname gitlab.com --token "$GITLAB_TOKEN"
```

## 3. Verify

```bash
glab auth status
glab api projects/abc-group4363531%2FNewA | head
```

## 4. After auth, agent continues with

- push `feature/gitlab-cicd-migration` to GitLab remote `gitlab`
- create `abc-group4363531/ProjectStatus`
- push ProjectStatus `master`
- set CI variables (`GITLAB_CD_ENABLED=false`, `STAGING_BASE_URL=http://newa-staging.local`, …)
