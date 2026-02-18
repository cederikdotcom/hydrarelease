# Runbooks

## Release Pipeline Check

Triggered by: "do the release pipeline check runbook on repo `<name>`"

Validates that a project's continuous deployment pipeline is working end-to-end.

### 1. Identify the project
- Navigate to `/home/cederik/workspaces/<name>`
- Read `go.mod` for module name
- Read `Makefile` for build/deploy targets
- Check if `.github/workflows/release.yml` exists

### 2. Check version state

```bash
git tag --sort=-v:refname | head -5        # latest tags
git log <latest-tag>..HEAD --oneline       # unreleased commits
```

Report: latest tag, number of unreleased commits, summary of changes.

### 3. Check release server

```bash
curl -s https://releases.experiencenet.com/<name>/production/latest.json
```

Does it exist (200) or 404? Does the version match the latest git tag?

### 4. Check deployed version

If the project has a health endpoint:
```bash
curl -s https://<service-url>/api/v1/health
```

Does the deployed version match the released version?

Known health endpoints:
- hydrarelease: `https://releases.experiencenet.com`
- hydratransfer: `https://hydratransfer.experiencenet.com`
- hydrapipeline: `https://hydrapipeline.experiencenet.com`
- hydraexperiencelibrary: `https://experiencenet.com`
- hydracluster: `https://hydracluster.experiencenet.com`

### 5. Check GitHub Actions workflow

```bash
gh run list --limit 5
```

Is the latest run for the latest tag? Did it succeed? If failed: `gh run view <id> --log-failed`

### 6. Report summary

| Check | Status |
|-------|--------|
| Unreleased commits | X commits since vY.Z.W |
| Release server (latest.json) | version / missing |
| Deployed version (health) | version / unreachable |
| GitHub Actions | passed / failed / never ran |

### 7. Recommend actions

- **Unreleased commits exist**: suggest tagging a new version (propose semver bump based on commit content)
- **latest.json missing/outdated**: GitHub Actions likely failed or never triggered — check workflow
- **Deployed != Released**: manual deploy happened, or auto-update hasn't kicked in yet
- **No workflow file**: CD not set up — flag as needing setup

### 8. If user approves a release

Only when explicitly asked:
1. Propose version bump (patch for fixes, minor for features, major for breaking)
2. `git tag v<new> && git push origin v<new>`
3. Monitor: `gh run watch` on the triggered workflow
4. After workflow completes, re-run steps 3-4 to confirm everything propagated
