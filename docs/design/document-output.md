# Document Artifact / Local Preview 詳細設計

- 文書版: 0.2.0-draft
- 作成日: 2026-07-31
- 対応要件: `docs/requirements.md` 0.9.2-draft FR-160〜FR-179
- 関連ADR: `docs/architecture.md` ADR-009
- 実装状態: 未実装

## 1. 目的

rctが生成する要件定義書、設計書、実装計画などを、開発者とGitHubが読む
Markdownとして安全に公開する。ローカルでは同じMarkdownから、読みやすい静的
HTML/CSSを生成する。

本設計は次の三層を明確に分ける。

| 層 | 形式 | 役割 | Review対象 |
|---|---|---|---|
| Job Result | JSON | Agent出力、Schema検証、監査証跡 | いいえ |
| Document Artifact | Markdown | 人間、Git、Agent引き継ぎの文書 | はい |
| Preview Artifact | HTML/CSS | ローカル閲覧用の派生物 | いいえ |

## 2. スコープ

### 2.1 対象

- Requirements JSONから決定的なMarkdownを生成する
- Versioned Markdown SnapshotをArtifact Storeへ保存する
- Output Rootを解決してManaged Publication Copyを作る
- Publication前のOwnership、Hash、Symlink検査
- Markdownから静的HTML/CSS Siteを生成する
- Read-onlyなLoopback Preview Server
- `start --output-dir`、`render`、`preview`、`doctor`、`status`の拡張
- JSON SubjectからMarkdown SubjectへのProtocol移行

### 2.2 対象外

- Markdownから構造化JSONへ自動Round-tripする一般Parser
- WYSIWYG Editor
- BrowserからのDocument編集、承認、Agent操作
- Mermaid、数式、Footnote、Raw HTMLの実行
- Remote Preview公開
- GitHubへの自動PushまたはPull Request作成
- 複数Runが同じPublication Fileを共同所有すること

## 3. 設計原則

1. Agentが返したJSONを直接Review Subjectにしない
2. Schema検証成功後にだけMarkdownをMaterializeする
3. Review Subjectは不変のVersioned Markdown Snapshotにする
4. Output RootのFlat MarkdownはSnapshotと同一bytesのPublication Copyにする
5. Publication CopyはCompare-and-Swap型のHash検査なしに更新しない
6. HTML/CSSからWorkflow状態を復元しない
7. Markdown変換とPreviewに外部RuntimeやNetworkを要求しない
8. Source、Destination、Runtime StateのPathを混同しない

## 4. コンポーネント

```text
CLI
 ├─ start ──> OutputResolver
 │             │
 │             v
 │          Workflow Engine
 │             │
 │       Structured Job Result
 │             │
 │             v
 │      DocumentMaterializer
 │             │
 │       Versioned Snapshot
 │             │
 │       ┌─────┴───────────┐
 │       v                 v
 │  Artifact Store   PublicationManager
 │                         │
 │                         v
 │                  Output Root/*.md
 │
 ├─ render ─> SourceDiscovery ─> MarkdownCompiler ─> StaticSiteBuilder
 │
 └─ preview ───────────────────────────────────────> PreviewServer
```

### 4.1 OutputResolver

責務:

- FR-168の優先順位でOutput Rootを解決する
- CLI相対Pathを起動時CWD基準で絶対Pathへする
- 明示指定されたRoot Symlinkを一度だけ物理Pathへ解決する
- 解決結果をRunへ保存する
- 書込可否と既存競合を事前診断する

Core WorkflowはCurrent Working DirectoryからOutput Rootを再計算しない。

### 4.2 DocumentMaterializer

責務:

- Schema検証済みの論理Documentを型付きGo ValueへDecodeする
- Document種別ごとのTemplateでGFM Markdownを生成する
- 見出し、Section順序、ID表記、Table Columnを決定的にする
- UTF-8、LF、末尾改行一つへ正規化する
- Materializer versionをMetadataへ記録する

初期実装はRequirementsだけに対応し、Architecture、Plan、Milestoneは同じInterfaceへ
後続実装を追加する。

```go
type Materializer interface {
    Kind() DocumentKind
    Version() string
    Materialize(ctx context.Context, input StructuredArtifact) (MarkdownDocument, error)
}
```

Materializerは生成AIを呼ばず、入力にない結論を追加しない。

### 4.3 VersionedArtifactStore

責務:

- `artifacts/<kind>/vNNN.md`を新規作成する
- Versioned Snapshotを上書きしない
- Markdown SHA-256と元Job Result SHA-256をMetadataへ記録する
- Snapshot確定後にだけReview Jobへ進める

初期Requirements Layout:

```text
.rct/runs/<run-id>/
├── artifacts/
│   └── requirements/
│       ├── v001.json
│       ├── v001.md
│       ├── v002.json
│       └── v002.md
├── publication-manifest.json
└── jobs/
```

JSONはJob Resultの監査Copy、MarkdownはDocument Artifactである。

### 4.4 PublicationManager

責務:

- Output Root内だけへPublication Copyを作る
- Internal Snapshotと同一bytesをPublishする
- Publication ManifestによるOwnershipとHashを検証する
- File競合を分類する
- 原子的に更新する
- 利用者編集を検出した場合にWorkflowへConflictを返す

```go
type PublicationManager interface {
    Publish(ctx context.Context, req PublishRequest) (PublicationRecord, error)
    Inspect(ctx context.Context, req InspectRequest) (PublicationStatus, error)
}
```

Conflict分類:

```text
unowned_existing_file
owned_but_modified
symlink_component
path_escaped_root
root_unavailable
concurrent_replacement
```

Conflictは通常のAgent失敗と区別し、Runを`WAITING_FOR_HUMAN`へ遷移させる。

### 4.5 MarkdownCompiler

責務:

- FR-160で定義したMarkdown/GFM SubsetをParseする
- 安全なHTML Fragmentへ変換する
- 見出しIDと目次情報を生成する
- Linkと画像Pathを検証・変換する
- Document種別と意味要素へ安定したClass/Data Attributeを付ける

候補実装は純Goの`github.com/yuin/goldmark`とする。

- `extension.GFM`でTable、Strikethrough、Linkify、Task Listを有効化する
- `parser.WithAutoHeadingID()`でHeading IDを生成する
- `html.WithUnsafe()`を使用しない
- 危険URLを許可する独自Renderer Optionを使用しない
- 必要なSemantic ClassはAST TransformerまたはNode Rendererで付与する

GoldmarkのDefault RendererはRaw HTMLと危険URLを出力しない。Compiler Contract Test
ではDefault依存を無条件に信頼せず、危険入力をGolden Testで固定する。

### 4.6 StaticSiteBuilder

責務:

- 一つまたは複数のMarkdownをHTML Pageへ包む
- Document一覧と相互Navigationを作る
- Default CSSとPrint CSSを出力する
- `preview-manifest.json`を生成する
- Source Hashが同じ場合に決定的な出力を作る

HTML WrapperはGoの`html/template`を使用する。Templateへ渡すHTML Fragmentは
MarkdownCompilerだけが生成し、任意文字列を`template.HTML`へ変換しない。

### 4.7 PreviewServer

責務:

- 指定DestinationをRead-onlyで配信する
- `127.0.0.1:0`をDefault Listen Addressとする
- 実PortとURLを起動後に表示する
- GET/HEAD以外を405で拒否する
- HostとOriginを検証する
- CORS Headerを付与しない
- Directory外Pathを配信しない
- 状態変更Endpointを提供しない
- Context cancellationでGraceful Shutdownする

## 5. Domain Model

### 5.1 Run拡張

```go
type Run struct {
    // existing fields
    ArtifactProtocolVersion string `json:"artifact_protocol_version"`
    RequestSourcePath       string `json:"request_source_path,omitempty"`
    OutputRoot              string `json:"output_root"`
    PublicationManifestPath string `json:"publication_manifest_path"`
}
```

新規Runの`ArtifactProtocolVersion`は`document-v1`とする。既存のFieldなしRunは
`json-v1`として扱い、同じRun内で自動変換しない。

### 5.2 MarkdownDocument

```go
type MarkdownDocument struct {
    Kind                DocumentKind
    MediaType           string
    MaterializerVersion string
    Bytes               []byte
    SHA256              string
    SourceResultSHA256  string
}
```

`MediaType`は`text/markdown; variant=GFM`とする。

### 5.3 PublicationRecord

```go
type PublicationRecord struct {
    ArtifactKind         DocumentKind `json:"artifact_kind"`
    ArtifactVersion      int          `json:"artifact_version"`
    VersionedPath        string       `json:"versioned_path"`
    VersionedSHA256      string       `json:"versioned_sha256"`
    PublicationPath      string       `json:"publication_path"`
    LastPublishedSHA256  string       `json:"last_published_sha256"`
    OwningRunID          string       `json:"owning_run_id"`
    MaterializerVersion  string       `json:"materializer_version"`
}
```

Manifestの`LastPublishedSHA256`は次回PublishのCompare値になる。

### 5.4 Review Subject

Review SchemaのSubjectを次へ更新する。

```json
{
  "path": ".rct/runs/<run-id>/artifacts/requirements/v002.md",
  "sha256": "<markdown-sha256>",
  "media_type": "text/markdown"
}
```

Reviewer PromptにはMarkdown本文をMarkdown Code Fenceへ入れず、明確なData Boundaryを
付けて渡す。Document内のFenceがPrompt境界を壊さないよう、LengthまたはDelimiterを
Job Envelopeで明示する。

## 6. Workflow

### 6.1 Requirements Round

```text
1. Designer Jobをread-onlyで実行
2. Structured JSONを独立Schema検証
3. JSON Job ResultをvNNN.jsonへ保存
4. RequirementsMaterializerでvNNN.mdを生成
5. Markdown Hashを計算
6. Versioned Snapshotを確定
7. PublicationManagerがrequirements.mdを安全にPublish
8. SnapshotとPublication CopyのHash一致をGateで確認
9. ReviewerへVersioned Markdown Path/Hash/本文を渡す
10. Review SchemaとSubjectを検証
11. approved / changes_requested / blockedを処理
```

Publication Conflictが起きた場合は7で停止し、Reviewerを起動しない。

### 6.2 Revision

Designerへ渡す前回ArtifactはMarkdownとする。前回の構造化JSONを正本として再利用
せず、承認対象だったDocumentとRequired Changesを入力にする。Designerの新出力は
毎回Schema検証し、新しいVersionへMaterializeする。

### 6.3 Manual Edit Adoption

Publication CopyのHashが変化した場合、自動採用しない。

```text
detect conflict
  -> WAITING_FOR_HUMAN
  -> keep: Output Rootを変更してresume
  -> adopt: 手編集Markdownを新Candidate Snapshotへcopyして再レビュー
  -> replace: 明示確認後にEngine生成Versionをpublish
```

`adopt`はMarkdownからRequirements JSONを推測生成しない。後続AgentへMarkdownを入力し、
次の構造化RoundでMachine Contractを再構成する。

## 7. Output Root解決

### 7.1 優先順位

```text
start --output-dir
  > project config artifacts.output_dir
  > dirname(request source file)
  > project root
  > cwd
```

### 7.2 解決手順

1. 起動時CWDを絶対Pathとして取得
2. 選択されたCandidateを絶対Path化
3. 既存Rootなら明示Root Symlinkを物理Pathへ解決
4. 未作成Rootなら最も近い既存Parentを物理Pathへ解決して配下を作成
5. Root DirectoryをOwner-only Permissionで開く
6. 解決済みPathをRunへ保存
7. resumeでは保存値だけを使用

### 7.3 安全なPublication

macOS/Linux用WriterはDirectory File Descriptorを起点とし、可能な限り
`openat`/`mkdirat`と`O_NOFOLLOW`を利用する。Platform差は`internal/platformfs`
InterfaceのBuild Tag実装へ閉じ込める。

```text
1. Rootを物理Pathへ解決してDirectory FDを取得
2. Relative Pathをcleanし、absolute/..を拒否
3. 各既存Componentをno-followで検査
4. TargetがSymlinkなら拒否
5. Existing File HashとManifest Hashを比較
6. 同一DirectoryへTemporary Fileを作成
7. write -> fsync -> close
8. Parent/Targetを再検査
9. renameatで置換
10. Directory fsync
11. Manifestを原子的更新
```

Platform APIが要件を満たせない場合、機能を弱めて続行せず、そのPlatformでPublication
をUnavailableとして診断する。

## 8. Document Compiler

### 8.1 Source Discovery

単一File入力:

- 指定FileだけをCompileする
- Default DestinationはSiblingの`preview/`

Directory入力:

- `*.md`と既知SubdirectoryのMarkdownを対象にする
- `preview/`、`.rct/`、Hidden Directory、Symlinkを除外する
- Lexical Path順で決定的に並べる
- Default Destinationは`<source>/preview/`

### 8.2 URL Policy

許可:

```text
relative paths
#fragment
https://
http://
mailto:
```

ただしHTML生成時に外部Resourceを自動Fetchしない。画像はSource Root内の相対Fileだけ
をAssetへcopyする。次を拒否する。

```text
javascript:
data:（Default）
file:
vbscript:
absolute local paths
protocol-relative URLs
```

### 8.3 CSS

Default ThemeはBinaryへEmbedする。

- System Font Stackだけを使用
- Light/Darkを`prefers-color-scheme`で切替
- Statusを色だけで表現しない
- Table/Codeだけ局所Scrollを許可
- Print MediaでNavigationを隠す
- 外部`@import`とRemote `url()`を含めない

Custom CSSは明示Fileだけをcopyする。`@import`、Remote URL、絶対File URLを検出した
場合は拒否し、Default Themeへ黙ってFallbackしない。

### 8.4 Derived Manifest

```json
{
  "schema_version": "1.0",
  "compiler_version": "document-v1",
  "theme_version": "default-v1",
  "sources": [
    {
      "path": "requirements.md",
      "sha256": "<sha256>",
      "output": "requirements.html",
      "output_sha256": "<sha256>"
    }
  ]
}
```

生成時刻をHTML本文または決定的Hash対象へ含めない。

## 9. CLI

### 9.1 start

```text
rct start --request-file request.md --output-dir ./docs/generated
```

表示項目へOutput RootとArtifact Protocol Versionを追加する。

### 9.2 doctor

`doctor`へ`--request-file`と`--output-dir`を追加し、実際に書き込まず次を検査する。

- Output Root解決結果
- Root作成可否
- Existing Publication Conflict
- Symlink Component
- Preview Destination Conflict

### 9.3 render

```text
rct render --source ./docs --destination ./docs/preview
```

Defaultは人間向けSummary、`--json`でManifestとWarningを出力する。

### 9.4 preview

```text
rct preview --source ./docs
```

初回Render後にURLを表示し、SIGINT/SIGTERMでGraceful Shutdownする。

### 9.5 status

次を追加表示する。

- Output Root
- Artifact Protocol Version
- Current Versioned Markdown
- Publication CopyとHash状態
- Preview stale status
- Publication Conflict

## 10. ErrorとState

追加Error:

```text
OutputRootResolutionError
PublicationConflictError
PublicationSymlinkError
PublicationRaceError
DocumentMaterializationError
MarkdownCompileError
UnsafeMarkdownURL
UnsafeCustomCSS
PreviewBindError
PreviewRequestRejected
```

`PublicationConflictError`、`PublicationSymlinkError`は利用者判断で解消可能なため
`WAITING_FOR_HUMAN`へ遷移する。Materialization、Compiler内部Error、Manifest破損は
`FAILED`とする。Preview単独Commandの失敗はWorkflow Runを失敗させない。

## 11. Security

- Agent出力とMarkdownを非信頼入力として扱う
- GoldmarkのUnsafe Renderingを有効にしない
- `html/template`のEscape Boundaryを維持する
- Absolute Local PathをHTMLへ出さない
- Image Asset CopyでSource Root外を拒否する
- Custom CSSのExternal Importを拒否する
- PreviewはLoopback、Read-only、Same-Originだけにする
- Output Root WriterはSymlinkを辿らない
- Raw stdout/stderrをPublication Rootへ出さない
- ErrorへDocument本文や秘密らしき値を含めない

## 12. Test設計

### 12.1 Unit

- Output Root優先順位の全組合せ
- Requirements JSONからのGolden Markdown
- LF、末尾改行、安定したSection順
- Versioned Snapshotの非上書き
- Publication Hash一致時の改版
- 同一Runでの手編集検出
- Unowned File Conflict
- Symlink Target/Parent拒否
- Path Escape拒否
- GFM SubsetのGolden HTML
- Raw HTMLと危険URLの無害化
- Custom CSSのImport/Remote URL拒否
- Manifest stale判定

### 12.2 Contract

- Review Schemaが`text/markdown` Subjectを要求する
- Reviewer PromptがMarkdown Path/Hashを含む
- JSON SubjectをMarkdown Protocol Runへ混入できない
- CompilerがNetwork Accessを要求しない
- PreviewがGET/HEAD以外を拒否する
- Host/Origin不一致を拒否する

### 12.3 Integration

- Request File ParentをDefault Output Rootにする
- Explicit Output RootがDefaultを上書きする
- Designer JSON→Markdown→Publication→Reviewの一巡
- changes_requested後のv002とPublication更新
- Review待ち手編集で`WAITING_FOR_HUMAN`
- render DirectoryからMulti-page Siteを生成
- Previewを`127.0.0.1:0`で起動・停止

### 12.4 Platform

- macOS arm64でSymlink/Atomic Replace Test
- Linux amd64でSymlink/Atomic Replace Test
- Race Detector
- Cross Build

## 13. Migration

現行実装のRun StateにはProtocol Versionがない。移行時は次とする。

1. Protocol VersionなしRunを`json-v1`として読む
2. `json-v1` Runは従来のJSON SubjectでのみStatus参照可能にする
3. 自動resumeで`document-v1`へ変換しない
4. 明示Migrationは最新JSON ArtifactからMarkdown Candidateを生成する
5. Migration後は既存Reviewを無効化し、新しいMarkdown Hashで再レビューする

初回公開前で既存Run互換を不要と判断する場合も、Protocol Fieldと混在拒否Testは残す。

## 14. Dependency方針

追加候補:

| Dependency | 用途 | Runtime追加Install |
|---|---|---|
| `github.com/yuin/goldmark` | CommonMark/GFM ParseとHTML Render | 不要 |
| `golang.org/x/sys/unix` | macOS/Linuxのno-follow File操作 | 不要 |

依存Versionは実装Milestone開始時に固定し、LicenseとTransitive Dependencyを確認する。
Node.js、Python、Pandoc、Browser Extensionは導入しない。

## 15. 要件追跡

| 設計領域 | 対応要件 |
|---|---|
| Materializer / Versioned Snapshot | FR-160〜163, AC-019, AC-025 |
| Compiler / Site Builder | FR-163〜167, FR-174〜178, AC-020〜024 |
| Output Resolver | FR-168〜171, AC-016〜018 |
| Publication Manager | FR-170〜173, AC-022, AC-026〜027 |
| Preview Server | FR-165〜167, AC-028 |
| Protocol Migration | FR-161〜162, ADR-009 |

## 16. 実装開始条件

- 本詳細設計が独立Architecture Reviewで承認されている
- Implementation Milestoneが独立Plan Reviewで承認されている
- Publicationのno-follow実装方式がmacOS/LinuxでTechnical Spike済み
- Go ToolchainがRace TestとCross Buildを実行できる状態である
- Claude/Codex E2Eに必要な両CLI認証が利用可能である
