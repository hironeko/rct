# Local Browser Control Plane 詳細設計

- 文書版: 0.2.0-draft
- 作成日: 2026-08-02
- 対応要件: `docs/requirements.md` 0.6.1-draft FR-190〜FR-214
- 対応ADR: `docs/architecture.md` ADR-010
- 状態: Claude Architecture Review待ち

## 1. 目的

CLIを知らない利用者でも、ローカルブラウザから次を実行できる操作面を提供する。

- 許可された場所から既存Projectを選ぶ
- 新しいProject Directoryを安全に作る
- 新規要望またはApplication Briefを入力する
- Markdownとしてローカルへ保存する
- 保存済みRequestからLoop Engine Runを開始する
- CLIと同じRun Stateと主要Artifactを確認する

Browserは新しいWorkflow Engineではない。CLIとBrowserは同じApplication Serviceへの
Inbound Adapterであり、正式状態は既存のState Store、Artifact Store、Event Logに置く。

## 2. 非ゴール

- InternetまたはLANへの公開
- 複数利用者、Account、Login、Role Based Access Control
- Cloud StorageまたはRemote Database
- Browser内でのSource Code編集
- 任意File Viewerまたは任意Shell Console
- Source Scaffold、Dependency install、`git init`の暗黙実行
- Browserを閉じたときのRun自動Cancel
- CLIと異なるWorkflowまたはApproval規則
- React、Node.js、npmを配布BinaryのRuntime依存にすること
- Full-stack Web Framework、SSR、React Server Components

## 3. UXフロー

### 3.1 Home

最初のViewportには次だけを主要Actionとして表示する。

```text
┌─────────────────────────────────────────────────────┐
│ Loop Engine                                         │
│ Turn a rough request into a reviewed implementation │
│                                                     │
│ [ New request ]          [ New application ]        │
│                                                     │
│ Recent runs                                         │
│ run-...  REQUIREMENTS_REVIEW  project-name          │
└─────────────────────────────────────────────────────┘
```

Global Navigationを増やさず、Recent runsは補助領域にする。初回目的である要望投入を
最も強い視覚優先度にする。

### 3.2 New request

1. Workspace Rootを選択
2. Root内の既存Project Directoryを選択
3. Title、Rough request、Goals、Constraintsを入力
4. Designer、Mode、Backend、Review上限を選択
5. 保存先を確認
6. `Save draft`または`Save and start`

保存先Default:

```text
<project>/requests/<UTC compact timestamp>-<slug>.md
```

### 3.3 New application

1. Workspace Rootを選択
2. 親Directoryを選択
3. Project nameとslugを入力
4. Application briefを入力
5. ProviderとRun Optionを選択
6. 作成予定Pathを確認
7. `Save draft`または`Save and start`

保存先Default:

```text
<parent>/<project-slug>/request.md
```

既存Pathが存在する場合、内容に関係なく自動Mergeしない。空Directoryを採用する機能も
初期版では実装せず、別slugを要求する。

### 3.4 Confirmation

State変更前に、次を一画面で確認できること。

- Workspace Root label
- 相対Project Path
- 作成または更新する相対File Path
- Request SHA-256
- Designer / Implementer / Reviewer割当
- Mode / Backend
- `Save draft`か`Save and start`か

絶対Pathは必要な確認画面だけに表示し、API Errorや通常Logへ無条件に含めない。

## 4. Component構成

```text
Browser
  -> HTTP Adapter
       -> Security Middleware
       -> Workspace Query Service
       -> Intake Application Service
       -> Run Application Service
       -> Run Query Service
            -> Domain / Workflow
            -> Provider Adapters
            -> Runtime Backends
            -> State / Artifact / Intake Stores
```

### 4.1 `internal/controlplane/http`

責務:

- RouteとMethod
- Request decodeとResponse encode
- Session/CSRF/Origin/Host/CSP Policy
- Body size、timeout、request ID
- Domain Errorから公開ErrorへのMapping
- Static AssetとHTML Template配信

禁止:

- Provider CLIの直接起動
- `exec.Command("loop-engine", ...)`
- Workflow Stateの直接書換
- Absolute Path文字列の無検証利用

### 4.2 `internal/workspace`

責務:

- 許可Rootの登録とStable Root ID生成
- Root配下のDirectory Entry列挙
- Root ID + Relative Pathの安全な解決
- Directory作成前の競合検査
- Symbolic Link / traversal / Root escape拒否

公開型:

```go
type Root struct {
    ID    string
    Label string
}

type RelativeDirectory struct {
    RootID       string
    RelativePath string
}

type Browser interface {
    Roots(context.Context) ([]Root, error)
    Directories(context.Context, RelativeDirectory) ([]Entry, error)
    ResolveProject(context.Context, RelativeDirectory) (ProjectRef, error)
}
```

Handlerへ絶対Pathを返す必要がないQueryでは`ProjectRef`内へ隠蔽する。

### 4.3 `internal/intake`

責務:

- Form入力の型検証
- Request Markdownの決定的生成
- Intake ID採番
- Atomic write
- Request SHA-256計算
- Idempotency Key管理
- DraftからRun開始済みへのRevision付き遷移

```go
type Kind string

const (
    KindRequest     Kind = "request"
    KindApplication Kind = "application"
)

type CreateInput struct {
    Kind            Kind
    WorkspaceRootID string
    Parent          string
    ProjectSlug     string
    Title           string
    Body            string
    Goals           []string
    Constraints     []string
    RunOptions      RunOptions
    Action          Action
    IdempotencyKey  string
}

type Intake struct {
    SchemaVersion   string
    ID              string
    State           State
    Kind            Kind
    RequestRef      FileRef
    RequestSHA256   string
    RunID           string
    Revision        uint64
}
```

State:

```text
DRAFT -> STARTING -> STARTED
  |         |
  |         -> START_FAILED
  -> CONFLICT
```

同一Idempotency Keyで`STARTED`済みなら、既存IntakeとRun IDを返す。

### 4.4 Shared Application Service

現在CLI内にあるRequest File読取とOption組立のうち、UIと共有すべき処理をApplication層へ
移す。

```go
type StartRunInput struct {
    Project           ProjectRef
    RequestPath       string
    ExpectedSHA256    string
    Mode              domain.RunMode
    Backend           runtime.Preference
    ProviderSelection domain.ProviderSelection
    MaxReviewRounds   int
    OutputDirectory   *workspace.RelativeDirectory
}

type RunStarter interface {
    StartSavedRequest(context.Context, StartRunInput) (domain.Run, error)
}
```

Application Serviceは開始直前にRequest bytesを再読込し、`ExpectedSHA256`と一致しなければ
Runを作らない。CLIの`start --request-file`も同じMethodへ移行する。

### 4.5 Run Manager

HTTP RequestのContextはClient切断でCancelされるため、開始済みRunへそのまま伝播しない。
Server所有のRun ManagerがRun単位ContextとCancel Functionを管理する。

- `Save and start`はRun永続化まで同期する
- 長時間Agent LoopはServer所有Goroutineで実行する
- Browser切断でCancelしない
- Server shutdownでは新規受付を止め、実行中Jobへ猶予付きCancelを送る
- State Storeへ確定済みStateを残す

同一Project / RunへのWriter Lockは既存Store境界で維持する。

## 5. File layout

既存ProjectへのRequest:

```text
project/
├── requests/
│   └── 20260802T120000Z-add-export.md
└── .loop-engine/
    ├── intakes/
    │   └── intake_<id>/
    │       ├── intake.json
    │       └── events.jsonl
    └── runs/
```

新規Application:

```text
workspace/
└── application-slug/
    ├── request.md
    └── .loop-engine/
        ├── intakes/
        └── runs/
```

Intake Metadata内のPathは、Workspace Root IDと相対Pathを正本とする。診断用にAbsolute Pathを
保持する場合はBrowser APIへ公開しない内部Fieldへ分離する。

## 6. Markdown materialization

Formから生成するMarkdownは入力内容だけを決定的に整形する。

```markdown
# <Title>

## Request

<Body>

## Goals

- <Goal>

## Constraints

- <Constraint>
```

空のOptional Sectionは出力しない。同じ正規化済み入力からは同じMarkdown bytesを生成する。
HTML、Script、Template構文を解釈せずTextとして保存する。

## 7. HTTP API v1

### 7.1 Read-only

```text
GET /api/v1/config
GET /api/v1/workspaces
GET /api/v1/workspaces/{root-id}/directories?path=<relative>
GET /api/v1/intakes/{intake-id}
GET /api/v1/runs
GET /api/v1/runs/{run-id}
GET /api/v1/runs/{run-id}/events
```

### 7.2 State-changing

```text
POST /api/v1/intakes
POST /api/v1/intakes/{intake-id}/start
```

初回VersionではDraft作成と開始を分離する。UIの`Save and start`は二つのAPIを順に実行するが、
Start側はIdempotency Keyにより再送可能にする。

### 7.3 Response envelope

```json
{
  "data": {},
  "error": null,
  "request_id": "req_<id>"
}
```

公開Error:

```text
invalid_input
unauthorized
forbidden_origin
workspace_escape
path_conflict
stale_intake
duplicate_request
provider_unavailable
backend_unavailable
internal_error
```

`internal_error`へ内部Error文字列を含めない。詳細はServer LogへRequest IDとともに保存する。

## 8. Security policy

### 8.1 Network

- Bind: `127.0.0.1:0`
- Allowed Host: 起動時に確定したHost/Portだけ
- CORS: 無効
- Methods: GET/HEAD/POSTだけ
- Read timeout、header timeout、idle timeoutを設定
- HeaderとBody sizeを制限

### 8.2 Session and CSRF

- 起動ごとに256-bit以上のRandom Session Tokenを生成
- TokenはURL Fragmentまたは初回BootstrapでSameSite=Strict Cookieへ移す
- TokenをQuery Log、Referer、Artifactへ保存しない
- 状態変更RequestはSession Cookieと別のCSRF Tokenを要求
- Originは起動Originとの完全一致を要求

### 8.3 Content Security Policy

```text
default-src 'none';
script-src 'self';
style-src 'self';
img-src 'self' data:;
connect-src 'self';
font-src 'none';
base-uri 'none';
form-action 'self';
frame-ancestors 'none'
```

Inline ScriptとInline Styleを使わない。Templateへ利用者入力を入れる場合は`html/template`の
Autoescapeを維持し、`template.HTML`へ変換しない。

### 8.4 File System

- Rootは起動時に一度Canonicalize
- Root IDはAbsolute Pathを露出しないStable random ID
- Relative PathはClean後も`..`、Absolute、NULを拒否
- 各Path Componentをno-followで検査
- Final write前に親Directory identityを再検査
- Temp FileとFinal renameは同じDirectoryで行う
- File modeはRequest `0644`、内部Metadata `0600`をDefaultとする

## 9. Frontend構成

### 9.1 Technology boundary

Frontendは次の小さい構成に固定する。

- Language: TypeScript Strict Mode + TSX
- UI: React + React DOM
- Routing: React Router Data Mode (`createBrowserRouter`)
- Build tool: Vite
- Styling: Plain CSS + CSS Custom Properties
- Server/API: 既存Go HTTP Adapter (`/api/v1`)

ViteはFrameworkではなく開発Serverと静的Asset Builderとしてだけ使う。React Routerの
Framework Mode、`@react-router/dev`、SSR/SSG、Route Module Convention、Next.js、Remixは使わない。
Data Modeの`loader`、`action`、pending stateを使用してよいが、正式なState TransitionとValidationは
常にGo Application Serviceが所有する。

### 9.2 Source and embedded asset layout

Go `embed.FS`へProduction Assetを埋め込む。

```text
web/
├── embed.go
├── package.json
├── package-lock.json
├── tsconfig.json
├── vite.config.ts
├── index.html
├── src/
│   ├── main.tsx
│   ├── router.tsx
│   ├── api/
│   │   ├── client.ts
│   │   └── types.ts
│   ├── routes/
│   ├── components/
│   └── styles/
└── dist/
    ├── index.html
    ├── manifest.json
    └── assets/
```

`vite.config.ts`では`build.manifest: "manifest.json"`を指定し、Dot-prefixed Directoryの
`go:embed`除外規則へ依存しない。`embed.go`は`//go:embed dist`でDirectory全体を保持する。

`dist/`はGenerated Artifactだが、`go install`とGo-only Source BuildでUIを欠落させないためRepositoryへ
Commitする。CIは`npm ci && npm run build`後に`git diff --exit-code -- web/dist`を実行し、Source、Lockfile、
Manifest、埋込Assetの乖離を拒否する。Release Buildも同じ手順で再生成してからGo Binaryを作る。

Node.js、npm、ViteはFrontend開発と再生成にだけ必要である。Release Binaryの利用者には要求しない。
Production DependencyはReact、React DOM、React Routerを基本上限とする。

### 9.3 Routes

Router basenameは`/ui`とし、次を定義する。

| Route | Responsibility |
|---|---|
| `/ui/` | Home、primary actions、recent runs |
| `/ui/requests/new` | Existing project request form |
| `/ui/applications/new` | New application form |
| `/ui/intakes/:intakeId` | Saved intake confirmation and start action |
| `/ui/runs/:runId` | Run state, review round, artifacts, failure reason |

Go Serverは`GET /ui/*`のうちStatic AssetとAPIに一致しないPathへ`dist/index.html`を返す。`/api/v1/*`、
`/ui/assets/*`、未知の非UI PathはFallback対象外とする。Vite `base`は`/ui/`、Asset名はContent Hash付きとする。

### 9.4 Client/server contract

`web/src/api/types.ts`に公開DTOを集約し、`client.ts`だけが`fetch`を直接使用する。Mutationは
Session Cookie、CSRF Header、Idempotency Key、JSON Content-Typeを含める。React Router `action`は
API Clientを呼び出すだけで、Path解決、Provider Default導出、Run生成を実装しない。

Route `loader`は再読込可能なServer Stateを取得する。Form DraftとNavigation StateだけをClientへ保持し、
Intake/Runの正式状態はAPI Responseから再構成する。Local StorageへSession Token、絶対Path、Prompt、
Run Stateを保存しない。

### 9.5 Rendering and failure behavior

初期HTMLは静的なReact mount pointと外部Module Script/CSSだけを含み、Inline Script/Styleを使わない。
各RouteにError Boundary、Loading表示、空状態を設け、APIの`request_id`を利用者向けErrorへ表示する。
JavaScriptが無効またはAssetが欠落した場合、Go Serverは安全な静的Error Pageを返し、Mutationを実行しない。

Client StateはForm Draftと表示中Runだけに限定する。正式なIntake/Run StateはAPIから取得する。

## 10. CLI

```text
loop-engine serve \
  --workspace-root /Users/example/Work \
  --listen 127.0.0.1:0 \
  --open
```

Option:

```text
--workspace-root <path>  repeatable; default current directory
--listen <address>       v1はloopbackのみ
--open                   default true when interactive
--no-open                do not launch a browser
--json                   print bootstrap information as JSON
```

標準出力へURLを表示する場合、Session Tokenを含まないSafe URLとする。Browser自動OpenへTokenを
渡す方式はProcess ListとShell Historyへ残らない実装を選ぶ。

## 11. Failure and recovery

| Failure | Result |
|---|---|
| Workspace Root不正 | Serverを起動しない |
| Request Validation失敗 | Form Error、File未作成 |
| Path Conflict | Intake `CONFLICT`または作成前停止 |
| Markdown保存失敗 | Intake未確定、Run未作成 |
| Request Hash変更 | Start拒否、Draft再確認 |
| Provider Preflight失敗 | Intake保持、Run開始失敗を表示 |
| Browser切断 | Run継続 |
| Server終了 | Graceful stop後、Stateから再表示可能 |

## 12. Test strategy

### Unit

- slug normalization and rejection
- Markdown materialization determinism
- Root containment
- traversal and symlink rejection
- Intake transitions and revision conflicts
- Idempotency Key replay
- public error mapping

### Contract

- HTTP method/content type/body limit
- Host/Origin/CSRF/Session enforcement
- CSP and security headers
- Fake Application Service invocation
- CLI and Web StartRunInput equivalence
- no external asset references
- `/ui/*` deep-link fallback and `/api/v1` exclusion
- TypeScript API DTO and Go JSON fixture compatibility

### Integration

- New request Save draft
- New request Save and start
- New application creation
- duplicate submission
- restart and Run rediscovery
- Browser disconnect while Fake Run continues
- React Router actionからCSRF/Idempotencyを含むFake API mutation
- Production Assetを埋め込んだGo Binaryだけでのoffline route load

### Frontend

- TypeScript strict type check
- Route loader/action and Error Boundary tests
- New request / New application form tests
- Keyboard, focus order, error summary tests
- Production dependency policy test
- `web/dist` reproducibility test

実Providerは通常Testで起動しない。

## 13. Traceability

| Design area | Requirements |
|---|---|
| CLI serve / loopback | FR-190, FR-191, AC-034 |
| Primary UX | FR-192, FR-194〜196, AC-035〜037 |
| Workspace boundary | FR-193, FR-205, AC-034, AC-037 |
| Intake persistence | FR-197, FR-199, AC-035, AC-038 |
| Shared Core | FR-198, FR-208, AC-036, AC-042 |
| Run display/recovery | FR-200, FR-201, AC-041 |
| HTTP security | FR-202〜204, FR-206, AC-039, AC-040 |
| Accessibility/runtime | FR-203, FR-207, AC-040, AC-043 |
| React/TypeScript boundary | FR-209〜214, AC-044〜048 |

## 14. Implementation entry conditions

- 本設計が独立Architecture Reviewで承認されている
- `docs/implementation-plan-local-control-plane.md`がPlan Reviewで承認されている
- Workspace no-follow方式がmacOS/Linuxで検証可能な形に分離されている
- Shared Application Serviceの境界がPlan/Implementation Loopの設計と競合しない
- React Router Data ModeとGenerated Asset commit方針が独立Reviewで承認されている
- 一度に一つのMilestoneだけを実装する
