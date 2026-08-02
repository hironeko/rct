# Document Output / Local Preview 実装計画

- 文書版: 0.2.0-draft
- 作成日: 2026-07-31
- 入力要件: `docs/requirements.md` 0.9.0-draft
- 入力設計: `docs/design/document-output.md` 0.2.0-draft
- 状態: 計画完了（実装未着手）

## 1. 実装方針

- 一度に一つのMilestoneだけを実装する
- 各Milestoneの最後にUnit Test、`go test ./...`、`go vet ./...`を実行する
- Domain、Store、Compiler、HTTP Serverの責務を分離する
- 高RiskなFile安全性を先に検証する
- Review Subject移行をCompiler UIより先に完成させる
- 各Milestoneを独立してRevert可能にする
- 独立Code ReviewのRequired Changeを解消してから次へ進む

## 2. 依存順

```text
M0 Protocol Baseline
  -> M1 Safe Publication Spike
      -> M2 Output Root and Manifest
          -> M3 Markdown Artifact Review Pipeline
              -> M4 Static HTML/CSS Compiler
                  -> M5 Local Preview Server
                      -> M6 Integration and Release Hardening
```

## 3. Milestone

### M0: Artifact Protocol Versioning

#### 目的

既存JSON Subject Runと新しいMarkdown Subject Runを混在させないDomain境界を作る。

#### Scope

- `Run.ArtifactProtocolVersion`
- 新規Runの`document-v1`
- FieldなしRunの`json-v1`判定
- `ReviewSubject.MediaType`
- Review Schemaの`media_type: text/markdown`
- Protocol不一致Error
- State/Schema/Prompt Contract Test
- `status --json`へのProtocol表示

#### Non-scope

- Markdown生成
- Output Rootへの書込
- 旧Runの自動Migration

#### 主な変更候補

```text
internal/domain/run.go
internal/domain/review.go
internal/app/design.go
schemas/review.schema.json
agent-assets/roles/reviewer.md
internal/**/**_test.go
```

#### 受け入れ条件

- 新規Runが`document-v1`を保存する
- FieldなしStateを`json-v1`として識別できる
- `document-v1` Reviewは`text/markdown`以外を拒否する
- JSON HashをMarkdown Protocol RunのReviewへ適用できない
- 既存State DecodeがPanicしない

#### 検証

```text
go test ./internal/domain ./internal/app ./internal/providers
go test ./...
go vet ./...
```

#### 完了定義

Protocol混在を防ぐTestが成功し、まだ既存Design Flowの出力内容を変更していない。

#### Rollback

Protocol FieldとSchema変更を一括Revertできる。一部だけ残さない。

---

### M1: Safe Publication Filesystem Spike

#### 目的

macOS/Linuxで、Output Root外やSymlink先へ書かず、既存Fileを無警告で上書きしない
File Primitiveを先行検証する。

#### Scope

- `internal/platformfs` Interface
- Darwin/Linux Build Tag実装
- Root Directory FD
- `openat`/`mkdirat`/`renameat`/`O_NOFOLLOW`候補の検証
- Relative Path正規化
- Existing Component/Target Symlink拒否
- Same-directory Temporary FileとAtomic Replace
- File/Directory `fsync`
- Hash Compare-and-Swap
- Concurrent replacement Test

#### Non-scope

- Workflow State遷移
- Publication Manifest Domain
- Markdown内容

#### 技術判断Gate

次をmacOS arm64とLinux amd64の両方でTestできなければ、M2へ進まない。

1. Symlink Targetを辿らない
2. Symlink Parent Componentを辿らない
3. `../`とAbsolute Pathを拒否する
4. Existing Hash不一致時に原文を保持する
5. Temporary Fileが同一Directoryでrenameされる
6. Error時にTemporary Fileを回収する

#### 主な変更候補

```text
internal/platformfs/fs.go
internal/platformfs/fs_darwin.go
internal/platformfs/fs_linux.go
internal/platformfs/fs_test.go
internal/platformfs/fs_unix_test.go
go.mod
```

#### 受け入れ条件

- AC-027を再現する自動Testが成功する
- 利用者Fileの内容を比較失敗時に一Byteも変更しない
- Unsupported Platformは安全側に明示Errorとなる
- `go test -race`でData Raceがない

#### 検証

```text
go test -race ./internal/platformfs
GOOS=linux GOARCH=amd64 go build ./...
```

#### 完了定義

Technical Spike結果を`docs/spikes/safe-publication-filesystem.md`へ記録し、独立Reviewerが
安全性をReviewできる。

#### Rollback

独立Packageのため、採用しない場合は他Milestoneへ影響せず削除できる。

---

### M2: Output Root Resolution and Publication Manifest

#### 目的

Request File基準のOutput Root解決と、Managed Publication CopyのOwnership/Hash
管理をWorkflowへ導入する。

#### Scope

- `StartOptions.RequestSourcePath` / `OutputDir`
- `Run.OutputRoot` / `PublicationManifestPath`
- FR-168の優先順位
- CLI `start --output-dir`
- `doctor --request-file --output-dir`
- Output Rootの起動時固定とresume用保存
- Publication Manifest Domain/Store
- `PublicationManager`
- Conflict Errorと`WAITING_FOR_HUMAN`
- `status`へのOutput Root/Conflict表示

#### Non-scope

- Requirements Markdown生成
- HTML Compiler
- Manual `adopt/replace` Commandの完成

#### 主な変更候補

```text
internal/cli/cli.go
internal/app/service.go
internal/domain/run.go
internal/domain/publication.go
internal/store/filesystem/publication.go
internal/publication/manager.go
```

#### 受け入れ条件

- AC-016〜018が自動Testで成功する
- Explicit `--output-dir`がRequest Parentより優先される
- Request File ParentがProject Rootより優先される
- 解決済みRootがStateと`start`出力へ現れる
- resumeでCWDを変更してもRootが変わらない
- Unowned Existing Fileで停止する
- 同一Run PublicationのHash不一致で停止する

#### 検証

```text
go test ./internal/cli ./internal/app ./internal/publication ./internal/store/filesystem
go test ./...
go vet ./...
```

#### 完了定義

空のTest Documentを安全にPublishできるが、Production WorkflowはまだMarkdownへ移行
していない。

#### Rollback

Runの新FieldはOptional Decodeとし、CLI Flagを削除しても旧Stateを読める。

---

### M3: Requirements Markdown Artifact and Review Migration

#### 目的

Schema検証済みRequirements JSONからVersioned Markdownを生成し、Reviewerの対象を
Markdownへ切り替える。

#### Scope

- Typed `RequirementsDocument`
- `RequirementsMaterializer`
- Deterministic GFM Template
- `vNNN.json`監査Copyと`vNNN.md`Snapshot
- Output Root `requirements.md` Publication
- Markdown Path/Hash/Media Type Review Subject
- Reviewer PromptのMarkdown入力化
- changes_requested時の前回Markdown引き継ぎ
- Snapshot/Publication Hash Gate
- Publication Conflict時のReviewer未起動保証
- Golden Test

#### Non-scope

- Architecture/Plan Materializer
- HTML/CSS
- Manual Markdown一般Parser

#### 主な変更候補

```text
internal/domain/requirements.go
internal/documents/materializer.go
internal/documents/requirements.go
internal/app/design.go
schemas/requirements.schema.json
schemas/review.schema.json
agent-assets/roles/designer.md
agent-assets/roles/reviewer.md
```

#### 受け入れ条件

- Valid JSONから同じMarkdown bytesを繰り返し生成する
- Versioned Markdownを上書きしない
- Review Subject HashがMarkdown Hashと一致する
- Reviewer PromptがJSONでなくMarkdown本文を評価対象にする
- Publication CopyとSnapshotが同一Hashである
- 手編集後のRevisionで既存内容を保持して停止する
- AC-025/026が成功する

#### 検証

```text
go test ./internal/documents ./internal/app ./internal/publication
go test ./...
go vet ./...
```

Fake Providerを用いて`changes_requested -> v002 -> approved`を通す。

#### 完了定義

Design-only Direct Flowの正式ArtifactがMarkdownとなり、旧JSON Subjectが新規Runへ
現れない。

#### Rollback

M0 Protocol VersionでRun Formatが分離されるため、`document-v1`だけを無効化できる。

---

### M4: Static Markdown to HTML/CSS Compiler

#### 目的

MarkdownをNetwork不要の読みやすい静的HTML/CSSへ変換する。

#### Scope

- `github.com/yuin/goldmark`導入
- FR-160のGFM Subset
- Auto Heading ID
- Table/Task List/Strikethrough/Linkify
- Raw HTMLと危険URLの無効化
- Semantic Class/Data Attribute
- TOCとMulti-page Navigation
- Embedded Default/Print CSS
- Custom CSS Policy
- Local Image Asset Copy
- `preview-manifest.json`
- `rct render`
- `--source` / `--destination` / `--json`

#### Non-scope

- HTTP Server
- Mermaid、数式、Footnote
- Remote Asset Download

#### 主な変更候補

```text
internal/documents/compiler.go
internal/documents/site.go
internal/documents/url_policy.go
internal/documents/css_policy.go
internal/documents/assets/
internal/cli/cli.go
cmd/rct/main.go
```

#### 受け入れ条件

- AC-019〜024がHTTP部分を除いて成功する
- Same Input/Version/ThemeからByte-identical Outputを生成する
- Raw HTML、`javascript:`、External Scriptを出力しない
- Default ThemeがExternal Resourceを参照しない
- Source Markdownを変更しない
- Existing Destination Conflictを安全側に扱う
- Semantic ClassとHeading FragmentをDOM Testで確認できる

#### 検証

```text
go test ./internal/documents ./internal/cli
go test ./...
go vet ./...
```

Golden Markdown/HTML、URL Fuzz Test、Path Fuzz Testを追加する。

#### 完了定義

単一FileとDirectory Siteを`render`でき、BrowserなしでもOutput構造を自動検証できる。

#### Rollback

CompilerはWorkflowと独立Package/Commandのため、Document Artifact生成を残して無効化
できる。

---

### M5: Read-only Local Preview Server

#### 目的

生成済みSiteを安全にLocal Browserで確認できるようにする。

#### Scope

- `rct preview`
- 初回Render
- `127.0.0.1:0`
- URL表示
- Host Header検証
- Origin完全一致
- GET/HEAD限定
- CORSなし
- Read-only Static File配信
- Directory Traversal拒否
- Graceful Shutdown
- Preview stale表示

#### Non-scope

- Browser自動Open
- Remote Listen
- State変更API
- Live Agent Progress UI

#### 主な変更候補

```text
internal/preview/server.go
internal/preview/policy.go
internal/cli/cli.go
cmd/rct/main.go
```

#### 受け入れ条件

- AC-028が自動Testで成功する
- GET/HEAD以外は405
- Host/Origin不一致は拒否
- `../`でDestination外を読めない
- Context Cancelで一定時間内に終了する
- ServerがProjectまたはRun Stateを変更しない

#### 検証

```text
go test -race ./internal/preview
go test ./...
go vet ./...
```

#### 完了定義

Local URLでSiteを閲覧でき、Security Policyを`httptest`で再現できる。

#### Rollback

`preview` Commandだけを無効化しても`render` Outputは利用できる。

---

### M6: Integration, Documentation, and Release Hardening

#### 目的

Document Flow全体をmacOS/Linuxで再現し、公開可能な品質へ仕上げる。

#### Scope

- Requirements→Review→Publication→Render E2E
- Codex Designer / Claude Reviewer方向
- Claude Designer / Codex Reviewer方向
- HelpとUsage Example
- `--help` Exit Code 0修正
- Config例
- Migration/Conflict Recovery手順
- License/Dependency Notice
- macOS arm64/Linux amd64 Build
- Race/Vet/Test
- CI Workflow

#### Non-scope

- Architecture/Plan/Implementation Loop自体の新規完成
- Herdr/tmux実制御
- Gemini Provider

#### 受け入れ条件

- AC-016〜028をTraceできる
- `rct start --help`が0で終了する
- `render`/`preview` HelpにDefault PathとSecurity Boundaryが出る
- 両Provider方向の実E2E記録がある
- macOS arm64/Linux amd64 Binaryが生成できる
- RuntimeへGo/Node/Python/Pandocを要求しない
- Third-party Licenseを同梱する

#### 検証

```text
go test ./...
go test -race ./...
go vet ./...
GOOS=darwin GOARCH=arm64 go build ./cmd/rct
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/rct
```

#### 完了定義

Release Candidateを独立ReviewerがCode/Artifact Reviewし、Critical/Highがなく、全必須Gateが
成功している。

#### Rollback

Release Tagを作成するまでDistributionを更新しない。失敗時は最後に承認された
Milestone Binaryへ戻す。

## 4. MilestoneごとのIndependent Review

各Milestone完了時に次を渡す。

- 対象Milestone
- Requirement/Design Trace
- Git Diff
- Test CommandとExit Code
- 新規/変更Artifact Schema
- Known Limitation
- Security Boundary変更
- 次Milestoneへの影響

Verdictが`approved`でなければ次Milestoneへ進まない。

## 5. Risk順序

| Risk | 先行対応 |
|---|---|
| 利用者File破損 | M1で最初に検証 |
| JSON/Markdown Review混在 | M0とM3でProtocol分離 |
| Markdown HTML Injection | M4 Golden/Fuzz Test |
| Preview Local攻撃 | M5 Policy Test |
| 既存Run互換 | M0 Protocol Version |
| Platform差 | M1とM6のmacOS/Linux Test |

## 6. Plan承認条件

- M0〜M6の依存順が妥当
- 各Milestoneが独立して検証・Review可能
- Data Loss RiskがCompiler UIより先に検証される
- FR-160〜179とAC-016〜028に未割当がない
- Rollback/停止境界が明確
- 実装前に未決のCritical/Highがない
