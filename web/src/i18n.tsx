import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from "react";

export type Locale = "ja" | "en";

const messages: Record<Locale, Record<string, string>> = {
  ja: {
    runControl: "Run コントロール", localOnly: "ローカルのみ", status: "ステータス",
    activeAgents: "稼働中・待機中", completedRuns: "完了", failedRuns: "失敗・停止",
    noActiveAgents: "稼働中のAgentはありません", noFailedRuns: "失敗したRunはありません",
    selectAgent: "左の一覧からAgentを選択してください", selectAgentBody: "Agentを切り替えると、工程の会話、現在の作業、承認待ちを確認できます。",
    observed: "AI ENGINEERING, OBSERVED", homeTitle: "AIチームの現在地を、ひとつの画面で。",
    homeBody: "CodexとClaudeの設計、レビュー、実装を、推測ではなく永続化されたRun状態から表示します。",
    workflow: "ワークフロー", conversation: "工程の会話", conversationEmpty: "まだ工程イベントはありません。",
    currentActivity: "現在の作業", noActiveJob: "現在実行中のJobはありません",
    phaseTimeline: "工程", artifacts: "成果物", noArtifacts: "公開可能な成果物はまだありません。",
    overallProgress: "全体進捗", milestones: "マイルストーン", phasesComplete: "{{completed}} / {{total}} 工程完了",
    connectionConnecting: "接続中", connectionLive: "ライブ", connectionPolling: "再接続中", connectionCurrent: "最新",
    humanAttention: "確認が必要です", nextAction: "次の操作", approveTitle: "実装開始を承認",
    waitingApproval: "実装開始の承認が必要です", waitingHuman: "Runを続けるには確認が必要です", runBlocked: "Runは停止しており確認が必要です", runFailed: "Runは完了前に停止しました",
    approveBody: "独立レビュー済みのPlanと現在のRevisionに対して、一度だけ承認を記録します。",
    approvalNote: "承認メモ（任意）", reviewApproval: "承認内容を確認", confirmApproval: "このPlanを承認", cancel: "戻る",
    approving: "承認を記録中…", approved: "承認しました", approvalFailed: "承認できませんでした。最新状態を確認してください。",
    reviewBudget: "レビュー回数", roundOf: "{{round}} / {{max}} 回", candidate: "候補版", version: "Version {{version}}",
    liveness: "稼働状態", job: "Job", previousVerdict: "前回レビュー", requiredChanges: "必須修正 {{count}} 件",
    waiting: "待機", running: "実行中", completed: "完了", failed: "失敗", notStarted: "未開始", changesRequested: "修正中", approvedState: "承認済み",
    designer: "設計", implementer: "実装", reviewer: "レビュー", controller: "制御", human: "あなた", rctCore: "rct Core",
    footer: "正式な状態遷移は rct Core が管理します", routeError: "画面を表示できません", backToRuns: "Run一覧へ",
    sessionRequired: "ローカルセッションが必要です", unableToOpen: "rctを開けません", restartServe: "ターミナルで rct serve を再起動してください。",
  },
  en: {
    runControl: "Run Control", localOnly: "Local only", status: "Status",
    activeAgents: "Active & waiting", completedRuns: "Completed", failedRuns: "Failed & stopped",
    noActiveAgents: "No active agents", noFailedRuns: "No failed runs",
    selectAgent: "Select an agent from the left", selectAgentBody: "Switch agents to inspect workflow conversation, current work, and approval gates.",
    observed: "AI ENGINEERING, OBSERVED", homeTitle: "See where the AI team is, in one place.",
    homeBody: "Codex and Claude design, review, and implementation are reconstructed from durable Run state—not guessed from terminal output.",
    workflow: "Workflow", conversation: "Workflow conversation", conversationEmpty: "No workflow events yet.",
    currentActivity: "Current activity", noActiveJob: "No active job",
    phaseTimeline: "Phases", artifacts: "Artifacts", noArtifacts: "No public artifacts yet.",
    overallProgress: "Overall progress", milestones: "Milestones", phasesComplete: "{{completed}} of {{total}} phases complete",
    connectionConnecting: "Connecting", connectionLive: "Live", connectionPolling: "Reconnecting", connectionCurrent: "Current",
    humanAttention: "Human attention", nextAction: "Next action", approveTitle: "Approve implementation start",
    waitingApproval: "Human implementation approval is required", waitingHuman: "Human input is required before the Run can continue", runBlocked: "The Run is blocked and requires attention", runFailed: "The Run stopped before completion",
    approveBody: "Record one approval bound to the independently reviewed Plan and current state revision.",
    approvalNote: "Approval note (optional)", reviewApproval: "Review approval", confirmApproval: "Approve this Plan", cancel: "Back",
    approving: "Recording approval…", approved: "Approved", approvalFailed: "Approval failed. Review the latest Run state.",
    reviewBudget: "Review budget", roundOf: "Round {{round}} of {{max}}", candidate: "Candidate", version: "Version {{version}}",
    liveness: "Liveness", job: "Job", previousVerdict: "Previous review", requiredChanges: "{{count}} required changes",
    waiting: "Waiting", running: "Running", completed: "Completed", failed: "Failed", notStarted: "Not started", changesRequested: "Changes requested", approvedState: "Approved",
    designer: "Designer", implementer: "Implementer", reviewer: "Reviewer", controller: "Controller", human: "You", rctCore: "rct Core",
    footer: "Workflow authority remains in rct Core", routeError: "This view could not be displayed", backToRuns: "View runs",
    sessionRequired: "Local session required", unableToOpen: "Unable to open rct", restartServe: "Return to the terminal and restart rct serve.",
  },
};

interface I18nValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: string, values?: Record<string, string | number>) => string;
}

const I18nContext = createContext<I18nValue | undefined>(undefined);

export function I18nProvider({ children, initialLocale }: { children: ReactNode; initialLocale?: Locale }) {
  const [locale, setLocaleState] = useState<Locale>(() => initialLocale ?? detectedLocale());
  useEffect(() => { document.documentElement.lang = locale; }, [locale]);
  const value = useMemo<I18nValue>(() => ({
    locale,
    setLocale: (next) => {
      setLocaleState(next);
      try { window.localStorage.setItem("rct.locale", next); } catch { /* language preference is optional */ }
    },
    t: (key, values = {}) => {
      let message = messages[locale][key] ?? messages.en[key] ?? key;
      for (const [name, replacement] of Object.entries(values)) message = message.replaceAll(`{{${name}}}`, String(replacement));
      return message;
    },
  }), [locale]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18nValue {
  const value = useContext(I18nContext);
  if (!value) throw new Error("useI18n must be used within I18nProvider");
  return value;
}

export function localizedState(state: string, locale: Locale): string {
  const labels: Record<Locale, Record<string, string>> = {
    ja: {
      INTAKE: "受付", REQUIREMENTS_DRAFT: "要件作成", REQUIREMENTS_REVIEW: "要件レビュー", REQUIREMENTS_REVISION: "要件修正",
      REQUIREMENTS_APPROVED: "要件承認済み", ARCHITECTURE_DRAFT: "設計作成", ARCHITECTURE_REVIEW: "設計レビュー",
      ARCHITECTURE_APPROVED: "設計承認済み", PLAN_DRAFT: "計画作成", PLAN_REVIEW: "計画レビュー", PLAN_REVISION: "計画修正",
      PLAN_APPROVED: "計画承認済み", IMPLEMENTATION_PREFLIGHT: "実装前確認", AWAITING_IMPLEMENTATION_APPROVAL: "実装承認待ち",
      IMPLEMENTATION_READY: "実装準備完了", MILESTONE_IMPLEMENTATION: "実装中", MILESTONE_VERIFICATION: "検証中",
      MILESTONE_REVIEW: "コードレビュー", MILESTONE_FIX: "修正中", FINAL_VERIFICATION: "最終検証", FINAL_REVIEW: "最終レビュー",
      WAITING_FOR_HUMAN: "確認待ち", BLOCKED: "停止", FAILED: "失敗", COMPLETED: "完了", CANCELLED: "キャンセル",
    },
    en: {},
  };
  return labels[locale][state] ?? titleIdentifier(state);
}

export function localizedIdentifier(value: string, locale: Locale): string {
  const ja: Record<string, string> = {
    requirements: "要件定義", architecture: "アーキテクチャ", plan: "実装計画", implementation_plan: "実装計画",
    implementation_preflight: "実装前確認", implementation_approval: "人間による承認", implementation: "実装",
    final_verification: "最終検証", final_review: "最終レビュー", designer: "設計者", implementer: "実装者", reviewer: "レビュアー",
    requirements_review: "要件レビュー", architecture_review: "アーキテクチャレビュー", plan_review: "実装計画レビュー",
    rct: "rct Core", controller: "制御", human: "あなた", codex: "Codex", claude: "Claude",
    queued: "待機中", running: "実行中", waiting: "待機", completed: "完了", failed: "失敗", stale: "更新確認が必要",
    reviewing: "レビュー中", drafting: "作成中", revising: "修正中", verifying: "検証中", changes_requested: "修正要求", approved: "承認済み",
  };
  return locale === "ja" ? (ja[value.toLowerCase()] ?? titleIdentifier(value)) : titleIdentifier(value);
}

export function localizedEvent(value: string, locale: Locale): string {
  const ja: Record<string, string> = {
    RunStarted: "Runを開始しました", JobQueued: "Agent Jobを登録しました", JobStarted: "Agentが作業を開始しました",
    JobCompleted: "Agent Jobが完了しました", JobFailed: "Agent Jobが失敗しました", ArtifactProduced: "成果物を作成しました",
    ReviewApproved: "レビューで承認されました", ReviewChangesRequested: "レビューで修正が求められました",
    ArchitectureApproved: "アーキテクチャが承認されました", PlanApproved: "実装計画が承認されました",
    ImplementationApprovalRequested: "実装開始の承認を待っています", HumanImplementationApprovalConsumed: "実装開始が承認されました",
    RunWaiting: "人間の確認を待っています", RunCompleted: "Runが完了しました", RunFailed: "Runが失敗しました",
    RequirementsDraftingStarted: "要件定義の作成を開始しました", RequirementsArtifactProduced: "要件定義書を作成しました", RequirementsApproved: "要件定義が承認されました",
    ArchitectureDraftingStarted: "アーキテクチャの作成を開始しました", ArchitectureArtifactProduced: "アーキテクチャ文書を作成しました",
    PlanDraftingStarted: "実装計画の作成を開始しました", PlanArtifactProduced: "実装計画書を作成しました",
  };
  return locale === "ja" ? (ja[value] ?? splitEventName(value)) : splitEventName(value);
}

function detectedLocale(): Locale {
  try {
    const saved = window.localStorage.getItem("rct.locale");
    if (saved === "ja" || saved === "en") return saved;
  } catch { /* fall through to browser language */ }
  return typeof navigator !== "undefined" && navigator.language.toLowerCase().startsWith("ja") ? "ja" : "en";
}

function titleIdentifier(value: string): string {
  return value.replaceAll("_", " ").toLowerCase().replace(/\b\w/g, (character) => character.toUpperCase());
}

function splitEventName(value: string): string {
  return value.replace(/([a-z])([A-Z])/g, "$1 $2");
}
