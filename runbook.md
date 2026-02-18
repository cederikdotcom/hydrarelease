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
- hydraexperiencelibrary: `https://hydraexperiencelibrary.experiencenet.com`
- hydracluster: `https://hydracluster.experiencenet.com`

### 5. Check GitHub Actions workflow

```bash
gh run list --limit 5
```

Is the latest run for the latest tag? Did it succeed? If failed: `gh run view <id> --log-failed`

### 6. Validate auto-updater

SSH into the production server and manually trigger the update command to verify the self-updater works:

```bash
ssh root@<server-ip> "<name> check-update"
```

If an update is available, trigger it:
```bash
ssh root@<server-ip> "echo yes | <name> update"
```

Then re-check the health endpoint to confirm the version changed. If the updater fails, check:
- Does the binary have write access to itself? (`ls -la $(which <name>)`)
- Can it reach the release server? (`curl -s https://releases.experiencenet.com/<name>/production/latest.json`)
- Check systemd journal for errors: `journalctl -u <name> --since '10 min ago' --no-pager`

Known server IPs:
- hydratransfer: `ssh root@hydratransfer.experiencenet.com`
- hydracluster: `ssh root@hydracluster.experiencenet.com`
- hydrapipeline: `ssh root@hydrapipeline.experiencenet.com`
- hydrarelease: `ssh root@46.225.120.7`
- hydraexperiencelibrary: `ssh root@hydraexperiencelibrary.experiencenet.com`

### 7. Audit docs and memory for stale instructions

Check the repo's CLAUDE.md and any memory files for instructions that contradict the CD pipeline:
- References to `make deploy` for production (should be tag → CI)
- Manual rsync/scp deploy instructions
- Hardcoded version numbers that will go stale
- Missing or incorrect release/deploy documentation

Also check `/home/cederik/.claude/projects/-home-cederik-workspaces/memory/` for stale references to this project (outdated versions, old deploy procedures).

Fix any issues found: update CLAUDE.md to point at the CI pipeline, remove manual deploy instructions, replace hardcoded versions with references to auto-update.

### 8. Report summary

| Check | Status |
|-------|--------|
| Unreleased commits | X commits since vY.Z.W |
| Release server (latest.json) | version / missing |
| Deployed version (health) | version / unreachable |
| GitHub Actions | passed / failed / never ran |

### 9. Recommend actions

- **Unreleased commits exist**: suggest tagging a new version (propose semver bump based on commit content)
- **latest.json missing/outdated**: GitHub Actions likely failed or never triggered — check workflow
- **Deployed != Released**: manual deploy happened, or auto-update hasn't kicked in yet
- **No workflow file**: CD not set up — flag as needing setup

### 10. If user approves a release

Only when explicitly asked:
1. Propose version bump (patch for fixes, minor for features, major for breaking)
2. `git tag v<new> && git push origin v<new>`
3. Monitor: `gh run watch` on the triggered workflow
4. After workflow completes, re-run steps 3-4 to confirm everything propagated
