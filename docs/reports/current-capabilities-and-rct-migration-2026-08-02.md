# rct 現状・名称移行報告書

- 作成日: 2026-08-02
- Repository: `github.com/hironeko/rct`
- CLI Version: `0.5.0-dev`
- 対象OS: macOS / Linux
- 実装言語: Go

## 1. 概要

旧名称から`rct`への移行をRepository全体へ適用した。利用者向け名称だけでなく、CLI、Go module、
内部State Directory、設定Path、Session ID Prefix、Schema URN、Agent指示、設計文書、README画像を
同じ名称へ統一している。

レビュー依頼・結果・対応記録として作成していた`docs/reviews/`は現在TreeおよびGit履歴から削除済みで、
`.gitignore`により再登録を防止している。レビューで確定した要件・設計・安全対策は、正式な要件書、
設計書、Schema、実装、Testへ反映された状態を維持している。

## 2. 現在実装済みの機能

### 2.1 CLIとRun初期化

- `rct start`
- `rct plan`
- `rct approve`
- `rct implement`
- `rct doctor`
- `rct status`
- `rct version`
- `rct help`
- 要望のCommand引数、Markdown File、標準入力からの受付
- Supervised、Autonomous、Design-only Mode
- CodexまたはClaude CodeをDesignerとして選択可能
- DesignerとImplementerを別Sessionへ分離
- Reviewerを別Provider・別Sessionへ分離

### 2.2 Requirements / Architecture / Plan Loop

- Requirements生成
- 独立Requirements Review
- `changes_requested`に対する有限Revision Loop
- Architecture生成・独立Review・有限修正
- Implementation Plan生成・独立Review・有限修正
- JSON Schema Validation
- Artifact Path、SHA-256、Review Subjectの一致Gate
- Review上限到達、`blocked`、Stale Hashでの安全停止

### 2.3 Human Approval Gate

- 承認済みImplementation Planの正確なSHA-256へHuman ApprovalをBinding
- Approval Recordの永続化
- Run単位File LockとState Revision CAS
- 同時承認時に一件だけを成功させる排他制御
- Reviewer否決、検証失敗、Stale Artifactを通常承認でOverrideしない制御

### 2.4 Implementation / Verification / Code Review Loop

- Plan順に一つのMilestoneだけを実装
- Clean Git Worktreeの要求
- Git HEADとIndexの無断変更検知
- Verification成功前のCode Review禁止
- Verification Executableの組込みAllowlist
- Verification Processへ渡すEnvironmentの明示Allowlist
- stdout、stderr、Exit Code、Attemptの永続化
- 累積Git DiffとSize制限付き未追跡FileをReviewer Evidenceへ収集
- 日本語、空白、Rename/Copyを扱えるNUL区切りGit Status Parser
- Code Reviewの`changes_requested`に対する有限Remediation Loop
- 全Milestone完了後のFinal Verification
- Requirements、Architecture、Plan、累積Diff、検証結果を使ったFinal Review
- Final Review承認時だけ`COMPLETED`へ遷移

### 2.5 Provider / Runtime / Persistence

- Codex CLI Adapter
- Claude Code CLI Adapter
- Role別Workspace Write / Read-only Permission Profile
- Herdr、tmux、Direct Backendの検出と優先順位選択
- Direct BackendでのCore Loop実行
- `.rct/runs/<run-id>/`へのRun State、Artifact、Job、Review、Verification、Approval保存
- Atomic File Write
- Append-only Event Log
- State Revision CAS
- Provider CLIの既存認証を利用し、rct自身はAPI Keyを保存しない方針

### 2.6 Build / Install / Release

- `make build`によるVersion埋込Build
- `make install`による`~/.local/bin/rct`への標準Install
- `PREFIX`によるInstall先変更
- `make uninstall`によるBinary単体の安全な削除
- macOS/Linuxとarm64/amd64を判定するRelease Installer
- GitHub ReleaseのSHA-256照合後だけBinaryをInstallする検証
- Installer / UninstallerのLocal Integration Test
- Push / Pull Requestで`make check`を実行するCI
- `v*` Tagから4 PlatformのArchiveとChecksumを生成するRelease Workflow

## 3. 設計済み・未実装の機能

次は要件・詳細設計・実装計画まで完成しているが、現時点のBinaryには未実装である。

- React + React Router Data Mode + TypeScript StrictによるLocal Browser Control Plane
- `rct serve`
- BrowserからのNew request / New application作成
- Markdownから静的HTML/CSSを生成するDocument Compiler
- `rct render`
- `rct preview`
- Markdown Artifact ProtocolとOutput Root Publication
- Herdr/tmux上でのManaged Session実行
- `resume`、`stop`、`logs`、`reject`、`answer`
- Agent AssetのInstall / Update Command

## 4. rct名称移行の変更点

| 領域 | 変更後 |
|---|---|
| Project名 | `rct` |
| Repository | `github.com/hironeko/rct` |
| Go module | `github.com/hironeko/rct` |
| CLI Binary | `rct` |
| Command Entry | `cmd/rct` |
| Run State | `.rct/` |
| Project Config | `.rct.toml` |
| User Config | `$XDG_CONFIG_HOME/rct/config.toml` |
| Session Prefix | `rct-` |
| Schema URN | `urn:rct:job-output-schema` |
| Concept Image | `docs/assets/rct-control-plane-concept.png` |

旧CLI名、旧Go module、旧State Directoryへの互換Aliasは設けていない。現段階はPre-releaseであり、
新名称を正式な唯一のContractとして扱う。

## 5. 検証結果

```text
go test -race ./... -count=1                       PASS
go vet ./...                                      PASS
go build ./cmd/rct                                PASS
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build    PASS
rct --help                                        PASS
rct version                                       0.5.0-dev
make test-installer                              PASS
git diff --check                                  PASS
```

## 6. 次の推奨工程

Core Loopは実装・検証済みである。次は共有Application Serviceの境界を維持したまま、次の順序を推奨する。

1. Local Browser Control Plane L0: Shared Start Contract
2. Workspace/File System Safety
3. Intake PersistenceとIdempotency/CAS
4. HTTP Security Foundation
5. React/TypeScript UI
6. Run Progress / Recovery表示
7. Document CompilerとLocal Preview

Browser UIやDocument Compilerを追加しても、Workflow StateとGate判定の正本はGo Coreに置き、
CLIとWebでWorkflowを複製しない。
