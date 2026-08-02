# rct 要件定義書

- 文書版: 0.11.1-draft
- ステータス: Draft（rct Core Loop実装済み、拡張機能は設計段階）
- 対象: MVP から v1
- 対象OS: macOS / Linux
- 実装言語: Go

## 1. 概要

rctは、利用者が入力した曖昧または概略的な要望を起点として、複数の生成AIエージェントを役割分担させながら、要件定義、設計、実装計画、実装、検証、レビュー、修正を段階的に進行させるローカル実行型のオーケストレーターである。

標準ロールは次のとおりとする。ロールは固定の生成AIへ紐付けず、Provider（CodexまたはClaude Code）を設定により割り当てる。

- Designer: PdM、要件定義者、設計者、実装計画者を担う
- Implementer: マイルストーン実装者、Review対応者（Fixer）を担う
- Reviewer: 要件、設計、計画、コードおよび検証結果を独立レビューする
- rct: 状態管理、ジョブ管理、成果物管理、停止条件、権限境界、再開処理を担う制御者
- 利用者: 最初の要望の提示、必要な判断、任意の承認、最終受け入れを行う、Designer/Implementer/ReviewerへのProvider割当を選択する

DesignerとImplementerには同一Providerを割り当てても別Providerを割り当てても構わない。ただしReviewerには、同一RunのDesignerおよびImplementerと異なるProviderを割り当てなければならない（詳細は9.16）。デフォルト割当はDesigner=Codex、Implementer=Codex、Reviewer=Claude Codeとする。

Designer、Implementer、Reviewerは、同じProviderが複数の非Review役割を担う場合でも、必ず異なるRole IDとAgent sessionを使用する。MVPで利用可能なProviderがCodexとClaude Codeの二つだけの場合、Reviewerを両方の成果物から独立させるため、DesignerとImplementerは同じProviderへ割り当て、Reviewerはもう一方へ割り当てる。利用者が要望を最初に受け取るDesigner Providerを選ぶことで、この組み合わせを反転できる。

Herdrは最優先のセッション表示・制御バックエンドとする。ただし必須依存にはせず、tmux、Direct実行へフォールバックできる設計とする。

## 2. 背景と課題

単一の生成AIに要件定義から実装と自己レビューまでを担当させると、次の問題が起きやすい。

- 初期の誤解が後工程まで引き継がれる
- 自身が作成した設計やコードへの評価が甘くなる
- 会話履歴が肥大化し、重要な制約が失われる
- レビューと修正の終了条件が曖昧になる
- セッション中断後に、何が承認済みか判断できなくなる
- 画面上の発言と実際の成果物の状態が一致しない

rctは、役割分離、構造化成果物、明示的なゲート、有限回の修正ループ、永続化された状態を用いてこれらを解決する。

## 3. プロダクトゴール

### G-001: 概略要望から実装可能な仕様を作る

利用者が自然言語で概略要望を入力すると、Designer役割のProviderが要件、制約、未決事項、受け入れ条件、設計方針を成果物として作成できること。

### G-002: 異なる生成AIによる独立レビューを行う

Reviewer役割のProviderが、Designer/Implementer役割のProviderの成果物を所定の評価基準でレビューし、承認、修正要求、判断不能を構造化形式で返せること。Reviewer役割には、同一RunのDesigner/Implementer役割と異なるProviderを割り当てる。

### G-003: 実装まで同一の制御モデルで進める

要件・設計の承認後、実装計画、マイルストーン実装、検証、コードレビュー、修正を同一ワークフローで実行できること。

### G-004: 実行環境へのロックインを避ける

Herdrが存在する場合はHerdrを利用し、存在しない場合はtmux、さらに存在しない場合はDirect実行で動作できること。

### G-005: プロジェクト種別へ依存しない

Node.js、Python、Go、Rust、Swiftなど特定の言語やフレームワークを前提にせず、プロジェクト調査から実行プロファイルを生成できること。

### G-006: 中断・再開可能にする

rctや端末が終了しても、永続化された成果物と状態から安全に再開できること。

### G-007: 成果物を人間が読みやすい形で提供する

要件定義書、設計書、実装計画などを、開発者とGitHubでの閲覧に適したMarkdownとして
保存すること。ローカル閲覧時は、同じMarkdownから視覚的認知コストを下げた静的
HTML/CSSを生成できること。

### G-008: ブラウザから安全に要望を投入する

利用者がローカルブラウザから既存プロジェクトへの新規要望または新規アプリケーション
作成依頼を入力し、許可したWorkspace Root内へMarkdownとして保存したうえで、CLIと
同一のrct CoreへRunを依頼できること。

## 4. 非ゴール

MVPでは次を対象外とする。

- クラウド上での分散実行
- 複数人による同時操作
- Remote公開、複数利用者、認証基盤を伴うHosted Web UI。単一利用者向けのLoopback
  Local Browser Control Planeはv1拡張として対象に含める
- Next.js、Remix、React Router Framework Modeなど、FrontendとServer Runtimeを一体化する
  Full-stack Web Framework、SSR、React Server Components
- Kubernetesやコンテナオーケストレーターとの統合
- 生成AIサービスのAPIキー管理
- 独自LLM推論基盤の提供
- GitHub Pull Requestの自動作成や自動マージ
- 本番環境への自動デプロイ
- 自動課金最適化
- Windowsネイティブ対応
- CodexおよびClaude Code以外の新規AIプロバイダーの完成実装（Designer/Implementer/Reviewer間でCodexとClaude Codeの割当を入れ替えること自体はMVP対象内とする）

ただし、将来追加できるようにProviderおよびRuntime Backendを抽象化する。

## 5. MVPスコープ

MVPに含める機能を次に限定する。

- Go製の単一CLIバイナリ
- 要望を最初に受け取るDesigner ProviderをCodexまたはClaude Codeから選択
- DesignerとImplementerを独立Role・独立Sessionとして実行
- ReviewerをDesigner/Implementerと異なるProvider・独立Sessionとして実行
- デフォルトはDesigner/Implementer=Codex、Reviewer=Claude Code
- 要件・設計の生成、レビュー、修正ループ
- 実装計画の生成、レビュー、修正ループ
- マイルストーン単位の実装、検証、レビュー、修正ループ
- Herdr、tmux、Direct Backendの自動選択
- Supervised、Autonomous、Design-onlyモード
- ファイルベースの成果物、状態、イベントログ
- GitHubで閲覧可能なMarkdown形式の利用者向け成果物
- Markdownから静的HTML/CSSを生成するローカルDocument Compiler
- 利用者向け成果物の出力先指定と、Request File基準のデフォルト配置
- 最大試行回数、タイムアウト、停止、再開
- 実装開始前の人間承認ゲート
- `start`、`status`、`resume`、`approve`、`stop`、`doctor`、`logs`
- macOS arm64とLinux amd64のリリース

macOS amd64、Linux arm64、追加Provider、通知、Pull Request連携は、Core設計上は考慮するがMVPのリリース条件には含めない。

v1拡張では、Go単一バイナリに埋め込まれたLocal Browser Control Planeを追加する。
この拡張はCLIを置き換えず、同じApplication Service、State Store、Artifact Store、
Gate Evaluatorを別の入力面から利用する。

## 6. 前提条件

- macOSまたはLinuxで動作する
- Codex CLIとClaude Codeがインストール済みである
- Codex CLIとClaude Codeの認証は各CLI側で完了している
- rctはAPIキーやログイン資格情報を保存しない
- 実装工程を行うプロジェクトは、実装前Preflight完了時点で有効なHEADを持つGitリポジトリである
- 要件定義・設計のみの実行ではGitを必須としない
- Herdrとtmuxは任意依存である
- Goランタイムは利用者環境に不要とし、ビルド済み単一バイナリを配布する

## 7. 利用者とユースケース

以下のユースケースでは、デフォルト割当（Designer/Implementer=Codex、Reviewer=Claude Code）を前提に「Codex」「Claude Code」と表記する。役割とProviderの対応関係が入れ替わっている場合は、それぞれ「Designer/Implementer役割のProvider」「Reviewer役割のProvider」と読み替える。

### UC-001: 新しい要望から要件と設計を作る

1. 利用者がプロジェクトディレクトリでrctを起動する
2. 利用者が概略要望を入力する
3. Codexがプロジェクトと要望を調査する
4. Codexが要件・設計成果物を作成する
5. Claude Codeが成果物をレビューする
6. 修正要求があればCodexが修正する
7. 承認または最大試行回数到達で終了する

### UC-002: 承認済みの要件から実装計画を作る

1. Codexが実装マイルストーンと受け入れ条件を作成する
2. Claude Codeが実現可能性、依存関係、検証可能性、リスクを評価する
3. Codexが修正要求を反映する
4. 承認後、実装開始ゲートへ進む

### UC-003: マイルストーンを実装する

1. Codexが対象マイルストーンだけを実装する
2. rctが定義済み検証コマンドを実行する
3. Claude Codeが差分、要件適合性、検証結果をレビューする
4. Codexが必須指摘へ対応する
5. 再検証と再レビューを行う
6. 承認後に次のマイルストーンへ進む

### UC-004: 中断した処理を再開する

1. 利用者が `resume` を実行する
2. rctが状態と成果物の整合性を検査する
3. 利用可能なら既存のエージェントセッションを再利用する
4. 再利用できなければ新しいセッションを作成する
5. 最後に確定したチェックポイントから処理を再開する

### UC-005: 人間が判断を与える

次のいずれかが発生した場合、利用者は承認、差し戻し、回答、停止を選択できる。

- 要件や設計に重大な未決事項がある
- 外部仕様が不足している
- 破壊的操作または高リスク操作が必要である
- 最大レビュー回数へ到達した
- 実装開始前の承認ゲートへ到達した
- エージェントが判断不能を返した

### UC-006: 生成資料をローカルで視覚的に確認する

1. 利用者がMarkdownの要望ファイルからRunを開始する
2. rctが要件定義書、設計書、実装計画などをMarkdownとして出力する
3. 利用者が`render`または`preview`を実行する
4. rctがMarkdownを静的HTML/CSSへ変換する
5. 利用者が見出しナビゲーション、表、コード、レビュー状態をブラウザで確認する
6. HTMLを削除しても、Markdownから同じ内容を再生成できる

### UC-007: ブラウザから既存プロジェクトへ新規要望を追加する

1. 利用者が許可するWorkspace Rootを指定してLocal Control Planeを起動する
2. ブラウザで「New request」を選ぶ
3. 許可されたRoot内から既存プロジェクトを選ぶ
4. 要望、制約、Designer Provider、Modeを入力する
5. rctが要望をMarkdownへ原子的に保存する
6. 利用者が`Save and start`を選んだ場合、同じ入力から一つのRunを開始する
7. 利用者がブラウザまたはCLIの`status`から同じRun IDと状態を確認する

### UC-008: ブラウザから新規アプリケーション作成依頼を登録する

1. 利用者がブラウザで「New application」を選ぶ
2. 許可されたRoot内の親Directoryと新しいProject名を指定する
3. 作成目的、対象利用者、主要機能、制約、希望する技術または未指定を入力する
4. rctが新しいProject Directoryと`request.md`を作成する
5. 既存Pathとの競合がなければ、保存したRequestを入力としてRunを開始できる
6. DesignerとReviewerがCLI起動時と同じArtifact ProtocolでLoopを進める

## 8. 動作モード

### 8.1 Supervisedモード

デフォルトモードとする。

- 要件・設計・実装計画は自動でレビューと修正を進める
- 最初のコード変更前に人間の承認を要求する
- 高リスク操作は常に人間へエスカレーションする
- 最終完了時に人間の確認を要求する

### 8.2 Autonomousモード

明示的なオプトインでのみ有効にする。

- 承認済みの要件と計画に基づく実装を自動進行する
- 設定された安全境界を超える操作は実行しない
- 最大試行回数、タイムアウト、重大な判断不能では停止する
- デプロイ、マージ、秘密情報操作は自動化しない

### 8.3 Design-onlyモード

- 要件定義、設計、レビューまでを実行する
- 実装計画およびコード変更を行わない
- Gitリポジトリを必須としない

## 9. 機能要件

以下の機能要件では、デフォルト割当（Designer/Implementer=Codex、Reviewer=Claude Code）を前提に「Codex」「Claude Code」と表記する箇所がある。実際にどちらのProviderがDesigner/Implementer/Reviewerを担うかは9.16の役割割当設定に従う。

### 9.1 CLIと実行開始

#### FR-001

rctは、コマンド引数、標準入力、またはファイルから概略要望を受け取れること。

#### FR-002

次のコマンドを提供すること。

```text
rct start
rct status
rct resume
rct approve
rct reject
rct answer
rct stop
rct doctor
rct logs
```

#### FR-003

`start` は少なくとも次のオプションを受け付けること。

```text
--request <text>
--request-file <path>
--mode supervised|autonomous|design-only
--backend auto|herdr|tmux|direct
--designer codex|claude
--implementer codex|claude
--reviewer codex|claude
--project <path>
--output-dir <path>
--execute
--max-review-rounds <number>
```

Provider割当は次の順序で導出する。

1. `--designer` が未指定なら `codex` とする
2. `--implementer` が未指定なら、解決済みの `--designer` と同じProviderとする
3. `--reviewer` が未指定なら、解決済みの `--designer` と異なるProviderとする。MVPでは `codex` に対して `claude`、`claude` に対して `codex` を選ぶ
4. 解決後のReviewer ProviderがDesignerまたはImplementer Providerと同じ場合、Runを開始せずエラーとする（詳細は9.16）

したがって、全Provider指定を省略した場合は `--designer codex --implementer codex --reviewer claude` となり、`--designer claude` だけを指定した場合は `--designer claude --implementer claude --reviewer codex` となる。

#### FR-004

起動時に採用したRuntime Backend、プロジェクトルート、解決済みOutput Root、実行モード、Designer Provider、Implementer Provider、Reviewer Provider、Run IDを表示すること。

#### FR-005

`--execute` を指定した場合、rctはRun初期化後にWorkflowを開始すること。実装途中のBackendが選択された場合、対応済みBackendへ暗黙に変更せず、利用可能な起動方法を示して停止すること。

`--max-review-rounds` は1以上とし、未指定時は3とする。上限到達時に自動承認してはならない。

### 9.2 環境診断

#### FR-010

`doctor` は次を検査すること。

- rctの設定ファイル
- Codex CLIの存在
- Claude Codeの存在
- 各CLIの認証状態と基本的な起動可否
- Herdrの存在と接続可否
- tmuxの存在
- プロジェクトディレクトリの読み書き可否
- Gitリポジトリ状態
- 成果物ディレクトリの整合性
- 解決済みOutput Rootの書込可否と既存ファイル競合
- Designer/Implementer/Reviewerの役割割当の妥当性（FR-151）

#### FR-011

診断結果は、人間向け表示に加えてJSON形式で出力できること。

### 9.3 Runtime Backendの選択

#### FR-020

`backend=auto` の場合、次の優先順で選択すること。

1. 接続可能なHerdr
2. 利用可能なtmux
3. Direct

#### FR-021

Herdrまたはtmuxが存在しなくても、Direct Backendで設計・レビュー・実装処理を実行できること。

#### FR-022

明示指定されたBackendが利用できない場合、暗黙に別Backendへ切り替えず、理由と選択肢を表示して停止すること。

#### FR-023

自動選択時のフォールバックはイベントログへ記録すること。

### 9.4 プロジェクト調査

#### FR-030

rctは対象ディレクトリから次を調査し、Project Profileを生成すること。

- プロジェクト名
- 使用言語
- フレームワーク
- パッケージマネージャー
- ビルドコマンド
- テストコマンド
- Lintおよび型検査コマンド
- 主要ディレクトリ
- 編集禁止または注意対象パス
- 既存のエージェント指示ファイル
- Git状態

#### FR-031

推定結果には根拠となるファイルと確信度を記録すること。

#### FR-032

プロジェクト固有設定が存在する場合は、自動推定より優先すること。

#### FR-033

推定した任意コマンドを初回から無制限に実行しないこと。実行対象は設定、既知のマニフェスト、承認済みProject Profileのいずれかから得ること。

MVPのImplementation Planに記録できるVerification Executableはrct組込みAllowlistに限定し、
`curl`、汎用Interpreter、Shell、Privilege Escalation ToolなどAllowlist外のExecutableはProcess生成前に
拒否すること。子Processへ渡すEnvironmentは明示Allowlistから再構成し、親Processの環境変数全体を
継承してはならない。組込みAllowlist外のCommand Profile拡張は、Project Profile由来の根拠と利用者の
明示承認を保持できる仕様が実装されるまで提供しない。

### 9.5 要件定義と設計

#### FR-040

Codexは概略要望とProject Profileから、少なくとも次の成果物を作成すること。

- 要望の再定義
- 背景と解決対象
- ゴール
- 非ゴール
- 利用者とユースケース
- 機能要件
- 非機能要件
- 制約
- 前提
- 未決事項
- 受け入れ条件
- アーキテクチャ概要
- リスク

#### FR-041

不足情報があっても合理的な仮定で進行できる場合は、仮定を明示して成果物を作成すること。

#### FR-042

回答によって設計が大きく変わる未決事項は、人間へ質問としてエスカレーションすること。

#### FR-043

Requirements承認後、DesignerはRequirementsとは別のVersioned Architecture Artifactを生成し、
Decision、Component責務、Interface、Data Flow、品質特性、Risk、Requirement Traceabilityを記録すること。

#### FR-044

Architecture Artifactは`review_type: architecture`として独立Reviewerの有限Review/修正Loopを通過し、
現在ArtifactのPathとSHA-256に対するGateが成功するまでImplementation Plan生成へ進まないこと。

#### FR-045

`0.5.x`の実行可能な初期実装ではRequirements、Architecture、Implementation PlanをVersioned JSON
Artifactとして保存し、各JSON Schemaへ適合させること。ADR-009のMarkdown Publication移行では、
同じDomain情報をMarkdown正本へ移行し、旧Runを暗黙に読み替えないこと。

### 9.6 レビュー

#### FR-050

Claude Codeはレビュー対象成果物、適用する評価基準、入力成果物のハッシュ、Job IDを受け取ること。

#### FR-051

レビュー結果は所定のJSON Schemaへ適合し、少なくとも次を含むこと。

- Job ID
- Run ID
- レビュー対象の種類
- レビュー対象ハッシュ
- verdict
- 評価スコア
- 必須修正
- 任意提案
- 未解決事項
- レビュー要約

#### FR-052

`verdict` は次のいずれかとする。

```text
approved
changes_requested
blocked
```

#### FR-053

必須修正には重大度、対象、問題、根拠、期待する修正結果を含めること。

#### FR-054

レビュー対象ハッシュが現在の成果物と一致しない場合、そのレビューを古いものとして拒否すること。

#### FR-055

Claude Codeは原則として成果物とコードを変更せず、レビュー結果のみを出力すること。

Reviewer RoleをCodexへ割り当てた場合も同じ制約を適用すること。

#### FR-056

Reviewerの`verdict: approved`は、品質評価の結果として「現在のReview Subjectに
承認を妨げる必須修正と未解決事項がない」ことだけを表すものとする。次の条件を
すべて満たさない`approved`は意味的に不正なReviewとして拒否すること。

- `required_changes`が空である
- `open_questions`が空である
- Review Jobが正常終了し、Review Schemaに適合している

`optional_suggestions`は存在してもよい。評価スコアは診断情報とし、MVPでは単一の
固定閾値だけで承認可否を上書きしないこと。

#### FR-057

Reviewerの`approved`だけで次工程へ遷移してはならない。rctのGate Evaluatorは、
工程に応じて少なくとも次の決定的条件を追加確認すること。

- ReviewのRun ID、Job ID、Review Typeが現在のJobと一致する
- Review SubjectのPath、Media Type、SHA-256が現在のCandidate Artifactと一致する
- Candidate Artifactが存在し、対象SchemaとArtifact Protocolに適合する
- Reviewer ProviderがProducer Providerと異なり、Role IDとSession IDも異なる
- Review上限へ到達していない
- 実装工程では必須検証が成功し、その検証後にReview対象が変更されていない
- 未解決のRequired Change、Publication Conflict、Policy違反がない

すべてを満たした場合だけGateを`pass`とし、原子的な状態遷移で次工程へ進めること。

`0.3.x`のJSON Artifact ReviewではMedia Typeを`application/json`とし、ADR-009の
Markdown Artifact移行後は`text/markdown`とする。移行前後のいずれでもMedia Typeを
Review Subjectへ明示し、暗黙の拡張子推定をGate条件に使用しないこと。

#### FR-058

`changes_requested`は一件以上の`required_changes`を必須とし、修正可能な欠陥を表す
ものとする。`blocked`は一件以上の`open_questions`を必須とし、人間判断または外部状態
変更なしにReviewを完了できない状態を表すものとする。未解決事項が存在する場合は
`approved`または`changes_requested`として扱わないこと。

#### FR-059

Review Schema適合、Reviewer verdict、Gate Evaluatorの結果を別々に記録すること。
Agentの自然言語、終了コード、評価スコア、Human Approvalのいずれか一つだけから
Reviewer ApprovalまたはGate通過を推測してはならない。

### 9.7 修正ループ

#### FR-060

`changes_requested` の場合、Codexへ必須修正を渡し、対象成果物を改版させること。

#### FR-061

修正後は新しい成果物ハッシュを発行し、新規レビューとして扱うこと。

#### FR-062

レビュー回数には工程別の上限を設定できること。

デフォルト値:

```text
要件・設計レビュー: 3回
実装計画レビュー: 3回
マイルストーン実装レビュー: 3回
最終レビュー: 2回
```

#### FR-063

上限へ到達した場合、自動承認せず、人間へ未解決事項を提示して停止すること。

### 9.8 実装計画

#### FR-070

Codexは承認済みの要件と設計から、順序付けられた実装マイルストーンを作成すること。

#### FR-071

各マイルストーンは少なくとも次を含むこと。

- マイルストーンID
- 目的
- スコープ
- 非スコープ
- 依存関係
- 変更候補領域
- 受け入れ条件
- 検証方法
- リスク
- 完了定義

#### FR-072

Claude Codeはマイルストーンの粒度、依存順、検証可能性、要件網羅性、リスクをレビューすること。

#### FR-073

実装計画の承認後、Supervisedモードでは最初のコード変更前に人間の承認を要求すること。

### 9.9 実装

#### FR-080

Codexは一度に一つのマイルストーンだけを実装すること。

#### FR-081

実装開始時点のGitコミット、差分状態、対象マイルストーン、入力成果物ハッシュを記録すること。

#### FR-082

デフォルトでは未コミット変更のある作業ツリーで実装を開始しないこと。

#### FR-083

明示的にDirty Worktreeを許可する場合は、開始時の差分をベースラインとして保存し、既存変更をrctの変更として扱わないこと。

#### FR-084

rctおよびエージェントは、明示承認なしに次を実行しないこと。

- 強制的なGit reset
- 未追跡ファイルの一括削除
- force push
- ブランチ削除
- 本番デプロイ
- Pull Requestのマージ
- 秘密情報の変更

### 9.10 検証

#### FR-090

各マイルストーン実装後、承認済みProject Profileに定義された検証を実行すること。

#### FR-091

検証結果は、コマンド、終了コード、開始・終了時刻、要約、ログファイルを記録すること。

#### FR-092

検証失敗時はコードレビューへ進まず、Codexへ失敗内容を渡して修正させること。

#### FR-093

同一検証の連続失敗回数に上限を設け、上限到達時は停止すること。

### 9.11 コードレビューと修正

#### FR-100

Claude Codeは次を入力としてコードレビューを行うこと。

- 承認済み要件
- 承認済み設計
- 対象マイルストーン
- Git差分
- 変更ファイル一覧
- 検証結果
- 既知の制約

#### FR-101

レビューは少なくとも次の観点を含むこと。

- 要件適合性
- 正しさ
- 既存動作への回帰リスク
- エラーハンドリング
- セキュリティ
- 保守性
- テスト充足度
- スコープ逸脱

#### FR-102

必須修正がある場合、Codexは対象マイルストーンの範囲で修正し、再検証後に再レビューすること。

#### FR-103

任意提案のみの場合は、設定に応じてマイルストーンを承認できること。

### 9.12 完了判定

#### FR-110

マイルストーンは次のすべてを満たした場合のみ完了とすること。

- 必須成果物が存在する
- 成果物Schemaが有効である
- 入出力ハッシュが整合する
- 必須検証が成功している
- 未解決の必須修正がない
- Reviewer verdictが `approved` である

#### FR-111

全マイルストーン完了後、要件トレーサビリティと最終検証を実行すること。

#### FR-112

完了時に、変更要約、検証要約、既知の制約、残課題、成果物へのパスを表示すること。

### 9.13 永続化と再開

#### FR-120

状態変更は原子的に保存すること。

#### FR-121

Runごとに一意なRun IDを発行すること。

#### FR-122

各Agent Jobに一意なJob IDを発行すること。

#### FR-123

次をチェックポイントとして保存すること。

- 現在のWorkflow State
- 現在のマイルストーン
- レビュー回数
- 入力成果物とハッシュ
- 出力成果物とハッシュ
- Backend情報
- Agent session参照
- 実行中または完了済みJob
- 人間の判断履歴

#### FR-124

再開時に、途中生成された未確定成果物を自動採用しないこと。

#### FR-125

Agent sessionを復元できない場合でも、承認済み成果物から新規セッションで再開できること。

### 9.14 ログと監査

#### FR-130

状態遷移、Agent Job、レビュー、検証、人間判断、Backend切替を追記型イベントログへ記録すること。

#### FR-131

標準出力・標準エラーのログをJob単位で保存すること。

#### FR-132

ログ出力時に既知の秘密情報パターンをマスクできること。

#### FR-133

`logs` コマンドでRun、Job、Agent、工程を指定して参照できること。

### 9.15 設定

#### FR-140

設定は次の優先順位で解決すること。

1. CLI引数
2. プロジェクト設定
3. ユーザー設定
4. 組み込みデフォルト

#### FR-141

ユーザー設定の標準パスを次とすること。

```text
$XDG_CONFIG_HOME/rct/config.toml
```

`XDG_CONFIG_HOME` が未設定の場合は `~/.config/rct/config.toml` とする。

#### FR-142

プロジェクト設定の標準パスを次とすること。

```text
<project>/.rct.toml
```

#### FR-143

少なくとも次を設定可能にすること。

- Runtime Backend
- Designer Provider
- Implementer Provider
- Reviewer Provider
- 実行モード
- 工程別最大試行回数
- タイムアウト
- 人間承認ゲート
- 成果物ディレクトリ
- Preview ThemeおよびCustom CSS
- Project Profile
- 実行許可コマンド
- ログレベル

### 9.16 役割割当

#### FR-150

利用者は、Designer、Implementer、Reviewerのそれぞれに、CodexまたはClaude Codeのいずれかを個別に割り当てられること。

#### FR-151

rctは、ReviewerへのProvider割当が、同一RunのDesignerまたはImplementerへの割当のいずれかと一致する場合、Runを開始せずエラーとすること。設定変更によって既存Runの割当が事後的に不整合となった場合も、当該Runの再開を拒否すること。

#### FR-152

`doctor` は、設定済みのDesigner/Implementer/Reviewer割当がFR-151の制約に違反していないかを検査項目に含めること。

#### FR-153

rctは、Designer、Implementer、Reviewerへそれぞれ異なるRole IDとAgent sessionを割り当てること。同一ProviderがDesignerとImplementerを兼任する場合も、会話ContextおよびSessionを共有してはならない。

#### FR-154

利用可能なProviderがCodexとClaude Codeの二つだけで、ReviewerがFR-151を満たす必要がある場合、rctはDesignerとImplementerを同じProviderへ割り当てること。利用者がDesigner Providerを選択した場合、Reviewerのデフォルト値はもう一方のProviderとし、Implementerのデフォルト値はDesignerと同じProviderとする。

### 9.17 利用者向け資料とDocument Compiler

#### FR-160

rctは、少なくとも次の利用者向け資料をGitHub Flavored Markdownと互換性の
あるMarkdownとして出力できること。

- 要件定義書
- アーキテクチャ設計書
- 受け入れ条件
- 実装計画
- マイルストーン定義と実装結果
- レビュー要約
- 検証要約
- 最終報告書

MVPの入力および出力で対応するMarkdown構文を次に限定する。

- ATX形式の見出し
- Paragraph、改行、水平線
- 順序付き・順序なしList
- Task List
- Blockquote
- Emphasis、Strong、Strikethrough
- GFM Table
- Inline CodeとFenced Code Block
- Link、Autolink、相対Link
- 相対Pathの画像

Fenced Code Blockに言語名が指定されている場合、生成HTMLへ`language-<name>`として
保持すること。Raw HTML、Footnote、数式、Mermaidなど上記以外の拡張構文はMVPの
互換性保証対象外とし、無害なTextとして扱うか、未対応警告を出すこと。

#### FR-161

Markdownを人間が閲覧、Git管理、レビューするための正式なDocument Artifactとする。
Workflow制御に必要なSchema version、Run ID、Job ID、入力参照、ハッシュ、状態などは
JSONのArtifact MetadataまたはResult Envelopeとして保持する。

Agentが構造化JSONを返す工程では、rctがSchema検証済みの論理内容から
Markdownを生成すること。Reviewerの承認対象ハッシュは、利用者が実際に読む確定前の
Markdown bytesに対して計算すること。

Agentが返した構造化JSONはJob Resultおよび監査証跡として保存できるが、同じDomain
内容を利用者が手作業でJSONとMarkdownの二箇所へ同期する設計にしてはならない。
後続Agentには確定したMarkdownとArtifact Metadataを渡し、利用者向け内容について
競合する二つの正本を作らないこと。

この要件は、`0.3.x`初期実装の「Agentが返した構造化JSON bytesをReview Subjectと
するPipeline」から、「Schema検証済みJSONをMarkdownへMaterializeし、そのMarkdown
bytesをReview SubjectとするPipeline」への破壊的変更である。FR-160以降の実装を
有効化する前に、Job Envelope、Result Envelope、Review Schema、Gate Evaluator、
`validateReviewSubject`相当処理、既存Testを同時に更新すること。

一つのRun内でJSON HashとMarkdown Hashを暗黙に混在させてはならない。旧形式Runは
旧形式として再開するか、明示的なMigrationにより新しいMarkdown Artifact Versionを
作成して再レビューすること。

#### FR-162

Markdownを人間または外部ツールが変更した場合、保存済みハッシュとの不一致を検出し、
既存の承認を有効なまま扱わないこと。変更後の文書を採用するには、新しいArtifact
Versionとして記録し、必要な再レビューを行うこと。

#### FR-163

rctは、Markdownを入力として静的HTMLとCSSを生成するDocument Compilerを
提供すること。変換時に生成AIを呼び出したり、Markdownに存在しない要件、結論、
レビュー判断を追加したりしてはならない。

#### FR-164

生成するHTMLは、少なくとも次の閲覧支援を提供すること。

- 文書タイトルと成果物種別
- 見出し階層に基づく目次
- 現在位置を把握できるセクション構造
- 表、リスト、引用、コードブロックの判別しやすい表示
- 要件ID、受け入れ条件、リスク、未決事項、レビュー結果の視覚的区別
- 文書間ナビゲーション
- 元のMarkdownへの相対リンク
- 画面幅に応じたレイアウト
- 印刷用スタイル

#### FR-165

Document Compilerは、次のコマンドを提供すること。

```text
rct render --source <markdown-or-directory> [--destination <path>]
rct preview --source <markdown-or-directory> [--destination <path>]
```

`render`はHTML/CSSを生成して終了すること。`preview`は初回Render後、ローカル
Previewを起動すること。単一Markdownと、rctが生成した複数文書のDirectory
の両方を入力にできること。`--destination`未指定時は、単一Markdownではその親
Directoryの`preview/`、Directory入力ではそのDirectory配下の`preview/`を使用する
こと。`start --output-dir`とCompilerの`--destination`を同じ意味のOptionとして
扱わないこと。

#### FR-166

`preview`のHTTP Serverはデフォルトで`127.0.0.1`だけをListenし、外部Interfaceへ
公開しないこと。未指定時はOSが選ぶ空きPortを使用し、起動ごとに固定Portを前提と
しないこと。Loopback以外を示す`Host` Headerを拒否し、CORSを許可せず、GETとHEAD
以外のMethodを拒否すること。`Origin` Headerが存在する場合は、起動時に表示した
Preview Originとの完全一致を要求すること。Preview Serverは状態変更APIを提供
しない。外部公開はMVPの対象外とする。

#### FR-167

CSSはHTML本文から分離した静的Assetとして出力し、rctに組み込まれたDefault
Themeだけで閲覧できること。Font、CSS、JavaScript、画像をCDNから自動取得しては
ならない。Custom CSSは利用者が明示指定したローカルファイルだけを使用できること。

#### FR-168

`start`が利用者向け資料を生成するOutput Rootは、次の優先順位で解決すること。

1. `start`のCLI Option `--output-dir`
2. Project設定の`artifacts.output_dir`
3. `--request-file`で指定したRequest Fileの親Directory
4. `--project`で指定したProject Root
5. Current Working Directory

相対パスはRun開始時のCurrent Working Directoryを基準に絶対パスへ解決すること。

#### FR-169

解決したOutput RootはRun開始時に絶対パスとして保存し、`resume`時にCurrent Working
Directoryや設定が変わっても暗黙に再解決しないこと。保存先が利用不能になった場合は
別の場所へ無断で出力せず停止すること。

#### FR-170

Output Rootが未指定でRequest FileがMarkdownの場合、利用者向けSource Documentは
そのRequest Fileと同じDirectory階層へ配置すること。標準Layoutを次とする。

```text
<request-parent>/
├── <request-name>.md
├── requirements.md
├── architecture.md
├── acceptance-criteria.md
├── implementation-plan.md
├── milestones/
├── reviews/
├── verification/
├── final-report.md
└── preview/
    ├── index.html
    ├── requirements.html
    ├── architecture.html
    ├── implementation-plan.html
    └── assets/
        └── style.css
```

存在しない工程の文書やDirectoryを空で作成する必要はない。

各Review Roundで生成したMarkdownは、まず内部Artifact Storeの版管理Path
（例: `.rct/runs/<run-id>/artifacts/requirements/v002.md`）へ不変Snapshot
として確定すること。Output Rootの`requirements.md`などは、その時点のCandidate
またはApproved Snapshotと同一bytesを持つ、GitHub向けのManaged Publication Copy
とする。Artifact ManifestはPublication Path、参照するVersioned Artifact、
最後にPublishしたSHA-256を対応付けること。

#### FR-171

`--output-dir`にはProject Root外を含む任意の書込可能Directoryを指定できること。
rctはOutput Root内の相対パスを正規化し、`..`、Symbolic Link、または
不正なArtifact名によってOutput Root外へ書き出さないこと。

Output Root自体が利用者に明示指定されたSymbolic Linkである場合は、Run開始時に
実体Pathへ解決して保存すること。Output Root配下の既存Path Componentと書込対象には
書込直前に`Lstat`相当の検査を行い、Symbolic Linkを検出した場合は辿らず競合として
停止すること。原子的renameの直前にも親Directoryと書込対象が検査時から置換されて
いないことを再確認し、物理的に解決したOutput Root外への書込を拒否すること。

#### FR-172

rctは、Output Rootに存在するファイルを暗黙に上書きしないこと。別Runまたは
利用者が作成した同名ファイルに加え、同じRunが所有するPublication Copyも書込前に
現在のSHA-256とArtifact Manifestの`last_published_sha256`を比較すること。

Hashが一致する場合だけ、同じRunの次Versionを原子的にPublishできる。Hashが一致
しない場合は利用者による手編集または外部変更として扱い、Workflowを停止して競合
Path、期待Hash、現在Hashを表示し、次のいずれかを利用者に求めること。

- 別の`--output-dir`を指定する
- Run固有のSubdirectoryを指定する
- 現在のMarkdownを新しいCandidateとして明示的に取り込み、再レビューする
- 利用者の変更を破棄して次VersionをPublishすることを明示確認する

非対話実行では競合を自動解決せず`WAITING_FOR_HUMAN`で停止すること。

#### FR-173

Output Root、生成した利用者向け資料、Preview Artifact、各SHA-256、所有Run IDを
Artifact Manifestへ記録すること。利用者向け資料は一時ファイルとrenameを用いて
原子的に更新すること。

#### FR-174

HTMLはMarkdownからいつでも再生成可能なDerived Artifactとし、Workflowの承認対象、
Agent間の引き継ぎ正本、または再開時の唯一の入力として扱わないこと。HTMLまたはCSSを
手動変更してもMarkdownや承認状態へ逆反映しないこと。

#### FR-175

Preview生成時に、元Markdownのハッシュ、Compiler version、Theme versionをManifest
へ記録すること。Markdownのハッシュが変化した場合、既存Previewを最新として表示せず、
再生成が必要であることを検出できること。

#### FR-176

Document Compilerは、見出しAnchor、文書内リンク、文書間の相対Markdownリンク、
ローカル画像参照をHTML用の相対リンクへ解決すること。解決不能なリンクは警告として
報告し、リンク先を推測して変更しないこと。画像はSource DocumentまたはOutput Root
内の明示的に許可された相対パスだけを対象とし、任意の絶対File Pathを生成HTMLへ
埋め込まないこと。

#### FR-177

Document Compilerは、Raw HTMLと実行可能Scriptをデフォルトで無効化または無害化し、
危険なURL Scheme、Event Handler、外部Resource読込を生成HTMLへ混入させないこと。
生成HTMLにはローカル閲覧を妨げない範囲でContent Security Policyを設定すること。

#### FR-178

同じMarkdown bytes、Compiler version、Theme version、設定からは同じHTML/CSSを
生成できること。生成時刻など内容に不要な非決定値をHTML本文へ埋め込まないこと。

#### FR-179

`.rct/`配下のState、Job Prompt、生のstdout/stderr、Lock、Cacheは内部運用
データとし、利用者向け資料のOutput Root指定とは分離すること。ただし、レビュー要約、
検証要約、最終報告書など利用者が読む資料はOutput Rootへ出力すること。

### 9.18 Human Approval Gate

#### FR-180

Human ApprovalはReviewer Approvalと区別し、Supervisedモードにおいて次の副作用を伴う
工程へ進むことを利用者が許可するAuthorizationとして扱うこと。MVPでは少なくとも、
承認済みImplementation Planから最初のコード変更へ進む直前に要求すること。

#### FR-181

`rct approve`は、現在のRunが明示的なHuman Approval待ち状態にあり、Reviewer
Approvalと決定的Gateがすでに通過している場合だけ受理すること。`changes_requested`、
`blocked`、検証失敗、Schema不正、Hash不一致、Review上限到達を上書きしてはならない。

#### FR-182

Human Approval Recordは少なくともRun ID、Gate Kind、対象PhaseまたはMilestone ID、
承認対象Artifact PathとSHA-256、承認者識別子、任意Note、作成日時を含むこと。
承認は記録された対象Hashだけに有効とし、対象変更後のRunへ流用してはならない。

#### FR-183

Human Approvalは一つのGate遷移に一度だけ使用できること。受理時にApproval Recordの保存、
Event追記、State Revision確認、次状態への遷移を一つの論理操作として行うこと。
State永続化はRun単位の排他Lock内で現在RevisionとExpected Revisionを再読込・比較する
Compare-and-Swapとし、同じRevisionを対象とする同時承認では一件だけを成功させること。

#### FR-184

Reviewerの否決または決定的Gate失敗を利用者が例外的に受容するOverride機能は、通常の
`approve`とは別の操作、権限、理由記録を必要とする。安全な監査仕様が確定するまで
MVPではOverrideを提供しないこと。

### 9.19 Local Browser Control Plane

#### FR-190

rctは次の形式でLocal Browser Control Planeを起動できること。

```text
rct serve [--workspace-root <absolute-path>]... [--listen 127.0.0.1:0]
```

`--workspace-root`を省略した場合は起動時のCurrent Working Directoryだけを許可Rootと
すること。複数回指定を許可し、`/`、Home Directory全体など広すぎるRootを暗黙の
Defaultにしてはならない。

#### FR-191

Control Planeはデフォルトで`127.0.0.1`の空きPortだけをListenし、起動時に推測困難な
Session Tokenを生成すること。Loopback以外へのBindは、危険性を表示した明示設定と
別途定義する認証なしには許可しないこと。MVP/v1初期版ではLoopback以外へのBindを
実装しなくてよい。

#### FR-192

ブラウザの主要操作として、少なくとも`New request`と`New application`を最初の画面に
表示すること。CLIを知らない利用者でも、保存先、要望、Designer、Mode、保存のみか
保存後開始かを一つのFlowで選択できること。

#### FR-193

ブラウザから選択できるDirectoryは、起動時に許可されたWorkspace Rootの配下だけと
すること。Server側でCanonical Path、Path traversal、Symbolic Link、Root containmentを
検証し、Browserから送られた絶対Pathを無条件に信頼してはならない。

Directory選択UIはServerが返したRoot IDと相対Pathを使用し、OS全体のFile Systemを
列挙してはならない。`.git`、`.hg`、`.svn`など既知のVCS Metadata DirectoryはDefaultの
Directory一覧から除外し、New request/New applicationの保存先として選択させないこと。

#### FR-194

`New request`は許可Root内の既存Project Directoryを対象とし、利用者入力から
GitHubで読めるMarkdown Requestを生成すること。Default PathはProject配下の
`requests/<UTC timestamp>-<slug>.md`とし、利用者が明示した許可Root内の別Pathも
選択できること。

#### FR-195

`New application`は許可Root内の親Directoryと検証済みProject slugから新しいDirectoryを
作成し、その直下へ`request.md`を保存すること。既存の非空Directory、File、Symbolic Link
と競合する場合は作成も上書きもせず、利用者へ競合を表示すること。Source Scaffoldと
Dependency installはRequest保存と同時に実行しないこと。Git初期化は、確認画面で独立した
選択項目として明示し、利用者の選択をIntakeへ記録した場合だけFR-230以降のGit Bootstrapを
実行してよい。

#### FR-196

入力Formは少なくとも次を扱うこと。

- Request kind: existing project / new application
- Title
- Rough requestまたはApplication brief
- GoalsまたはDesired outcomes
- Constraints
- Designer Provider
- Run Mode
- Backend
- Max review rounds
- 任意のOutput Directory
- Initialize Git repository（New applicationではDefault ONだが確認画面へ明示する）
- Action: Save draft / Save and start

Titleと本文を必須とし、入力サイズ、文字Encoding、Project slug、数値範囲をServer側でも
検証すること。

#### FR-197

Request MarkdownとIntake Metadataは一時Fileへの書込、Flush、Atomic renameにより保存する
こと。Intake ID、作成時刻、Request kind、Workspace Root ID、相対Path、Request SHA-256、
Idempotency Key、State Revision、選択したRun Optionを
`.rct/intakes/<intake-id>/intake.json`へ保存すること。

#### FR-198

`Save and start`は保存済みRequest PathとSHA-256をApplication Serviceへ渡し、CLIの
`start --request-file`と同じValidation、Provider割当、Backend選択、State Store、
Artifact Storeを利用すること。Web HandlerからCLI Binaryを子Processとして再実行したり、
Web専用Workflowを複製したりしてはならない。

#### FR-199

一つのIntakeから作成できるRunは一つだけとする。主たる重複防止は、ServerがExpected Revisionを
照合し、Intake Stateを`DRAFT`から`STARTING`へCompare-and-Swapで原子的に遷移させることで行う。
異なるIdempotency Keyを持つ同時Start RequestでもCASに成功した一件だけがRunを作成できること。

Idempotency Keyは同一HTTP Requestの再送、Browser再読込、Network retryに対して最初に確定した
Intake ID、Run ID、Responseを再利用する補助機構とし、CASの代替として扱わないこと。

#### FR-200

Control PlaneはRun ID、Project、現在State、Role Provider、Review round、停止理由、主要な
Artifact Linkを表示できること。更新はPollingまたはServer-Sent Eventsで取得し、画面表示を
正式なState Sourceとして扱わないこと。

#### FR-201

Browserを閉じても開始済みRunを暗黙にCancelしないこと。Control Plane再起動後はState Store
からRunを再表示し、CLIの`status`と同じ状態を示すこと。Stop、Resume、Approveなどの状態変更
操作は、各CLI CommandのDomain/Application Serviceが実装された後に同じ契約へ接続すること。

#### FR-202

すべての状態変更HTTP EndpointはPOSTまたはそれ以上に限定されたMethodを使用し、Session
Token、Origin、Host、Content-Type、Body Size、CSRF Tokenを検証すること。GET/HEAD Endpointは
読取専用とし、CORSを有効化しないこと。

#### FR-203

Control PlaneはCSPを設定し、Inline Script、外部Script、外部Font、外部Image、外部Network
RequestをDefaultで許可しないこと。UI AssetはGo Binaryへ埋め込み、利用時にNode.js、npm、
Python、外部CDNを要求しないこと。

#### FR-204

Control Planeは任意Shell Command、任意Executable、任意File Content読取APIを提供しないこと。
Browser入力をCommand引数へ直接連結せず、既存の型付きOptionと承認済みCommand Profileだけを
使用すること。

#### FR-205

Workspace Root、Project Directory、Request File、Output Directoryの各書込処理は、検査時と
使用時の差し替えを考慮したno-follow処理を行うこと。Root外参照、Symbolic Link、Ownership
Conflict、既存利用者FileのHash不一致ではFail Closedとすること。

#### FR-206

Web APIは`/api/v1`でVersion管理し、成功時と失敗時にMachine-readableなJSON Envelopeを返す
こと。内部Error、絶対Path、Prompt、stdout/stderr、秘密情報をBrowserへ無条件に返さないこと。

#### FR-207

初期UIはKeyboard操作、Label、Focus表示、Error Summary、200% Zoom、狭い画面に対応すること。
色だけをRun State、Error、Review verdictの唯一の識別手段にしないこと。

#### FR-208

Control PlaneはCLIの代替ではなく追加Adapterとすること。Headless利用、dotfiles、Automation、
CIでは引き続きCLIから同じApplication Serviceを利用でき、Browserを起動しなくても全Workflowを
実行できること。

#### FR-209

Control PlaneのFrontend SourceはTypeScriptのStrict Modeで記述し、UI LibraryとしてReactと
React DOM、Routing LibraryとしてReact Router Data Modeを使用すること。Routeは
`createBrowserRouter`相当のData Router APIで明示的に定義すること。

React Router Framework Mode、`@react-router/dev`によるFramework Convention、Next.js、Remix、
その他のFull-stack Web Frameworkは使用しないこと。

#### FR-210

Frontendの開発ServerとProduction Asset生成にはViteをBuild Toolとして使用してよい。ただし
Vite、Node.js、npmは開発・Build時だけの依存とし、配布Binaryの実行時依存にしてはならない。
Production AssetはGoの`embed.FS`へ含め、利用者はGo BinaryだけでControl Planeを起動できること。

#### FR-211

Browser Routeは少なくとも次を提供すること。

- `/ui/`: Home
- `/ui/requests/new`: New request
- `/ui/applications/new`: New application
- `/ui/intakes/:intakeId`: Intake confirmation
- `/ui/runs/:runId`: Run detail

Go HTTP Adapterは`/ui/*`の直接Accessと再読込に同じFrontend Entryを返し、`/api/v1/*`および
Static Asset RouteをSPA Fallbackへ誤転送してはならない。

#### FR-212

FrontendのRuntime DependencyはReact、React DOM、React Routerを基本上限とする。State管理Library、
Component Framework、CSS-in-JS Runtime、外部Icon/Font Packageは、具体的必要性とSecurity/Bundle影響を
独立Reviewで承認するまで追加しないこと。Styleは通常のCSSとDesign Token Custom Propertiesで実装すること。

#### FR-213

Frontendは`/api/v1`のRequest/Response DTOをTypeScript Typeとして一箇所に定義し、API Clientを
経由して使用すること。Client側Validationは操作支援であり、Path、権限、入力、State Revision、
Idempotencyの正式Validationは必ずGo Server側で再実行すること。

#### FR-214

Frontend DependencyはLockfileで固定し、CIとRelease BuildでType Check、Unit Test、Production Build、
外部URL検査を実行すること。埋込済みProduction AssetとFrontend Sourceの対応をBuild Manifestまたは
同等のMachine-readable Metadataで検査できること。

### 9.20 Build / Install / Release

#### FR-220

Source Checkoutから`make build`、`make install`、`make uninstall`、`make check`を実行できること。
標準Install先は`$HOME/.local/bin/rct`とし、`PREFIX`で変更可能にすること。

#### FR-221

GitHub ReleaseはmacOS/Linuxのarm64/amd64向けに、Go Runtimeを必要としない単一Binary Archiveを
配布すること。Archive名はOS、Architecture、Versionを一意に識別できること。

#### FR-222

Releaseごとに全ArchiveのSHA-256を含む`checksums.txt`を配布すること。InstallerはChecksum一致前に
BinaryをInstallしてはならない。

#### FR-223

Release InstallerはOSとArchitectureを判定し、Latestまたは明示Versionを取得して、既定では
`$HOME/.local/bin/rct`へInstallすること。Install先は環境変数で変更可能にすること。

#### FR-224

Uninstallerは指定Install Directory内の`rct` Binaryだけを削除し、Directory、User Config、Run State、
Shell設定を暗黙に削除してはならない。

#### FR-225

Push/Pull RequestではRace Test、Vet、Build、Installer Integration Testを実行すること。`v*` Tagでは
対応PlatformのArchiveとChecksumを生成し、GitHub Releaseへ公開すること。

#### FR-226

Providerへ渡すStructured Output Schemaは、rct内のJSON Schema Validatorだけでなく、対象Providerが
受理するSchema Subsetにも適合すること。Claude Codeへ渡すSchemaのTop Levelでは`oneOf`、`allOf`、
`anyOf`を使用しないこと。Review verdictと`required_changes`、`open_questions`の意味的整合性は、
Provider出力後にrctのDomain Validatorが必ず検証し、不整合なReviewをGateへ渡してはならない。

### 9.21 Git Bootstrap / Implementation Preflight / Recovery

#### FR-230

実装を伴うRunは、Human Implementation Approvalを受け付ける前にImplementation Preflightを実行し、
Git実行ファイル、Repository Root、有効な`HEAD`、Clean Worktree、Project Lock、承認対象Plan Hashを
検証すること。Preflight成功時のHEAD CommitをImplementation BaselineとしてRunへ保存すること。

#### FR-231

Git状態を少なくとも次へ分類すること。

- `existing_repository`: Projectが有効なHEADを持つ既存Repository内にある
- `managed_minimal_uninitialized`: Git Repository外にあり、`.rct/`を除くInventoryが、Project直下で
  利用者が`--request-file`で選択した一つの通常Fileと任意の通常File `.gitignore`だけである
- `unmanaged_uninitialized`: Git Repository外にあり、Managed Minimal条件に含まれないEntryが存在する
- `unborn_repository`: `.git`は存在するがHEAD Commitがない
- `unsafe_repository_boundary`: Symbolic Link、Root逸脱、不正なGit metadata、または許可されないNested Repositoryである

Managed Minimal判定はBrowser Intakeの有無へ依存しないこと。CLI利用者が作成した最小Directoryも、
Inventory、明示`--init-git`、TTY確認または`--yes`を根拠にManaged Modeを利用できること。

親DirectoryのRepository内にあるProjectを、Git未初期化と誤判定してNested Repositoryを作成してはならない。
v1ではLinked Worktree（`.git`が`gitdir:` Pointer Fileである構成）と、Project自身がSubmoduleである構成、
Project配下にSubmoduleを含む構成を`unsafe_repository_boundary`として変更せず拒否すること。

#### FR-232

CLIは`rct init --project <path>`を提供し、`rct start`から同じGit Bootstrap Application Serviceを
明示的に呼べるOptionを提供すること。非対話実行でGit変更を行う場合は明示Optionを必須とし、
入力待ちへ移行してはならない。`rct init`は既存Repositoryに対して冪等であること。

#### FR-233

`managed_minimal_uninitialized`では、Browser Intakeの有無にかかわらず、利用者がGit初期化を明示した
場合に限り、次を一つのBootstrap処理として実行できること。

1. Git利用可否とAuthor identityを変更前に検査する
2. Repositoryを初期化する
3. Rootの`.gitignore`へ`/.rct/`を重複なく追加する
4. 利用者が選択したRequest Fileと`.gitignore`だけをStageする
5. 初回Commitを作成し、そのCommit SHAを記録する

Source Scaffold、Dependency install、Remote追加、Pushはこの処理へ含めないこと。

#### FR-234

Managed Minimal条件を超える非空Directoryでは、`git init`、Stage、Commitを暗黙に行わないこと。
既存Fileを初回Baselineへ採用する場合は`--adopt-existing`相当の明示Option、対象File一覧とDigestの
事前表示、対話確認を必要とすること。非対話実行では追加の明示確認Optionを要求すること。

#### FR-235

BootstrapのStage対象はNUL区切り等の安全なPath処理で明示し、Shell文字列、Glob、`git add .`へ
依存しないこと。`.git/`、`.rct/`、許可Root外Path、追跡先を辿ったSymbolic LinkをStageしてはならない。
Symbolic Linkを採用する場合はLink自体のPathとTarget文字列だけを記録し、Target内容を読まないこと。

#### FR-236

初回CommitではProject内またはGlobalのGit Hookを実行せず、自動署名を要求しないこと。利用者の
Remote、Credential、Branch、Global Git Configを変更してはならない。Git Author identityが未設定の
場合は推測値を保存せず、設定方法を示して安全に停止すること。

#### FR-237

Bootstrap完了時に、Repository Root、Project相対Path、Bootstrap Mode、Initial Commit SHA、
対象FileとDigest、`.gitignore`変更前後Hash、実行時刻を含む`git-bootstrap.json`をRunまたはIntakeへ
保存すること。秘密情報、Git credential、Environment値をReceiptへ保存してはならない。

#### FR-238

Git未初期化、Unborn HEAD、Dirty Worktree、Author identity不足など利用者が修正可能なPreflight失敗を
`FAILED`へ遷移させてはならない。`WAITING_FOR_HUMAN`へ遷移し、Machine-readableなReason Code、
再開先State、修正手順、検査時Revisionを保存すること。内部State破損、Artifact改ざん、Policy境界違反は
回復可能な環境不足と区別すること。

#### FR-239

Git Bootstrapまたは環境修正後、利用者は`rct resume --project <path> [--run <id>]`で同じRunを明示的に
再開できること。ResumeはRecovery Planを表示し、Project Lock取得、Reason Code再検査、Plan Hash、
Artifact Hash、State Revision、Git Bootstrap Receipt、現在HEADとWorktreeを検証してからだけ、保存済みの
Resume Targetへ遷移すること。Agentによる要件・設計・Plan生成を再実行してはならない。
v1の`resume`はGit BootstrapおよびImplementation Preflight由来のInterruptionだけを対象とし、Review上限、
Verification上限、Reviewerの`blocked`など一般的なResumeは別Incrementとすること。

#### FR-240

Implementation BaselineはHuman Approval RecordへPlan SHA-256と共にBindingすること。Approval後にHEAD、
Plan Hash、またはBaseline対象Fileが変化した場合、そのApprovalで実装を開始してはならない。Git Bootstrapが
Approval後に必要となったLegacy Runでは、既存ApprovalをSupersededとして保持し、同じRunを
`AWAITING_IMPLEMENTATION_APPROVAL`へ戻して新しいBaselineへの再承認を要求すること。

#### FR-241

`rct implement`はMilestone Stateへ遷移する前にPreflightを再検査すること。回復可能な失敗では承認済み
ArtifactとReviewを保持して`WAITING_FOR_HUMAN`へ停止し、再開後に同じRun・同じMilestoneから開始すること。
Dirty Worktreeを自動Commit、Reset、Clean、Stash、Checkoutしてはならない。

#### FR-242

Git Bootstrap、Resume、Implement Preflight、Milestone Implementation、Verification、Code Review Subject
生成、Final VerificationはProject単位Writer LockとExpected State Revisionを共有すること。Bootstrap Applyは
変更処理の間、`rct implement`は開始PreflightからImplementation Loopの完了・中断・失敗まで同じ排他Leaseを
保持すること。同じProjectの別Runは、実装中RunがLeaseを保持している間は`IMPLEMENTATION_PREFLIGHT`を
通過してはならない。Lock競合は`ConcurrentRunError`として扱い、別Runの`.git`、Index、Stateを変更しては
ならない。Process crash時はOS Lock解放後にRecovery検査から再取得し、MetadataだけでLock所有を判断しないこと。

#### FR-243

Bootstrapは変更前検査をすべて先行し、途中失敗時の所有範囲を記録すること。rctがこの処理で新規作成した
Git metadataまたはStage状態だけを回復対象にできるが、既存`.git`、既存Commit、利用者所有Fileを削除・
Resetしてはならない。自動Rollbackが安全に証明できない場合は部分完了Receiptと手動復旧手順を残すこと。

#### FR-244

Local Browser Control PlaneのNew applicationは`Initialize Git repository`をDefault ONで表示し、確認画面に
初回Commit対象、`.rct/`除外、Remote/Pushを行わないことを示すこと。選択結果をIntakeへ保存し、HTTP Handlerが
Git Commandを直接起動せず、CLIと同じGit Bootstrap Application Serviceを呼ぶこと。

#### FR-245

旧VersionがGit Repository不足を理由に`FAILED`へ遷移させたRunは、Failure文字列だけを一般的なResume権限へ
使用せず、既知の旧Event列、Approval、Plan Hash、未開始Milestoneを検証した限定Migrationで回復できること。
旧Failure文字列は`inspect git worktree`と`not a git repository`などVersionごとの既知Patternへ完全一致または
限定的に一致させ、構造的Predicateをすべて満たす場合だけ補助Evidenceとして使用すること。
回復時は旧Failureを監査履歴に残し、Baseline確立後にHuman Approvalを再要求すること。

### 9.22 Live Progress / Run Observability

#### FR-250

rctはRuntime Backendや表示面に依存しない共通Progress Modelを提供すること。Workflow State、現在Phase、
Activity、Role、Provider、Job ID、Review Round、Artifact Version、開始時刻、最終観測時刻、直前のReview
Verdict、停止理由を、CLI、`rct watch`、Browserから同じ意味で参照できること。

#### FR-251

Workflow Stateと実行中Activityを分離すること。Workflow Stateは承認・遷移の正式状態、Activityは現在実行中の
Jobまたは検証処理を示す観測Snapshotとする。Activity表示、Heartbeat、Terminal AnimationをGate判定や
Artifact承認の根拠として使用してはならない。

#### FR-252

Job Coordinatorは少なくとも次のLifecycle Signalを、Agentまたは画面文字列の推測ではなくrct自身の制御点から
発行すること。

```text
JobQueued
JobStarted
JobHeartbeat
JobOutputObserved
JobCompleted
JobFailed
JobCancelled
ArtifactProduced
ReviewChangesRequested
ReviewApproved
VerificationStarted
VerificationCompleted
RunWaiting
RunCompleted
```

状態変化を表すSemantic EventはRun内で単調増加するSequenceを持ち、UTC Timestamp、Phase、Role、Provider、
Job ID、Round、Workflow State、公開可能なSummaryを含めること。高頻度の`JobHeartbeat`と
`JobOutputObserved`はActivity Revisionを持つLive Signalとして集約でき、永続Semantic Sequenceを無制限に
消費してはならない。

#### FR-253

Runごとに`activity.json`相当のCurrent Activity ProjectionをAtomicに保存すること。少なくとも次を含めること。

```text
status: queued|running|waiting|completed|failed|cancelled|stale
phase
action
role
provider
job_id
round / max_rounds
artifact_kind / candidate_version
started_at
last_heartbeat_at
previous_verdict
required_change_count
```

Activity ProjectionはState SnapshotやEvent Logの代替ではなく、再構築可能な表示用Projectionとすること。

#### FR-254

`rct start`、`plan`、`approve`、`implement`など長時間Commandは、Defaultで実行中Terminalへ進捗を表示すること。
TTYでは同じ表示領域を更新してよいが、非TTYでは一Event一行の追記形式とすること。未知の残り時間や根拠のない
Percentageを表示せず、現在工程、担当、Round、経過時間、最終Activityを表示すること。

#### FR-255

Progress出力はFinal Resultのstdoutと分離すること。DefaultではProgressをstderr、最終Resultをstdoutへ出力し、
`--json`のstdoutを単一の有効なJSONとして維持すること。少なくとも次の選択を提供すること。

```text
--progress auto|tty|plain|jsonl|none
```

`auto`はTTYなら`tty`、非TTYなら`plain`を選択すること。Color、Unicode、Animationが利用できない環境でも
意味を失わないこと。

#### FR-256

Read-only Commandとして次を提供すること。

```text
rct watch --project <path> [--run <id>] [--follow] [--format plain|jsonl]
```

既存Runへ途中参加し、現在Snapshotを直ちに表示した後、新しいEventとActivity変更を追跡できること。
Terminal State到達時は最終Summaryを表示して正常終了し、RunのStateを変更してはならないこと。

#### FR-257

`rct status`は少なくともCurrent Job、Role、Provider、Action、Phase、Round、Candidate Version、Started At、
Elapsed、Last Activity、Livenessを表示すること。`Review verdict`を現在Jobの判定と誤解させず、過去の結果は
`Previous review verdict`と明示すること。生成中は確定済み旧Artifactと生成中Candidateを区別すること。

#### FR-258

`status`と`watch`は`--run <id>`でRunを指定できること。未指定時はProjectのCurrent Runを使用するが、
表示にCurrent Pointer由来であることを明示すること。過去Runを選択してもCurrent Pointerを変更してはならない。

#### FR-259

Provider ProcessとVerification Processのstdout/stderrはProcess終了後の一括保存ではなく、生成された順にJob Logへ
Stream保存すること。File modeは`0600`をDefaultとし、Process異常終了やrct Crash時も書込済み範囲を診断へ
利用できること。UI ProgressはRaw Logの存在や自然言語をJob完了条件として扱ってはならない。

#### FR-260

Progress EventとBrowser DTOへPrompt、Raw stdout/stderr、Credential、Environment、秘密情報らしい値、任意の
Project File内容を含めてはならない。Browserには安全な相対Artifact参照と正規化済みError CodeだけをDefaultで
公開すること。Raw Job LogはLocal Fileとして保持し、専用の明示操作なしにBrowser配信しないこと。

#### FR-261

rct ControllerはJob出力の有無と独立して、実行中ProcessまたはBackend Sessionの観測結果からHeartbeatを更新すること。
Defaultでは10秒以内の間隔で更新し、30秒以上Controller Heartbeatを観測できないActivityを`stale`として表示すること。
`stale`は自動的な`FAILED`判定ではなく、Process/Session、State、ArtifactをRecovery Managerが再検査すべき観測状態とすること。

#### FR-262

Direct、Herdr、tmux Backendは共通ActivityとLifecycle Eventへ正規化すること。Herdrやtmuxの`idle`、Pane文字列、
Process画面は補助観測とし、rctがJobをSubmitしていないBackendの状態をCurrent Runの進捗として表示してはならない。
Backend固有Detailは任意表示に分離し、共通PhaseやGateの意味を変えてはならない。

#### FR-263

BrowserのRun Detailは、少なくとも次を視覚的に区別して表示すること。

- Run全体StateとMode
- 現在Activity Card（担当、Action、Round、経過時間、Liveness）
- Requirements、Architecture、Plan、Human Approval、Milestone、Final ReviewのPhase Timeline
- 完了、実行中、待機、修正中、失敗をText、Icon、Shapeで識別可能な状態
- Previous ReviewのSummaryとRequired Change件数
- 主要Artifact Link
- Error/Waiting Reasonと次に取れるAction

Activityのない待機状態を無限Spinnerで表示せず、Human Approval待ちなど具体的な理由を表示すること。

#### FR-264

Browser ProgressはServer-Sent EventsをDefault Transportとし、次を提供すること。

```text
GET /api/v1/runs/{run-id}/events?after_seq=<n>
GET /api/v1/runs/{run-id}/stream
```

SSEはEvent Sequenceを`id`として送り、`Last-Event-ID`から再接続・Replayできること。接続維持用Commentと
Run Activity Heartbeatを区別し、Browser切断でRunをCancelしないこと。SSE不能時はPollingへFallbackできること。
Durable Event LogはRun存続中にPruneせず、SSE ServerのBounded In-memory Backlog外からの再接続はDurable Logから
Replayすること。遅いSSE Clientは切断できるが、Runを停止せず再接続可能にすること。

#### FR-265

Control Plane再起動、Browser再読込、Network一時切断後も、State Snapshot、Activity Projection、Event Sequenceから
画面を再構成できること。重複EventをSequenceで排除し、欠落を検出した場合はSnapshot再取得後に追跡を再開すること。
In-memory Backlog範囲外であることだけをEvent欠落として扱ってはならない。

#### FR-266

Progress UIはKeyboard操作、Screen Reader向けLive Region、Focus管理、200% Zoom、狭いViewport、Reduced Motionへ
対応すること。ColorとAnimationを状態の唯一の表現にせず、毎HeartbeatをScreen Readerへ読み上げないこと。
重大なPhase変更、失敗、Human Action要求だけを控えめに通知すること。

#### FR-267

Semantic EventのSequence採番と追記はRun State更新と同じCritical Sectionまたは同等の順序保証で行うこと。
HeartbeatはWorkflow State Revisionを増加させず、Activity Projection専用Revisionを使用すること。Watcherは
Writer Lockを取得せずRead-onlyで動作し、部分書込JSONを正式Eventとして解釈してはならない。
Sequenceは独立Counterの事前予約ではなく、Writer Lock取得後にDisk上の最後の確定Eventを再読込し、そのEffective
Sequenceへ1を加えて採番すること。Crashにより未使用Sequence Gapを作ってはならない。

#### FR-268

Progress記録はAgent Jobの実行を著しく遅延させないこと。Raw Log書込にはBounded BufferまたはBackpressureを持たせ、
UI Consumerが遅い場合もProvider Processを無期限に停止させないこと。HeartbeatをSemantic Event Logへ無制限に
追記せず、Activity Projection更新または非永続SSE Heartbeatとして扱うこと。Provider Pipe読取とDisk Log書込も
Byte上限付きQueueで分離し、Disk書込の継続的な遅延または失敗でQueueが上限へ達した場合はRaw Outputを黙って
破棄せず、Jobを`LOG_SINK_BACKPRESSURE`として安全に停止し、Logが不完全であることを診断情報へ記録すること。

#### FR-269

Job失敗時はProvider、Job ID、Phase、経過時間、Machine-readable Error Code、安全なSummary、Job Directoryへの
CLI向け参照、再試行または確認Actionを表示すること。Process Exit Codeだけを表示して終わらず、Raw Error、絶対Path、
秘密情報をBrowserへ無条件に公開しないこと。

#### FR-270

Progress Model、CLI Renderer、`watch`、Streaming Log、SSE Replay、Browser TimelineはFake ClockとFake Providerで
決定的にTestできること。通常Testで実Providerを起動せず、Direct/Herdr/tmuxのContract Fixtureが同じPublic
Progress Sequenceへ正規化されることを検証すること。

## 10. 非機能要件

### NFR-001: ポータビリティ

- macOS arm64 / amd64をサポートする
- Linux arm64 / amd64をサポートする
- rct自身のためにGo、Node.js、Python、jqの追加導入を要求しない
- Codex CLI、Claude Code、選択した任意Backend以外の外部依存を必須にしない
- Markdown変換のためにNode.js、Python、Ruby、Pandoc、Browser拡張を要求しない

### NFR-002: 信頼性

- 状態ファイルは一時ファイルへの書き込みとrenameで原子的に更新する
- 同一Projectの複数RunによるGit Writer処理をProject単位Lockで防止する
- タイムアウトとキャンセルを全Agent Jobへ伝播する
- 異常終了後に最後の確定チェックポイントを復元できる

### NFR-003: 安全性

- Reviewerをデフォルトで読取専用として扱う
- 秘密情報を成果物やログへ意図的に含めない
- 任意コマンド実行を設定と承認で制限する
- 破壊的Git操作を標準機能として使用しない
- プロンプト内のプロジェクト文書を命令ではなく入力データとして区別する
- Markdownおよび生成HTMLを非信頼入力として扱い、Script実行と外部Resource取得を
  デフォルトで許可しない

### NFR-004: 観測可能性

- 現在の工程、Agent、試行回数、経過時間、停止理由を確認できる
- 全状態遷移をイベントログから再構成できる
- JSON出力を用意し、dotfilesや別ツールから利用可能にする

### NFR-005: テスト容易性

- Agent ProviderとRuntime BackendをFake実装へ差し替えられる
- 生成AIを実行せずWorkflow全体をテストできる
- 異常終了、古いレビュー、Schema不正、タイムアウトを再現できる

### NFR-006: 保守性

- Core WorkflowはHerdr、tmux、特定のAI CLIへ直接依存しない
- Artifact Schemaには明示的なバージョンを持たせる
- 状態遷移は列挙されたイベントと状態で定義する
- Promptと評価基準はコードから分離する
- Markdown ThemeとDocument CompilerをWorkflowおよびProvider Adapterから分離する
- HTML/CSSはMarkdownから再生成でき、手作業での同期を必要としない

### NFR-007: 性能

- rct自身の待機時CPU使用率を無視できる水準に保つ
- ポーリングよりイベントまたはプロセス待機を優先する
- 大きなログはメモリへ全件保持せず、ストリームとして保存する

### NFR-008: 可読性とアクセシビリティ

- Semantic HTMLを使用し、見出しLevelを飛ばさない
- Keyboardだけで目次と本文へ移動できる
- Light/Dark環境の双方で十分なContrastを確保する
- 色だけを状態や重大度の唯一の識別手段にしない
- 200% Zoomおよび狭い画面でも本文を横Scrollなしで読めることを目標とする
- Code blockと幅広い表だけは局所的な横Scrollを許可する

## 11. 成果物要件

成果物は、利用者向けDocument Artifactと内部運用Artifactに分離する。

Request FileがMarkdownでOutput Root指定がない場合、利用者向けDocument Artifactは
FR-170に従いRequest Fileと同じDirectory階層へ保存する。任意のOutput Rootが指定
された場合は、その配下へ同じ論理Layoutで保存する。

内部運用Artifactの標準ディレクトリを次とする。

```text
.rct/
├── current-run
├── runs/
│   └── <run-id>/
│       ├── state.json
│       ├── request.md
│       ├── project-profile.json
│       ├── artifacts/
│       │   ├── requirements/
│       │   ├── architecture/
│       │   ├── acceptance-criteria/
│       │   ├── implementation-plan/
│       │   └── milestones/
│       ├── reviews/
│       ├── jobs/
│       ├── verification/
│       ├── logs/
│       └── events.jsonl
└── cache/
```

内部Artifact Storeの`artifacts/<type>/vNNN.md`を改変しないVersioned Snapshotとし、
Output Root上のFlatなMarkdownをManaged Publication Copyとする。両者はPublish時に
同一bytesおよび同一SHA-256を持ち、Artifact Manifestで対応付ける。ReviewはVersioned
SnapshotをSubjectとし、Publication Copyが同じHashであることをGate Evaluatorが
検証する。Publication Copyだけを見て承認済みVersionを推測してはならない。

各成果物には、少なくとも次のメタデータを付与する。

- Schema version
- Run ID
- 生成Job ID
- 作成日時
- 入力成果物の参照とSHA-256
- 生成Agent
- ステータス
- 利用者向けDocument Artifactの出力先
- MarkdownとDerived HTMLのSHA-256

## 12. レビュー結果の論理形式

```json
{
  "schema_version": "1.0",
  "run_id": "run_20260726_xxxxx",
  "job_id": "job_requirements_review_001",
  "review_type": "requirements",
  "subject": {
    "path": "artifacts/requirements/v002.md",
    "sha256": "<sha256>",
    "media_type": "text/markdown"
  },
  "verdict": "changes_requested",
  "scores": {
    "clarity": 3,
    "completeness": 4,
    "feasibility": 4,
    "testability": 2,
    "risk_control": 3
  },
  "required_changes": [
    {
      "id": "RC-001",
      "severity": "high",
      "target": "受け入れ条件",
      "issue": "失敗時の振る舞いが定義されていない",
      "evidence": "FR-090からFR-093に対応する受け入れ条件がない",
      "expected_result": "検証失敗時と上限到達時の確認可能な条件を追加する"
    }
  ],
  "optional_suggestions": [],
  "open_questions": [],
  "summary": "検証失敗時の完了条件を追加したうえで再レビューが必要"
}
```

## 13. 状態

主要なWorkflow Stateを次とする。

```text
NEW
INTAKE
PROJECT_INSPECTION
REQUIREMENTS_DRAFT
REQUIREMENTS_REVIEW
REQUIREMENTS_REVISION
REQUIREMENTS_APPROVED
ARCHITECTURE_DRAFT
ARCHITECTURE_REVIEW
ARCHITECTURE_APPROVED
PLAN_DRAFT
PLAN_REVIEW
PLAN_REVISION
PLAN_APPROVED
IMPLEMENTATION_PREFLIGHT
AWAITING_IMPLEMENTATION_APPROVAL
IMPLEMENTATION_READY
MILESTONE_IMPLEMENTATION
MILESTONE_VERIFICATION
MILESTONE_REVIEW
MILESTONE_FIX
MILESTONE_APPROVED
FINAL_VERIFICATION
FINAL_REVIEW
COMPLETED
WAITING_FOR_HUMAN
BLOCKED
FAILED
CANCELLED
```

`BLOCKED`、`FAILED`、`CANCELLED` から暗黙に実行を再開してはならない。利用者の明示的な `resume` または判断を必要とする。

## 14. Backend別要件

### 14.1 Herdr Backend

- Herdrの実行環境または接続可能なセッションを検出する
- CodexおよびClaude Code用のPaneを作成または再利用する
- Agentへ名前を付与する
- Agent promptとwaitを利用する
- Agentの画面状態だけを完了の唯一の根拠にしない
- 成果物ファイル、Job ID、Schema、ハッシュで完了を確定する
- Session参照を保存し、可能な場合は再開に利用する

### 14.2 tmux Backend

- 専用Session名をRun IDから生成する
- Designer、Implementer、Reviewer、必要に応じてVerification用WindowまたはPaneを作成する
- 既存の無関係なtmux Sessionを変更しない
- `capture-pane` の文字列だけで承認や完了を判定しない
- detach後も処理を継続できる
- 停止時にrctが作成したSessionだけを対象にする

### 14.3 Direct Backend

- 非対話または制御可能なCLI実行を使用する
- 標準出力、標準エラー、終了コードをJob単位で取得する
- rctの終了時に子プロセスを適切に停止する
- Pane表示がなくても他Backendと同一の成果物契約を使用する

## 15. 受け入れ条件

### AC-001

Herdrが利用可能な環境で、概略要望から要件成果物を生成し、Reviewer役割のレビューを経て、承認または最大試行回数到達まで進行できる。

### AC-002

Herdrが存在せずtmuxが存在する環境で、同一の要件定義・レビューフローを実行できる。

### AC-003

Herdrとtmuxが存在しない環境で、Direct Backendにより同一の成果物を生成できる。

### AC-004

不正なReview JSONを受け取った場合、状態を承認済みにせず再試行または停止できる。

### AC-005

古い成果物ハッシュに対するレビューを受け取った場合、そのレビューを拒否できる。

### AC-006

最大レビュー回数へ到達した場合、自動承認せず `WAITING_FOR_HUMAN` または `BLOCKED` になる。

### AC-007

rctを要件レビュー中に強制終了しても、再起動後に確定済み成果物を失わず再開できる。

### AC-008

実装開始前にDirty Worktreeを検出し、デフォルト設定ではコード変更を開始しない。

### AC-009

各マイルストーンは検証成功とReviewer承認の両方がなければ完了にならない。

### AC-010

Reviewerに割り当てたProviderが利用不能な場合、エラー理由を記録して停止し、Designer/Implementerに割り当てたProviderによる自己承認へ暗黙に切り替えない。

### AC-011

既存のAgent CLIが満たすべき依存を除き、rctのためにGo、Node.js、Python、jqを追加インストールせず、配布バイナリを起動できる。

### AC-012

macOS arm64とLinux amd64で同一の設定および成果物Schemaを用いて動作する。

### AC-013

利用者がDesigner ProviderとしてClaude Codeを選択した場合、Claude Codeが要望の整理と要件・設計作成を開始し、CodexがReviewerとして起動する。

### AC-014

DesignerとImplementerへ同じProviderを割り当てた場合でも、両者が異なるRole IDとAgent sessionで実行される。

### AC-015

ReviewerがDesignerまたはImplementerと同一Providerまたは同一Agent sessionになる設定では、Runを開始できない。

### AC-016

`--request-file /work/feature/request.md`を指定し、`--output-dir`と
`artifacts.output_dir`を指定しなかった場合、`requirements.md`などの利用者向け
Source Documentが`/work/feature/`へ作成される。

### AC-017

`--output-dir /work/output`を指定した場合、利用者向け資料とPreview Artifactは
`/work/output`配下へ作成され、Request Fileの親Directoryへ利用者向け資料を作成
しない。

### AC-018

Request Fileを使用せず`--project /work/project`を指定した場合、Output Root未指定時
の利用者向け資料は`/work/project`へ作成される。

### AC-019

生成された`requirements.md`、`architecture.md`、`implementation-plan.md`が、
FR-160で列挙したMVP対応構文だけを使用し、採用したGFM互換ParserまたはLintによる
構文検証を通過する。GitHub.com上の実表示はReleaseごとの手動Spot Checkとし、
自動受け入れ判定を外部Rendererへ依存させない。

### AC-020

`render`を実行すると、元Markdownを変更せずに`preview/index.html`、各文書のHTML、
`preview/assets/style.css`を生成し、Network接続なしで閲覧できる。

### AC-021

Markdownに要件ID、リスク表、コードブロック、レビュー結果が含まれる場合、HTMLで
それぞれに`data-document-kind`、`requirement`、`risk`、`review-verdict`、
`language-*`など仕様で定めた識別可能なSemantic要素またはCSS Classが付与され、
目次LinkのFragmentが対応する見出しIDと一致する。

### AC-022

Output Rootに別Runまたは利用者所有の`requirements.md`が存在する場合、または同一
RunのPublication Copyの現在HashがManifestの`last_published_sha256`と異なる場合、
明示承認なしには上書きせず、競合Path、期待Hash、現在Hash、解決方法を表示して
`WAITING_FOR_HUMAN`で停止する。

### AC-023

Markdown変更後に古いPreviewが残っている場合、ManifestのHash不一致を検出し、
Previewを最新Artifactとして扱わない。

### AC-024

MarkdownにRaw HTML、`javascript:` URL、Inline Event Handler、外部Script参照が
含まれていても、生成HTMLを開いただけでScriptまたは外部Resourceを実行・取得しない。

### AC-025

利用者がMarkdownを手動変更した場合、承認済みArtifactのHash不一致を検出し、
再レビューなしに後続の実装工程へ進まない。

### AC-026

Review待ちの間に利用者がOutput Rootの`requirements.md`を変更し、その後Designerが
次のRevisionを生成した場合、利用者の変更を上書きせず停止する。利用者が取込を選択
した場合は、変更内容を新しいVersioned Candidateとして保存し、Reviewerへ新しい
Markdown Hashを渡す。

### AC-027

Output Root配下の書込対象またはその既存親Path ComponentがSymbolic Linkである場合、
Link先へ書き込まず競合として停止する。検査後にPathがSymbolic Linkへ置換された場合
も、rename直前の再検査によりOutput Root外への書込を拒否する。

### AC-028

`preview`をOptionなしで起動した場合、`127.0.0.1`の空きPortだけでListenする。
Loopback以外を示す`Host` Header、POSTなどGET/HEAD以外のMethod、Cross-Origin
Requestを受け取った場合はDocumentを返さず拒否する。

### AC-029

`verdict: approved`であっても`required_changes`または`open_questions`が一件以上ある
Reviewは拒否され、承認済みStateへ遷移しない。

### AC-030

Review SubjectのRun ID、Job ID、Path、Media Type、SHA-256のいずれかが現在のCandidateと
異なる場合、Reviewerが`approved`を返しても次工程へ進まない。

### AC-031

SupervisedモードでImplementation PlanのGateが通過すると実装開始前に停止し、対象Plan
のSHA-256を含むHuman Approvalを受け取るまでSource Codeを変更しない。

### AC-032

Human Approval後に対象PlanのHashが変わった場合、以前のApprovalを無効として扱い、
新しい対象Hashへの承認なしに実装を開始しない。

### AC-033

`WAITING_FOR_HUMAN`の理由がReview上限、`blocked`、検証失敗、またはHash不一致である場合、
通常の`rct approve`では状態を進められず、理由に応じた修正、回答、再検証、
再レビューのいずれかを要求する。

### AC-034

`rct serve --workspace-root /work`を起動すると`127.0.0.1`の空きPortだけで待受け、
許可Root外のPath、`..` traversal、Symbolic Link経由のDirectory一覧またはFile作成を拒否する。

### AC-035

ブラウザの`New request`から既存Project、Title、要望、Designer、Modeを入力して`Save draft`を
選ぶと、許可Root内へMarkdownとIntake Metadataが原子的に保存され、Agent Jobは開始されない。

### AC-036

同じFormで`Save and start`を選ぶと一つのIntakeと一つのRunだけが作成され、Browserと
`rct status`が同じRun ID、State、Artifact Pathを表示する。

### AC-037

`New application`で未使用のProject slugを指定すると新しいDirectoryと`request.md`だけを作成し、
既存の非空Directory、File、Symbolic Linkと競合する場合は既存内容を変更しない。

### AC-038

同じIdempotency Keyを持つ`Save and start`を二回送信してもRunが重複せず、最初に確定した
Intake IDとRun IDを返す。

### AC-039

不正なHost、Origin、CSRF Token、Session Token、Content-Type、過大Body、状態変更GET Requestを
受け取った場合、Request保存、Run作成、State変更を行わず拒否する。

### AC-040

Control PlaneのHTML/CSS/JavaScriptをNetwork接続なしで表示でき、BrowserのDeveloper Tools上で
外部Script、Font、Image、APIへのRequestが発生しない。

### AC-041

Browserを閉じて再度開いても開始済みRunは継続し、Control Plane Processを再起動した場合は
State Storeから同じRunを表示できる。

### AC-042

Web HandlerのContract TestではFake Application Serviceを使用でき、実Codex、Claude Code、
Herdr、tmuxを起動せずNew request/New application/Run開始/Error処理を検証できる。

### AC-043

配布Binaryから`serve`を起動でき、利用環境へNode.js、npm、Python、Browser Extension、外部Web
Serverを追加Installする必要がない。

### AC-044

Frontend SourceがTypeScript Strict ModeでErrorなくType Checkされ、`.tsx`内で`any`によるAPI
Envelopeの無条件な型回避を行っていない。

### AC-045

`/ui/requests/new`、`/ui/applications/new`、`/ui/intakes/<id>`、`/ui/runs/<id>`をBrowserへ直接入力して
再読込しても該当画面またはMachine-readableなNot Foundを表示し、`/api/v1` ResponseをFrontend
Entryへ置換しない。

### AC-046

CleanなRelease BuildでFrontend Production Assetを生成してGo Binaryへ埋め込める。完成Binaryを
Node.js、npm、Vite、外部CDNがない環境で起動して、全主要RouteとCSS/JavaScriptを読込できる。

### AC-047

Frontend Production DependencyにReact、React DOM、React Router以外が追加された場合はDependency
Policy Gateが失敗するか、承認済み例外記録を要求する。React Router Framework Mode、Next.js、Remix、
SSR用Server Runtimeを含む場合はBuildを失敗させる。

### AC-048

New request/New applicationのForm送信、Intake確認、Run表示のFrontend TestがFake APIで成功し、
同じ操作のHTTP Contract TestがGo Application Serviceへの入力とCSRF/Idempotency Headerを検証する。

### AC-049

Requirements承認済みRunへPlanningを実行すると、Architecture生成・独立Review・必要な修正が先に収束し、
承認されたArchitectureを入力としてImplementation Plan生成・独立Review・必要な修正が実行される。
ArchitectureまたはPlanのReview上限、`blocked`、Stale Hashでは次工程へ進まない。

### AC-050

Supervised Runの承認済みPlan Hashに対してHuman approvalを記録した後、Clean Git Worktreeで
`rct implement`を実行すると、Plan順に一つのMilestoneだけをImplementerが変更し、承認済み
引数配列のVerificationがすべて成功した場合だけ独立Code Reviewへ進む。

### AC-051

Verification失敗時はCode Reviewerを起動せず、失敗記録をImplementerへ渡して有限回修正する。
Code Reviewの`changes_requested`ではReview結果をImplementerへ渡し、再Verification後に新しい
Diff Subject Hashで再Reviewする。全MilestoneがVerification成功かつ`approved`になった場合だけ
Runを`COMPLETED`へ遷移する。

### AC-052

全Milestone承認後、Plan内の全Verification CommandをFinal Verificationとして再実行し、累積Git Diff、
Requirements、Architecture、Plan、検証結果を`review_type: final`で独立Reviewする。Final Reviewの
`changes_requested`はImplementerによる有限修正とFinal Verification/Reviewの再実行へ戻し、Final
Verification成功かつFinal Review `approved`の場合だけ`COMPLETED`へ遷移する。

### AC-053

同じ`DRAFT` IntakeとExpected Revisionに対し、異なるIdempotency Keyを持つ二つのStart Requestを
同時実行した場合、`DRAFT -> STARTING`のCASに成功した一件だけがRunを作成し、もう一件は確定済み
Intake/Runを返すかRevision Conflictとして失敗する。Run IDは二つ作成されない。

### AC-054

Workspace Directory一覧は`.git`、`.hg`、`.svn`を返さず、相対Pathを直接指定してもそれらの配下へ
New requestまたはNew applicationを作成しない。

### AC-055

Implementation Planが`curl`またはAllowlist外ExecutableをVerification Commandへ指定した場合、
rctはProcessを一度も生成せずPlanまたはVerificationを拒否する。許可Commandの子Processには
`PATH`など明示許可されたEnvironmentだけが渡され、親ProcessのCredential環境変数を継承しない。

### AC-056

Git Worktreeに日本語名または空白を含む未追跡Fileがある場合、Code Review Subject生成は失敗せず、
安全な相対PathとSize上限を維持したまま対象Fileの内容をReviewer Evidenceへ含める。

### AC-057

同じRunとExpected State Revisionに対する二つのHuman Approvalを同時実行した場合、Storeの原子的な
Revision CASに成功した一件だけが`IMPLEMENTATION_READY`へ遷移し、もう一件はRevision Conflictまたは
遷移済みStateとして拒否される。

### AC-058

一時Prefixへ`make install`すると実行可能な`rct`が配置され、Versionを表示できる。同じPrefixで
`make uninstall`するとBinaryだけが削除される。

### AC-059

Local Release FixtureをInstallerへ渡すとOS/Architectureに対応するArchiveをChecksum検証後にInstallし、
改ざんしたChecksumでは失敗してInstall先へBinaryを作成しない。

### AC-060

Release Buildはdarwin/arm64、darwin/amd64、linux/arm64、linux/amd64のBinaryを生成し、Tag Versionを
`rct version`へ埋め込める。

### AC-061

全埋め込みStructured Output Schemaに対する静的互換性Testが、Top Levelの`oneOf`、`allOf`、`anyOf`を
検出して失敗する。Review Schemaから条件分岐を除いても、`approved`に必須修正または未解決事項を含む
Review、必須修正のない`changes_requested`、未解決事項のない`blocked`はDomain Validationで拒否される。

### AC-062

Browser Intakeが作成した新規Application、またはCLI利用者が作成した`request.md`以外に利用者Fileを持たない
最小DirectoryでGit初期化を選択すると、Request Fileと`/.rct/`を含む`.gitignore`だけを追跡した初回Commitが
作成され、有効なHEAD、Clean Worktree、Bootstrap Receiptを得る。
Remote追加、Push、Source Scaffold、Dependency installは実行されない。

### AC-063

Git未初期化Projectの承認済みPlanにImplementation Preflightを実行してもRunは`FAILED`にならず、
`GIT_BOOTSTRAP_REQUIRED`とResume Targetを持つ`WAITING_FOR_HUMAN`になる。Agent JobとHuman Approvalは
Preflight解消前に開始されない。

### AC-064

AC-063のProjectで`rct init`と`rct resume`を実行すると、Run ID、承認済みRequirements、Architecture、
PlanとReview Hashを維持したまま`AWAITING_IMPLEMENTATION_APPROVAL`へ進み、AIによる再生成を行わない。

### AC-065

rct所有でない非空Directoryに対して明示的なAdopt OptionなしでGit Bootstrapを要求すると、対象File一覧を
変更せず拒否する。`.git`、Index、Commit、`.gitignore`を作成または変更しない。

### AC-066

既存DirectoryをAdoptする場合、対話確認または同等の非対話向け二重明示がなければ実行されない。承認後は
表示済みInventoryと同一のFileだけが初回Commitへ含まれ、Inventoryが確認後に変化した場合は失敗する。

### AC-067

Projectが親Repository内に存在する場合、rctは新しいNested `.git`を作成せず既存Repository RootとHEADを
Baseline候補として表示する。Repository境界がPolicyに適合しない場合は変更せず停止する。

### AC-068

Git Author identity不足、Commit失敗、Project Lock競合のいずれでも既存利用者Fileと既存Repositoryを
削除・Resetせず、Machine-readableなReasonと安全な再実行手順を返す。

### AC-069

Human ApprovalにBindingされたPlan HashまたはBaseline Commitが変更された状態で`rct implement`を実行すると、
Implementerを起動せずApprovalをstaleとして停止する。同じHashとBaselineを再承認するまで実装へ進まない。

### AC-070

同一Projectに対する二つのGit Bootstrap、または二つの異なるRunの`rct implement`を並行実行すると、一つだけが
Project Writer LeaseとExpected Revisionを取得する。Lease取得者がMilestone実装、Verification、Code Review、
Final Verificationを行っている間、他方は`IMPLEMENTATION_PREFLIGHT`を通過せず、`ConcurrentRunError`でGitと
Run Stateを変更せず終了する。

### AC-071

旧VersionでGit未初期化Errorにより`FAILED`となった、Implementation未開始かつ承認済みPlanを持つRunを
明示Resumeすると、限定Migration、Git Bootstrap、Baseline検証、Human Approval再確認を経て同じRun IDで
実装可能になる。Requirements、Architecture、PlanのAgent Jobは再実行されない。

### AC-072

macOSとLinuxの両方で、InventoryのSymbolic Link非追跡、no-followな`.gitignore`更新、親Repository検出、
Linked WorktreeおよびSubmoduleのFail ClosedをIntegration Testする。いずれの拒否CaseでもProject外File、
既存Git metadata、Index、Commitを変更しない。

### AC-073

Fake ProviderでRequirementsの初稿、Claude Reviewの`changes_requested`、Codex Revision、再Reviewの
`approved`を実行すると、起動CommandのTerminal、`rct watch`、Browser Run Detailが同じJob、Role、Provider、
Round、Artifact Versionを同じ順序で表示し、最終Workflow Stateが一致する。

### AC-074

Plan Round 2をReviewerが実行中のRunで`rct status`を実行すると、Current JobがPlan Reviewer、ProviderがClaude、
ActionがPlan v2のReview、Roundが`2/3`と表示される。Round 1の`changes_requested`は
`Previous review verdict`として表示され、Current VerdictまたはCurrent Artifactと誤認されない。

### AC-075

長時間CommandをTTY、Pipe、`--json`で実行する。TTYは更新表示、Pipeは一Event一行、`--json`はstdoutに単一の
有効なJSONを出力し、Progressはstderrへ分離される。`--progress none`ではProgressを出力せず、いずれも
Workflow Resultは同一になる。

### AC-076

実行中Runへ`rct watch --follow`で途中参加すると、最初にCurrent Snapshot、続けて未観測EventをSequence順に
表示する。別Terminalから複数Watcherを接続してもWriterをBlockせず、Terminal Stateで同じFinal Summaryを
表示して正常終了する。

### AC-077

Fake Providerが複数Chunkを出力した後に異常終了またはControllerを模擬Crashさせる。Process終了前に各Chunkが
`0600`のJob LogへFlushされ、再起動後に書込済み範囲を診断できる。Raw ChunkはProgress EventやBrowser DTOへ
含まれない。

### AC-078

出力を行わない長時間Fake ProviderでもController Heartbeatにより`running`が維持される。Fake Clockで30秒以上
Heartbeatを停止すると`stale`になるが、Workflow Stateは自動的に`FAILED`へ変化せず、再検査Actionが表示される。

### AC-079

Direct、Herdr、tmuxのContract Fixtureで同じLogical Jobを実行すると、公開されるActivityとSemantic Event Sequenceが
Backend Detailを除いて一致する。rctがJobをSubmitしていないHerdr Sessionが`idle`でもCurrent RunのActivityへ
混入しない。

### AC-080

BrowserがSSEのSequence Nまで受信後に切断し、`Last-Event-ID: N`で再接続すると、Nより後のEventだけをReplayする。
In-memory Backlog外でもDurable Event LogからReplayする。Durable Logの欠落、破損、Schema不一致を検出した場合は
Snapshotを再取得し、重複Phaseや重複通知を表示しない。遅いClientをServerが切断しても同じ手順で再接続できる。

### AC-081

Browser Run DetailをKeyboardのみ、Screen Reader、200% Zoom、狭いViewport、Reduced Motionで操作・確認できる。
実行中、修正中、Human Approval待ち、失敗をColorやAnimationだけに依存せず識別でき、待機中に無限Spinnerや
根拠のないPercentageを表示しない。

### AC-082

Prompt、Credentialらしい文字列、Environment、絶対Path、Raw stdout/stderrを含むFake Jobを実行しても、
Progress Event、`status --json`のPublic Field、SSE、Browser DOMへ秘密情報が出ない。CLIの明示的な診断参照だけが
権限を保ったLocal Job Directoryを示す。

### AC-083

旧VersionのSequenceなし、またはSequenceあり・なしが混在する`events.jsonl`を持つRunは、Run終了までFile内の
確定済み物理行順をEffective Legacy Sequenceとして扱う。既存Sequence Fieldは非Authorityとし、既存Fileを
書き換えずCurrent Snapshotを構築する。Upgrade後のWriterも同じRunへLegacy互換Recordを追記し、新旧形式を
切り替えない。`progress-v1`として新規作成されたRunのSequence欠落、重複、逆行はContract Errorとして検出する。

### AC-084

Jobが失敗すると、CLIとBrowserはProvider、Job ID、Phase、経過時間、Error Code、安全なSummary、次の確認または
再試行Actionを表示する。Exit Codeだけの表示で終わらず、失敗表示そのものがRunの再実行やState変更を行わない。

## 16. 初期リスク

| リスク | 影響 | 対応 |
|---|---|---|
| CLIの出力や画面がバージョンで変化する | 完了検出の誤り | 画面解析を補助情報とし、成果物契約を正とする |
| Agent sessionの復元に失敗する | 継続不能 | 成果物から新規Sessionを再構成可能にする |
| レビューが細部へ偏り収束しない | 無限ループ | 回数上限、必須と任意の分離、人間ゲート |
| CodexとClaudeが同じ誤解を共有する | 誤った承認 | 受け入れ条件、根拠提示、検証結果をゲートへ含める |
| プロンプトインジェクション | 不正な操作 | プロジェクト文書を非信頼入力として扱い、権限と実行コマンドを制限 |
| Dirty Worktree上で既存変更を壊す | データ損失 | デフォルト拒否、明示許可時のベースライン保存 |
| Herdr/tmux固有処理がCoreへ漏れる | 保守性低下 | Runtime Backend境界を設ける |
| MarkdownとHTMLの内容が乖離する | 誤った資料を閲覧する | Markdownを正本、HTMLを再生成可能な派生物とする |
| 出力先の既存ファイルを上書きする | 利用者データの損失 | Ownership Manifest、競合停止、原子的改版 |
| Markdown由来のHTMLでScriptが実行される | 情報流出・任意処理 | Raw HTML無効化、URL Sanitization、CSP、外部Resource禁止 |
| Browser UIから任意Pathへ書き込まれる | 利用者Fileの破損 | 明示Workspace Root、相対Path、no-follow、Atomic write |
| 悪意あるLocal Web PageがControl Planeを操作する | 意図しないRun開始 | Session Token、Origin/Host/CSRF検証、CORS無効 |
| 二重送信で複数Runが作成される | 重複費用と競合 | Idempotency Key、Intake State Revision、Run一意制約 |
| Frontend FrameworkがGo CoreとWorkflowを重複実装する | 状態不整合と保守コスト増大 | React Router Data Modeに限定し、Go APIを唯一の正式状態変更境界とする |
| Frontend Build AssetとSourceが乖離する | 古いUIまたは脆弱な依存を配布する | Lockfile、Build Manifest、再現Build、Source/Asset対応Gate |

## 17. 未決事項

実装着手前または技術検証で次を確定する。

1. Codex CLIの非対話実行とSession再開に用いる正式な呼び出し方法
2. Claude Codeの非対話実行とSession再開に用いる正式な呼び出し方法
3. Herdr Backendで長期Sessionを再利用するか、工程ごとに新規Sessionを作るか
4. tmux BackendにおけるAgentプロセス終了と再開の正確な制御方法
5. Reviewerの読取専用権限をCLIレベルでどこまで強制できるか
6. Project Profileの自動推定結果に対する初回承認を必須にするか
7. `.rct/` を標準でGit管理対象にするか、`.gitignore` 対象にするか
8. Skillsをrctリポジトリから各Agentのグローバル領域へコピーするか、シンボリックリンクするか
9. MVPのDefault ThemeでMermaidなどJavaScriptを必要とするDiagramを扱うか、静的画像
   へ限定するか
10. 複数Runの資料を同じOutput Rootへ集約するためのRun Subdirectory Layoutを
    v1で提供するか

## 18. 参考仕様

- Herdr Agent automation: https://herdr.dev/docs/agent-automation/
- Herdr Socket API: https://herdr.dev/docs/socket-api/
- Herdr Plugins: https://herdr.dev/docs/plugins/
- Herdr Agent skill: https://herdr.dev/docs/agent-skill/
- Codex customization: https://learn.chatgpt.com/docs/customization/overview
- Claude Code skills: https://code.claude.com/docs/en/skills
- Claude Code subagents: https://code.claude.com/docs/en/sub-agents
- React TypeScript: https://react.dev/learn/typescript
- React Router modes: https://reactrouter.com/start/modes
- Vite backend integration: https://vite.dev/guide/backend-integration.html
