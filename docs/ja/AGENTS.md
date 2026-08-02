# Loop Engine プロジェクト指示（日本語版）

この文書は、リポジトリ直下の英語版`AGENTS.md`に対応する日本語の参照文書である。
Agentが実行時に従う正本はリポジトリ直下の`AGENTS.md`とする。

## 1. 最初に読むもの

変更前に、タスクへ関係する範囲として最低限次を読むこと。

1. `docs/requirements.md`
2. `docs/architecture.md`
3. リポジトリ直下の`AGENTS.md`

設計とコードが矛盾する場合、推測で設計を変更せず、矛盾点を報告すること。合意された仕様変更は、コードと同じ変更内で文書へ反映すること。

## 2. プロダクトの役割

Loop Engineの標準ロールは次のとおりとする。RoleとProviderは固定対応ではない。

- Designer: 要望整理、要件定義、設計、実装計画
- Implementer: マイルストーン実装、検証、レビュー対応
- Reviewer: 要件、設計、計画、コード、検証結果の独立レビュー
- Loop Engine Core: 状態遷移、Job管理、成果物管理、停止条件、再開
- 利用者: 概略要望、Provider選択、必要な判断、承認、最終受け入れ

CodexとClaudeを直接相互呼び出しさせない。すべての工程遷移はLoop Engine Coreが管理する。

要望を最初に受け取るDesigner Providerは利用者が選択できる。Designer、Implementer、Reviewerは必ず別Role ID・別Agent sessionとする。同一ProviderがDesignerとImplementerを担う場合もSessionや会話Contextを共有しない。Reviewer ProviderはDesignerおよびImplementer Providerと異ならなければならない。

## 3. アーキテクチャ境界

次の境界を維持すること。

- DomainとWorkflowはHerdr、tmux、各AI CLIへ依存しない
- AI固有処理はProvider Adapterへ置く
- Pane、Session、Process固有処理はRuntime Backendへ置く
- Git操作はVCS Adapterへ置く
- プロジェクト検出はProject Inspectorへ置く
- 検証コマンド実行はVerification Runnerへ置く
- ファイル永続化はArtifact StoreとState Storeへ置く

CoreからHerdrやtmuxのコマンドを直接呼び出さない。

## 4. 正式な情報源

ターミナル出力、Agentの自然言語による完了宣言、画面上のstatusを正式な完了条件にしない。

正式な状態は次で確定する。

- Run ID
- Job ID
- Versioned Artifact
- JSON Schema
- SHA-256
- Verification Result
- Reviewer Verdict
- Workflow State

Agent sessionは失われる可能性がある一時的な実行資源として扱う。承認済み成果物から新しいsessionで再開できることを維持する。

## 5. Workflow規則

- 一度に一つのマイルストーンだけを実装する
- Requirements、Plan、Implementationの各レビューに上限を設ける
- 上限到達時に自動承認しない
- `blocked`を`approved`として扱わない
- 古いArtifact Hashに対するReviewを拒否する
- Verification失敗中にCode Reviewへ進まない
- Reviewer承認だけでなく、決定的なGate条件も確認する
- ProducerとReviewerのProviderおよびSession分離を確認する
- Terminal Stateから暗黙に再開しない

状態遷移を追加または変更する場合、遷移テスト、不変条件、復旧経路を同時に更新すること。

## 6. 安全性

既存の利用者変更を保持すること。明示的な依頼なしに次を実行しない。

- `git reset --hard`
- `git clean`
- force push
- ブランチ削除
- 未追跡ファイルの一括削除
- 自動commit
- 自動merge
- 本番デプロイ
- 秘密情報の変更または出力

実装工程では、デフォルトでClean Worktreeを要求する。Dirty Worktree対応を実装する場合は、開始時の差分をBaselineとして保存し、既存変更をLoop Engineの成果として扱わない。

## 7. Process実行

- Shell文字列の連結より、実行ファイルと引数配列を使用する
- `shell=false`相当を標準にする
- Working Directoryを明示する
- TimeoutとContext cancellationを伝播する
- stdoutとstderrをJob単位でストリーム保存する
- 任意コマンドは承認済みCommand Profileからのみ実行する
- Signal処理と子Processの終了をテストする

Shellが必要なプロジェクトコマンドは、明示的にShell実行として設定・承認された場合だけ許可する。

## 8. Reviewerの分離

Reviewer Roleへ割り当てられたProviderは、CodexかClaude Codeかに関係なく原則として次だけを行う。

- プロジェクト、成果物、差分、検証結果の読取
- 所定SchemaのReview結果作成

ReviewerによるSource変更、Git変更、デプロイを許可しない。CLI側で完全に強制できない場合は、Permission設定、書込範囲、Job前後のGit差分検査を組み合わせる。

## 9. 永続化

- State更新は原子的に行う
- Event Logは追記型にする
- Artifactを上書きせず版管理する
- State Revisionで多重更新を検出する
- 同一RunへのWriterをLockで一つに制限する
- 復旧時に未確定Artifactを自動採用しない

## 10. テスト

新しいDomainまたはWorkflow機能にはUnit Testを追加すること。

Adapterを変更する場合はContract Testを追加すること。通常のCIでは実AIを必要としないFake ProviderとFake Backendを使用する。

少なくとも次のケースを維持すること。

- 一回のReviewで承認
- 複数回の修正後に承認
- Review上限到達
- 古いReviewの拒否
- Schema不正
- Verification失敗からの修正
- Reviewer blocked
- 異常終了後のresume
- Herdrからtmux、Directへのauto fallback

標準コマンド:

```text
gofmt -w cmd internal
go test ./...
go vet ./...
go build ./cmd/loop-engine
```

Sandbox環境で標準Go Cacheへ書き込めない場合は、書き込み可能な一時ディレクトリを`GOCACHE`に指定すること。

## 11. 文書

次の変更では文書更新を必須とする。

- Workflow Stateまたは遷移
- ArtifactまたはReview Schema
- CLI commandまたは設定
- Provider Adapterの契約
- Runtime Backendの契約
- Permissionまたは安全境界
- 配布方法

## 12. 現在のスコープ

MVPは次へ限定する。

- macOS arm64
- Linux amd64
- CodexまたはClaude CodeからDesigner Providerを選択
- 選択したProviderを別SessionのDesigner / Implementerへ割当
- もう一方のProviderを独立Reviewerへ割当
- Herdr、tmux、Direct Backend
- Supervised、Autonomous、Design-only
- File-based Artifact、State、Event Log
- 中断と再開

追加Provider、Pull Request連携、デプロイ、Remoteまたは複数利用者向けWeb UIはMVP外とする。FR-190〜FR-214のLoopback限定Local Browser Control Planeはv1拡張であり、CLIと同じApplication ServiceへのInbound Adapterとして維持する。
