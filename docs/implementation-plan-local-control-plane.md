# Local Browser Control Plane 実装計画

- 文書版: 0.4.2-draft
- 作成日: 2026-08-02
- 対応設計: `docs/design/local-control-plane.md` 0.4.2-draft、`docs/design/live-progress-and-run-observability.md` 0.2.0-draft
- 状態: 計画完了（実装未着手）

## 1. 実装原則

- 一度に一つのMilestoneだけを実装・検証・Reviewする
- File System境界をUIより先に完成させる
- BrowserとCLIでWorkflowを複製しない
- 実Providerを使わずFakeで通常Testを完結させる
- Node.js、npm、外部CDNをRuntime依存にしない
- FrontendはTypeScript Strict + React + React Router Data Modeで実装し、Full-stack Frameworkを使わない
- Frontend生成物はManifestと再現BuildでSourceとの一致を検査する
- Critical/HighのRequired Changeを解消するまで次Milestoneへ進まない

## 2. Milestone一覧

```text
L0 Contracts and Shared Start Boundary
  -> L1 Workspace Capability and Safe Paths
    -> L2 Intake Store and Markdown Materializer
      -> L3 HTTP Security Foundation
        -> L4 New Request / New Application UI
          -> L5 Run Start, Progress, and Recovery
            -> L6 Packaging and Release Hardening
```

## 3. Milestones

### L0: Contracts and Shared Start Boundary

#### 目的

CLIと将来のHTTP Adapterが同じ保存済みRequest開始Contractを使用できるようにする。

#### Scope

- `StartSavedRequest` Application Service Contract
- Request Path + Expected SHA-256の開始直前照合
- CLI `start --request-file`の共有Contract移行
- Public Domain Error分類
- Fake RunStarter
- Design-only既存挙動の回帰Test

#### Non-scope

- HTTP Server
- UI
- Workspace Directory列挙
- Plan / Implementation Loop自体の新規実装

#### 変更候補

```text
internal/app/start.go
internal/app/service.go
internal/app/errors.go
internal/cli/cli.go
internal/cli/cli_test.go
```

#### Acceptance

- CLIのinline requestとrequest fileが共有Application Contractへ到達する
- Expected Hash不一致ではRunを作成しない
- 既存Design-only Testがすべて通る
- HTTP固有型がApplication/Domainへ入らない

#### Verification

```text
go test ./internal/app ./internal/cli -count=1
go test ./... -count=1
go vet ./...
```

#### Rollback boundary

CLI Adapterを旧Start pathへ戻してもDomain型追加だけが残る構成にする。

### L1: Workspace Capability and Safe Paths

#### 目的

Browserから利用できるFile System範囲を明示Rootへ限定し、Path traversalとSymlink escapeを
OS levelで拒否する。

#### Scope

- repeatable Workspace Root設定
- Root IDとRelative Path型
- Directory query
- Project/parent resolution
- slug validation
- no-follow component traversal
- macOS/Linux Contract Test
- TOCTOU差替えTest
- VCS Metadata DirectoryのDefault非表示・直接指定拒否

#### Non-scope

- File content表示
- Request materialization
- HTTP Routing

#### 変更候補

```text
internal/workspace/root.go
internal/workspace/browser.go
internal/workspace/path.go
internal/workspace/platformfs/
internal/workspace/*_test.go
```

#### Technical gate

macOSとLinuxの双方で、検査後に親PathをSymlinkへ差し替えてもRoot外へDirectory/Fileを
作成できないことを確認する。このGateを満たすまでL2へ進まない。

#### Verification

```text
go test -race ./internal/workspace -count=1
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test ./internal/workspace
```

#### Rollback boundary

Workspace packageは未接続のまま削除可能にする。Portableに安全なno-follow方式が成立しない
場合、Browser Directory選択機能を実装せず再設計する。

### L2: Intake Store and Markdown Materializer

#### 目的

Browser入力をRunより先に監査可能なMarkdown RequestとIntake Metadataとして確定する。

#### Scope

- Intake Domain/State/Revision
- Request/Application入力Validation
- deterministic Markdown materializer
- Atomic Request/Metadata write
- Intake Event Log
- Idempotency Key persistence
- Expected Revision付き`DRAFT -> STARTING` CAS
- 異なるIdempotency Keyによる同一Intake同時Start Test
- New application Directory conflict behavior

#### Non-scope

- HTTP Cookie/CSRF
- Agent起動
- Source Scaffold、Dependency install、Git操作

#### 変更候補

```text
internal/intake/model.go
internal/intake/service.go
internal/intake/materializer.go
internal/store/filesystem/intake.go
internal/intake/*_test.go
```

#### Acceptance

- 同じ正規化入力から同じMarkdown bytesとSHA-256を得る
- `Save draft`でAgent/Runを開始しない
- 同一Idempotency Keyの再送でFileを増やさない
- 異なるIdempotency Keyの同時StartでもRunを一つだけ作る
- 競合PathとSymlinkを変更しない
- Metadata modeが`0600`、Request modeが`0644`

#### Verification

```text
go test -race ./internal/intake ./internal/store/filesystem -count=1
```

#### Rollback boundary

IntakeはRunとは独立保存とし、機能Flagで新規作成を停止しても既存Intakeを読取可能にする。

### L3: HTTP Security Foundation

#### 目的

State変更機能を接続する前に、Loopback、Session、CSRF、Origin、CSP、Body Limitを強制する。

#### Scope

- `rct serve` command shell
- loopback-only listener policy
- generated session bootstrap
- SameSite Strict Cookie
- CSRF token
- exact Host/Origin checks
- GET/HEAD/POST method policy
- JSON envelope and public errors
- CSP/security headers
- embedded minimal diagnostic asset（React UI接続前のSecurity Test用）
- graceful server shutdown

#### Non-scope

- Intake作成Handler
- Run起動Handler
- Remote bind
- Authentication account

#### 変更候補

```text
internal/controlplane/http/server.go
internal/controlplane/http/security.go
internal/controlplane/http/errors.go
internal/controlplane/ui/
internal/cli/serve.go
internal/cli/cli.go
```

#### Acceptance

- `127.0.0.1:0`以外をDefaultで使用しない
- Host/Origin/CSRF/Sessionの各Mutationが独立して拒否される
- GETでState変更Routeを呼べない
- CORS Headerがない
- CSPが外部Resourceとinline scriptを拒否する
- Error Responseに内部Path/秘密情報を含めない

#### Verification

```text
go test -race ./internal/controlplane/http ./internal/cli -count=1
```

#### Rollback boundary

`serve` commandをBuild対象から外してもCLI Workflowが影響を受けないAdapter境界にする。

### L4: New Request / New Application UI

#### 目的

許可RootのDirectory選択、Form入力、Draft保存をBrowserから完了できるようにする。

#### Scope

- TypeScript Strict + TSX project
- React + React DOM
- React Router Data Mode and `/ui` route tree
- Vite production build and manifest
- Home with two primary actions
- Workspace/Directory browser API
- New request form
- New application form
- Confirmation summary
- Intake create API
- Save draft
- Form validation/error summary
- Keyboard/focus/responsive behavior
- typed API client and DTOs
- plain CSS and design tokens
- committed, reproducible embedded production assets

#### Non-scope

- Run start/progress
- Arbitrary file upload
- File content browser
- Rich text editor
- React Router Framework Mode、Next.js、Remix、SSR
- State management library、UI component framework、CSS-in-JS runtime

#### 変更候補

```text
internal/controlplane/http/workspaces.go
internal/controlplane/http/intakes.go
web/embed.go
web/package.json
web/package-lock.json
web/tsconfig.json
web/vite.config.ts
web/index.html
web/src/main.tsx
web/src/router.tsx
web/src/api/
web/src/routes/
web/src/components/
web/src/styles/
web/dist/
```

#### Acceptance

- New requestを既存Projectへ保存できる
- New applicationが新しいDirectoryと`request.md`だけを作る
- Root外/競合/二重Submitが安全に表示される
- JavaScript無効時も安全なError Pageを返す
- 200% Zoomと狭いViewportで主要操作を失わない
- Assetに外部URLが含まれない
- `/ui/*`の直接Access/再読込が成功し、`/api/v1/*`はSPA Fallbackされない
- TypeScript Strict CheckとProduction Buildが成功する
- React Router Framework Modeまたは未承認Runtime Dependencyが含まれない
- 再Build後の`web/dist`に未Commit差分がない

#### Verification

```text
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web test -- --run
npm --prefix web run build
git diff --exit-code -- web/dist
go test -race ./internal/controlplane/... -count=1
go test ./... -count=1
```

#### Rollback boundary

UI Routeを無効化してもIntake Application ServiceはCLI/将来Adapterから利用可能にする。

### L5: Run Start, Progress, and Recovery

L5着手前にLive Progress設計のP0〜P2をCore/CLI Milestoneとして完了する。L5はP3〜P4を実装し、P0〜P4の
Acceptanceがすべて通過した時点で完了とする。

#### 目的

保存済みIntakeからRunを一つだけ開始し、BrowserとCLIで同じStateを確認できるようにする。

#### Scope

- Intake start API
- Idempotent Intake->Run link
- Server-owned Run Manager
- background workflow execution
- Recent runs / run detail
- 共通Progress Query ServiceとCurrent Activity DTO
- Sequence付きSSE ReplayとPolling Fallback
- Current Activity Card、Phase Timeline、Recent Events、Next Action
- Macro Phase GaugeとPlan Binding済みMilestone Gauge
- Previous VerdictとCandidate Versionの分離表示
- Keyboard、Screen Reader、Reduced Motion、Responsive Layout
- Browser disconnect behavior
- server restart rediscovery
- Provider/Backend preflight error display

#### Non-scope

- BrowserからのApproval/Stop/Resume mutation
- Multi-process distributed scheduler
- WebSocket

#### 変更候補

```text
internal/controlplane/runmanager/
internal/controlplane/http/runs.go
internal/app/query.go
internal/store/filesystem/query.go
internal/controlplane/ui/
```

#### Acceptance

- Save and startの二重送信でRunが一つだけ作られる
- 異なるIdempotency KeyでもIntake CASによりRunが一つだけ作られる
- BrowserとCLI statusが同じRun ID/Stateを返す
- Browser disconnectでRunをCancelしない
- Server restart後にRun一覧を再構成できる
- Fake ProviderでRequirements revision loopを最後まで表示できる
- Plan Round 2 Review中にClaude、Reviewer、v2、2/3、Previous Verdictを誤りなく表示する
- `Last-Event-ID`再接続でEventを重複または欠落させない
- SSE不能時のPollingでも同じSnapshotとTerminal Stateへ収束する
- Human Approval待ちを無限Spinnerでなく具体的なNext Actionとして表示する
- Gaugeが完了済みGateだけを数え、Review Roundや実行中JobをPercentage化しない
- Raw Log、Prompt、Credential、絶対PathをBrowser DTOとDOMへ出さない

#### Verification

```text
go test -race ./internal/controlplane/... ./internal/app/... -count=1
go test ./... -count=1
go vet ./...
```

#### Rollback boundary

Run start endpointを無効化してもSave draftとCLI startを維持する。

### L6: Packaging and Release Hardening

#### 目的

単一Binary、macOS/Linux、offline UI、Documentation、Security regressionをRelease可能にする。

#### Scope

- embedded asset verification
- frontend lockfile and production dependency policy
- source-to-dist reproducibility check
- offline/no-external-resource test
- macOS arm64/Linux amd64 build
- command help and README
- request size defaults and configuration
- shutdown/recovery E2E
- security review evidence
- browser smoke test procedure

#### Non-scope

- Hosted deployment
- Remote bind
- Account authentication
- Browser auto-update

#### Acceptance

- `rct serve --help`がRoot/Bind/Security Defaultを説明する
- Binary一つでUIとCoreが起動する
- Node.js/npm/Pythonなしで利用できる
- `/ui/*`の全主要Routeを直接再読込できる
- macOS/Linux buildが成功する
- Critical/HighのSecurity/Architecture Review Findingがない

#### Verification

```text
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web test -- --run
npm --prefix web run build
git diff --exit-code -- web/dist
gofmt -w cmd internal
go test -race ./... -count=1
go vet ./...
CGO_ENABLED=0 go build ./cmd/rct
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/rct
```

#### Rollback boundary

Release Tagを作るまでは配布更新を行わない。問題時は`serve`をExperimentalのまま無効化し、
CLI Binaryの既存機能をRelease可能に保つ。

## 4. Milestone Review contract

各Milestone完了時に独立Reviewerへ次を渡す。

- 承認済みRequirements / Architecture / Plan Hash
- 対象Milestone ID
- 変更File一覧とGit diff
- Test command、exit code、log
- 新規/更新SchemaとContract
- Known limitations
- Rollback boundary

Reviewer verdictが`approved`かつGate Evaluatorが対象Hashと必須検証を確認するまで次へ進まない。

## 5. Plan承認条件

| Milestone | Acceptance Criteria |
|---|---|
| L0 | AC-036, AC-042 |
| L1 | AC-034, AC-037, AC-054 |
| L2 | AC-035, AC-037, AC-038, AC-053 |
| L3 | AC-039, AC-040, AC-043 |
| L4 | AC-035, AC-037, AC-040, AC-044〜AC-048 |
| L5 | AC-036, AC-038, AC-041, AC-042, AC-053, AC-073〜086 |
| L6 | AC-040, AC-043, AC-046, AC-047 |

- File System safetyをUIより先に実装する順序である
- CLI/Coreを複製しない
- 各Milestoneが独立してTest/Review/Rollback可能である
- Browser閉鎖とServer再起動のOwnershipが明確である
- Remote公開を初期Scopeに含めない
- Local Control PlaneのAC-034〜AC-048、AC-053、AC-054が少なくとも一つのMilestoneへ追跡できる
