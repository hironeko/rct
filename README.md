# Loop Engine

Loop Engineは、概略的な要望から要件定義、設計、実装計画、実装、検証、レビュー、修正までを、複数の生成AIへ役割分担させて進行するローカルオーケストレーターです。

要望を最初に受け取るDesignerはCodexまたはClaude Codeから選択できます。MVPでは、選択したProviderが独立SessionのDesignerとImplementerを担当し、もう一方が独立Reviewerを担当します。両者を直接再帰的に呼び合わせず、Goで実装する中央のWorkflow Engineが順序、成果物、停止条件、再開処理を管理します。

## Status

GoによるMVP実装を開始しています。

現在実装済み:

- FR-003 / FR-154に基づくProvider割当の導出
- Designer、Implementer、ReviewerのRole・Session分離
- 自己レビュー構成の拒否
- Herdr、tmux、Direct Backendの選択Core
- `start` によるINTAKE Runの初期化と永続化
- `doctor` によるProvider実行ファイル、認証、Backend、Role割当の診断
- `status` による現在Runの表示
- Provider非依存のDesigner / Reviewer Role Contract
- `design-requirements` / `review-artifact` Skill
- Requirements / Review JSON Schema
- Agent出力に対するGo側の独立JSON Schema検証
- Codex / Claude Code Provider Adapter
- Direct Process Runner
- Design-onlyの要件生成、独立レビュー、有限修正ループ
- Review対象ArtifactのPath / SHA-256照合
- Job単位のPrompt、Schema、stdout、stderr、構造化出力保存
- Codex最終出力ファイル欠落時のfail-closed処理

未実装:

- tmux / Herdr Sessionの作成と再開
- Request Bundle
- Architecture ArtifactとImplementation Plan Loop
- Verification、Implementation Loop
- 中断Runのresume
- SkillsのProvider標準ディレクトリへのinstall-assets

## Runtime backends

実行環境は次の優先順で自動選択します。

1. Herdr
2. tmux
3. Direct process

Herdrとtmuxは任意依存です。どちらも存在しない場合でもDirect Backendで動作する設計です。

## Documents

- [要件定義書](docs/requirements.md)
- [アーキテクチャ設計書](docs/architecture.md)
- [共通プロジェクト指示](AGENTS.md)
- [Claude Code向け役割指示](CLAUDE.md)

## Core principles

- ターミナル出力ではなく、Schema・Job ID・ハッシュ付き成果物を正式な状態とする
- Workflow、AI Provider、Runtime Backendを分離する
- Designer、Implementer、Reviewerを別Role・別Sessionとして実行する
- Reviewer ProviderをDesigner/Implementer Providerから分離する
- レビューと修正のループに上限を設ける
- Agent sessionが失われても成果物から再開できるようにする
- Reviewerは原則として読取専用にする
- Supervisedモードをデフォルトとし、コード変更前に人間の承認を要求する
- 破壊的なGit操作、デプロイ、マージを暗黙に実行しない

## Development

```text
go test ./...
go vet ./...
go build -o bin/loop-engine ./cmd/loop-engine
```

現在利用できるコマンド:

```text
loop-engine start
loop-engine doctor
loop-engine status
loop-engine version
```

## Working design-only flow

最小の実フローはDirect Backendで起動します。

```text
loop-engine doctor --backend direct

loop-engine start \
  --project /path/to/project \
  --backend direct \
  --mode design-only \
  --designer codex \
  --request-file /path/to/request.md \
  --max-review-rounds 3 \
  --execute
```

実行前にCodex CLIとClaude Code CLIの両方がインストール・認証済みである必要があります。`--designer claude` を指定すると、Claude CodeがDesigner、CodexがReviewerになります。

現在、実Agent実行に対応するRuntime BackendはDirectのみです。Herdrとtmuxは検出・選択Coreまで実装済みで、Session実制御は次のMilestoneです。

Reviewが `changes_requested` の場合はRequired Changeと直前ArtifactをDesignerへ渡し、新しいVersionを生成します。上限到達時は自動承認せず `WAITING_FOR_HUMAN` で停止します。

`approved`は単独では次工程への許可になりません。Loop Engineは、独立Reviewerの品質承認、対象ハッシュや検証結果を確認する決定的Gate、Supervisedモードでの人間による実装開始許可を別々に記録します。通常の人間承認で、修正要求、検証失敗、古いArtifactへのReviewを上書きしません。

成果物は `.loop-engine/runs/<run-id>/` 配下の `artifacts/`、`reviews/`、`jobs/`へ保存されます。

開始AIをClaude Codeにする例:

```text
loop-engine start \
  --backend direct \
  --mode design-only \
  --designer claude \
  --request "作りたいものの概略" \
  --execute
```

この場合、ImplementerはClaude Code、ReviewerはCodexへ自動的に割り当てられ、三つのRoleは別Session IDを持ちます。

## Planned distribution

macOSおよびLinux向けのGo単一バイナリをGitHub Releasesで配布し、dotfilesからインストールできる形を予定しています。利用環境にはLoop EngineのためのGo toolchainを要求しません。

`go.mod` のModule Pathは、Remote Repository作成前の暫定値として `github.com/hironeko/loop-engine` を使用しています。
