# Loop Engine project instructions

This file contains persistent shared instructions for Codex, Claude Code, and compatible agents working in this repository. The Japanese reference version is available at `docs/ja/AGENTS.md`.

## 1. Read first

Before making changes, read at least the task-relevant portions of:

1. `docs/requirements.md`
2. `docs/architecture.md`
3. this `AGENTS.md`

If the design and code conflict, do not silently change the design based on an assumption. Report the conflict. Reflect an agreed specification change in the documentation within the same change as the code.

## 2. Product roles

Loop Engine defines the following standard roles. Roles are not permanently bound to providers.

- Designer: request clarification, requirements, architecture, and implementation planning
- Implementer: milestone implementation, verification, and review remediation
- Reviewer: independent review of requirements, architecture, plans, code, and verification results
- Loop Engine Core: state transitions, job coordination, artifact management, stopping conditions, and recovery
- User: rough request, provider selection, required decisions, approvals, and final acceptance

Do not make Codex and Claude call each other directly. Loop Engine Core must control every workflow transition.

The user may choose the Designer provider that receives the initial request. Designer, Implementer, and Reviewer must always have different role IDs and agent sessions. Even when one provider performs both Designer and Implementer roles, those roles must not share sessions or conversation context. The Reviewer provider must differ from both the Designer and Implementer providers.

## 3. Architectural boundaries

Maintain these boundaries:

- Domain and Workflow do not depend on Herdr, tmux, or a specific AI CLI.
- Provider-specific behavior belongs in Provider Adapters.
- Pane, session, and process behavior belongs in Runtime Backends.
- Git operations belong in the VCS Adapter.
- Project detection belongs in the Project Inspector.
- Verification command execution belongs in the Verification Runner.
- File persistence belongs in the Artifact Store and State Store.

Core must not invoke Herdr or tmux commands directly.

## 4. Source of truth

Do not treat terminal output, an agent's natural-language completion claim, or a screen status label as an authoritative completion condition.

Authoritative state is established by:

- Run ID
- Job ID
- Versioned Artifact
- JSON Schema
- SHA-256
- Verification Result
- Reviewer Verdict
- Workflow State

Treat agent sessions as replaceable execution resources that may be lost. Preserve the ability to resume in a new session from approved artifacts.

## 5. Workflow rules

- Implement only one milestone at a time.
- Limit Requirements, Plan, and Implementation review rounds.
- Never auto-approve after reaching a review limit.
- Never interpret `blocked` as `approved`.
- Reject reviews for stale artifact hashes.
- Do not enter Code Review while Verification is failing.
- Check deterministic gate conditions in addition to the Reviewer verdict.
- Verify Producer and Reviewer provider and session separation.
- Do not resume implicitly from terminal state.

When adding or changing a state transition, update transition tests, invariants, and the recovery path in the same change.

## 6. Safety

Preserve existing user changes. Do not perform the following without an explicit request:

- `git reset --hard`
- `git clean`
- force push
- branch deletion
- bulk deletion of untracked files
- automatic commit
- automatic merge
- production deployment
- secret modification or disclosure

Require a clean worktree by default before an implementation phase. If dirty-worktree support is introduced, capture the starting diff as a baseline and never treat pre-existing changes as Loop Engine output.

## 7. Process execution

- Prefer an executable plus argument array over concatenated shell strings.
- Use behavior equivalent to `shell=false` by default.
- Set the working directory explicitly.
- Propagate timeouts and context cancellation.
- Stream stdout and stderr into job-specific storage.
- Run arbitrary commands only through an approved Command Profile.
- Test signal handling and child-process termination.

Allow a project command that requires a shell only when shell execution has been explicitly configured and approved.

## 8. Reviewer separation

Regardless of whether Codex or Claude Code is assigned, the Reviewer role should normally perform only:

- read access to the project, artifacts, diffs, and verification results
- creation of a review result conforming to the required schema

Do not allow the Reviewer to modify source, Git state, or deployments. If a provider CLI cannot enforce read-only access completely, combine permission settings, restricted writable paths, and Git-diff inspection before and after the job.

## 9. Persistence

- Update state atomically.
- Keep the Event Log append-only.
- Version artifacts instead of overwriting them.
- Detect concurrent updates with the State Revision.
- Allow only one writer per Run through locking.
- Do not automatically adopt an uncommitted artifact during recovery.

## 10. Tests

Add unit tests for new Domain or Workflow behavior.

Add contract tests when changing an Adapter. Normal CI should use Fake Providers and Fake Backends without requiring live AI services.

Maintain coverage for at least:

- approval after one review
- approval after multiple revisions
- review-limit exhaustion
- stale-review rejection
- invalid schemas
- remediation after verification failure
- Reviewer blocked
- resume after abnormal termination
- automatic fallback from Herdr to tmux and Direct

Standard commands:

```text
gofmt -w cmd internal
go test ./...
go vet ./...
go build ./cmd/loop-engine
```

If a sandbox cannot write to the default Go cache, set `GOCACHE` to a writable temporary directory.

## 11. Documentation

Documentation updates are required when changing:

- Workflow States or transitions
- Artifact or Review Schemas
- CLI commands or configuration
- Provider Adapter contracts
- Runtime Backend contracts
- permissions or safety boundaries
- distribution

## 12. Current scope

The MVP is limited to:

- macOS arm64
- Linux amd64
- selecting Codex or Claude Code as the Designer provider
- assigning the selected provider to separate Designer and Implementer sessions
- assigning the other provider as the independent Reviewer
- Herdr, tmux, and Direct Backends
- Supervised, Autonomous, and Design-only modes
- file-based Artifacts, State, and Event Log
- interruption and resume

Additional providers, Web UI, Pull Request integration, and deployment are outside the MVP.
