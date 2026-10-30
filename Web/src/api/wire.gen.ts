// Generated from the Go wire types by TestTypeScript. Do not edit: run
// BELTLINE_UPDATE_GOLDENS=1 go test ./internal/server/api -run TestTypeScript.

export const adjustmentFieldValues = ["backend", "model", "thinking", "brief"] as const;
export type AdjustmentField = (typeof adjustmentFieldValues)[number];

export const afkDecisionStatusValues = ["deciding", "applied", "leftForYou", "failed", "superseded"] as const;
export type AfkDecisionStatus = (typeof afkDecisionStatusValues)[number];

export const afkModeValues = ["always", "never", "global"] as const;
export type AfkMode = (typeof afkModeValues)[number];

export const afkOverrulePhaseValues = ["restarted", "held"] as const;
export type AfkOverrulePhase = (typeof afkOverrulePhaseValues)[number];

export const afkSituationKindValues = ["question", "failure", "approval", "hold"] as const;
export type AfkSituationKind = (typeof afkSituationKindValues)[number];

export const attemptInterruptionConfirmationValues = ["pending", "confirmed", "failed"] as const;
export type AttemptInterruptionConfirmation = (typeof attemptInterruptionConfirmationValues)[number];

export const attemptInterruptionContinuationStatusValues = ["queued", "launching", "launched", "failed", "ambiguous", "invalidated"] as const;
export type AttemptInterruptionContinuationStatus = (typeof attemptInterruptionContinuationStatusValues)[number];

export const attemptLaunchModeValues = ["interactive", "headless"] as const;
export type AttemptLaunchMode = (typeof attemptLaunchModeValues)[number];

export const backendValues = ["copilot", "pi", "claude", "codex", "cursor"] as const;
export type Backend = (typeof backendValues)[number];

export const boxMapKindValues = ["trigger", "agent", "approval", "watch"] as const;
export type BoxMapKind = (typeof boxMapKindValues)[number];

export const costProvenanceValues = ["reported", "estimated"] as const;
export type CostProvenance = (typeof costProvenanceValues)[number];

export const eventResourceValues = ["runs", "library", "settings", "state", "intake", "assistant", "afk", "models"] as const;
export const fallbackValues = ["hold", "retry", "drop"] as const;
export type Fallback = (typeof fallbackValues)[number];

export const groupByValues = ["none", "repo", "status", "belt", "epic"] as const;
export type GroupBy = (typeof groupByValues)[number];

export const inlineSpanKindValues = ["text", "strong", "code"] as const;
export type InlineSpanKind = (typeof inlineSpanKindValues)[number];

export const insightsDeltaToneValues = ["better", "worse", "flat", "none"] as const;
export type InsightsDeltaTone = (typeof insightsDeltaToneValues)[number];

export const insightsEvolutionAxisValues = ["version", "week"] as const;
export type InsightsEvolutionAxis = (typeof insightsEvolutionAxisValues)[number];

export const insightsEvolutionOutcomeValues = ["done", "shipped", "dropped"] as const;
export type InsightsEvolutionOutcome = (typeof insightsEvolutionOutcomeValues)[number];

export const insightsFigureIDValues = ["lead", "toPR", "cost", "visits", "touches", "shipRate"] as const;
export type InsightsFigureID = (typeof insightsFigureIDValues)[number];

export const insightsKpiIDValues = ["firstTry", "shipRate", "leadTime", "cost"] as const;
export type InsightsKpiID = (typeof insightsKpiIDValues)[number];

export const insightsPeriodValues = ["7d", "30d", "90d", "all"] as const;
export type InsightsPeriod = (typeof insightsPeriodValues)[number];

export const libraryCardStatusToneValues = ["good", "grey", "need"] as const;
export type LibraryCardStatusTone = (typeof libraryCardStatusToneValues)[number];

export const libraryItemStateValues = ["available", "installed", "update", "conflict"] as const;
export type LibraryItemState = (typeof libraryItemStateValues)[number];

export const libraryPublishValues = ["pullRequest", "push"] as const;
export type LibraryPublish = (typeof libraryPublishValues)[number];

export const libraryRoleValues = ["backup", "shared"] as const;
export type LibraryRole = (typeof libraryRoleValues)[number];

export const pullRequestCheckStateValues = ["pending", "passing", "failing"] as const;
export type PullRequestCheckState = (typeof pullRequestCheckStateValues)[number];

export const pullRequestReviewStateValues = ["approved", "changesRequested", "commented", "dismissed", "other"] as const;
export type PullRequestReviewState = (typeof pullRequestReviewStateValues)[number];

export const pullRequestStateValues = ["open", "merged", "closed"] as const;
export type PullRequestState = (typeof pullRequestStateValues)[number];

export const restartIntentPhaseValues = ["intent", "applied", "held"] as const;
export type RestartIntentPhase = (typeof restartIntentPhaseValues)[number];

export const runBoxesDotValues = ["needs", "held"] as const;
export type RunBoxesDot = (typeof runBoxesDotValues)[number];

export const runBoxesStateValues = ["idle", "done", "todo", "skip", "needs", "held", "running", "watching", "paused"] as const;
export type RunBoxesState = (typeof runBoxesStateValues)[number];

export const runBoxesTrackValues = ["done", "todo", "skip"] as const;
export type RunBoxesTrack = (typeof runBoxesTrackValues)[number];

export const runCommandValues = ["decide", "resume", "answer", "abort", "drop", "park", "unpark", "pause", "resume-run", "retry-continuation", "start-now", "continue-session", "adjust-stage", "afk", "erase", "overrule-afk"] as const;
export type RunCommand = (typeof runCommandValues)[number];

export const runMenuOpensValues = ["command", "send", "sendAgain", "confirm"] as const;
export type RunMenuOpens = (typeof runMenuOpensValues)[number];

export const runPausePhaseValues = ["pausing", "paused", "incomplete", "resuming", "reconcileFailed"] as const;
export type RunPausePhase = (typeof runPausePhaseValues)[number];

export const runPresentationAttemptEntryContentValues = ["running", "decision", "historicalOutput", "empty"] as const;
export type RunPresentationAttemptEntryContent = (typeof runPresentationAttemptEntryContentValues)[number];

export const runStateValues = ["queued", "running", "waiting", "held", "parked", "done", "shipped", "dropped"] as const;
export type RunState = (typeof runStateValues)[number];

export const scheduleDaysValues = ["everyDay", "weekdays", "weekly"] as const;
export type ScheduleDays = (typeof scheduleDaysValues)[number];

export const sessionSidebarProjectionStatusValues = ["live", "problem", "done"] as const;
export type SessionSidebarProjectionStatus = (typeof sessionSidebarProjectionStatusValues)[number];

export const shadowArmValues = ["live", "shadow"] as const;
export type ShadowArm = (typeof shadowArmValues)[number];

export const shadowResultValues = ["accepted", "discarded"] as const;
export type ShadowResult = (typeof shadowResultValues)[number];

export const shadowStageChangeValues = ["added", "changed", "removed"] as const;
export type ShadowStageChange = (typeof shadowStageChangeValues)[number];

export const stageIconValues = ["funnel", "doc", "approve", "wrench", "scope", "ship", "merge", "flag", "eye", "send", "list", "pen", "cycle", "branch", "gauge", "chip", "bolt", "jira", "clock", "hand"] as const;
export type StageIcon = (typeof stageIconValues)[number];

export const stageKindValues = ["agent", "gate", "watch"] as const;
export type StageKind = (typeof stageKindValues)[number];

export const stageStatusLookValues = ["done", "running", "waiting", "held", "plain"] as const;
export type StageStatusLook = (typeof stageStatusLookValues)[number];

export const stageTypeValues = ["agent", "approve", "watch"] as const;
export type StageType = (typeof stageTypeValues)[number];

export const subjectKindValues = ["issue", "pullRequest", "jira", "manual", "scheduled"] as const;
export type SubjectKind = (typeof subjectKindValues)[number];

export const thenActionValues = ["next", "goto", "route", "retry", "hold", "park", "done", "ship", "drop"] as const;
export type ThenAction = (typeof thenActionValues)[number];

export const thinkingValues = ["low", "medium", "high", "xhigh", "max"] as const;
export type Thinking = (typeof thinkingValues)[number];

export const tintValues = ["blue", "sand", "gold", "green", "violet", "rose", "teal", "coral", "grey"] as const;
export type Tint = (typeof tintValues)[number];

export const toneValues = ["good", "you", "problem", "neutral"] as const;
export type Tone = (typeof toneValues)[number];

export const triggerAssigneeValues = ["anyone", "me"] as const;
export type TriggerAssignee = (typeof triggerAssigneeValues)[number];

export const triggerKindValues = ["issues", "reviews", "jira", "schedule", "manual"] as const;
export type TriggerKind = (typeof triggerKindValues)[number];

export const triggerReviewersValues = ["me", "meAndTeams"] as const;
export type TriggerReviewers = (typeof triggerReviewersValues)[number];

export const usageCoverageValues = ["complete", "partial", "untracked"] as const;
export type UsageCoverage = (typeof usageCoverageValues)[number];

export const watchEventValues = ["merged", "closed", "checksFailed", "conflicted", "behind", "approved", "commented", "changesRequested", "newChanges", "none"] as const;
export type WatchEvent = (typeof watchEventValues)[number];

export const watchTargetValues = ["pullRequest", "ticket"] as const;
export type WatchTarget = (typeof watchTargetValues)[number];

export const workspaceValues = ["readOnly", "worktree", "worktreeReadOnly"] as const;
export type Workspace = (typeof workspaceValues)[number];

export interface Adjustment {
  stageId: string;
  date: string;
  fields: AdjustmentField[];
}

export interface AfkDecision {
  id: string;
  runId: string;
  beltName: string;
  subjectRef: string;
  subjectTitle: string;
  stageId: string;
  stageName: string;
  situation: AfkSituationKind;
  reason?: string;
  key: string;
  status: AfkDecisionStatus;
  startedAt: string;
  decidedAt?: string;
  choice?: string;
  action?: string;
  message?: string;
  why?: string;
  detail?: string;
  session?: string;
  sessionId?: string;
  backend?: Backend;
  model?: string;
  thinking?: Thinking;
  folder: string;
  reviewed: boolean;
  situationLabel: string;
  overrule?: AfkOverrule;
  needsReview: boolean;
  label: string;
  tone: Tone;
  headline: string;
  overruleLine?: string;
  runExists: boolean;
  stageExists: boolean;
  folderExists: boolean;
  revealUnavailable?: string;
  overruleOffer?: AfkOverruleOffer;
}

export interface AfkDecisionCounts {
  review: number;
  all: number;
}

export interface AfkDecisionsResponse {
  filter: string;
  counts: AfkDecisionCounts;
  decisions: AfkDecision[];
}

export interface AfkOverrule {
  comments: string;
  stageId: string;
  stageName: string;
  routeName?: string;
  at: string;
  attemptId: string;
  phase: AfkOverrulePhase;
  detail?: string;
  stageLabel: string;
}

export interface AfkOverruleOffer {
  available: boolean;
  reason?: string;
  previous?: AfkTarget;
  choices: AfkTarget[];
  decisionStage?: string;
  currentStage?: string;
  warning: string;
  notice: string;
  commentsHelp: string;
}

export interface AfkRequest {
  enabled: boolean;
}

export interface AfkSettings {
  enabled: boolean;
  answerQuestions: boolean;
  rerunFailures: boolean;
  approve: boolean;
  resolveHolds: boolean;
  drop: boolean;
  limit: number;
  guidance: string;
  backend: Backend;
  model: string;
  thinking?: Thinking;
}

export interface AfkTarget {
  stageId: string;
  label: string;
}

export interface Agent {
  id: string;
  version: number;
  name: string;
  key: string;
  workspace: Workspace;
  slots: number;
  runner: Backend;
  model: string;
  thinking?: Thinking;
  alternates: SessionSettings[];
  tint: Tint;
  icon?: StageIcon;
  outcomes: AgentOutcome[];
  brief: string;
  origin?: LibraryOrigin;
}

export interface AgentAction {
  id: string;
  label: string;
  enabled: boolean;
  reason?: string;
}

export interface AgentChoice {
  id: string;
  name: string;
  version: number;
}

export interface AgentDetailResponse {
  agent: Agent;
  archived: boolean;
  actions: AgentAction[];
  presentation: AgentPresentation;
  folder: string;
  origin?: LibraryOrigin;
}

export interface AgentDraftPresentationRequest {
  agent: Agent;
}

export interface AgentDraftResponse {
  agent: Agent;
  actions: AgentAction[];
  presentation: AgentPresentation;
}

export interface AgentDraftSwitchRunnerRequest {
  agent: Agent;
  runner: Backend;
  alternateIndex?: number;
}

export interface AgentOutcome {
  name: string;
  when: string;
  tone: Tone;
  then: Then;
  to: string[];
}

export interface AgentOutcomeTarget {
  id: string;
  label: string;
}

export interface AgentPresentation {
  isNew: boolean;
  archived: boolean;
  warnings: string[];
  footer: string;
  placeholders: PlaceholderHelp;
  usedBy: AgentUsage[];
  outcomeTargets: AgentOutcomeTarget[];
  glyph: StageIcon;
  status: string;
}

export interface AgentRecordResponse {
  agent: Agent;
  archived: boolean;
  beltCount?: number;
  glyph: StageIcon;
  detail: string;
}

export interface AgentRef {
  id: string;
  version: number;
}

export interface AgentSaveRequest {
  agent: Agent;
}

export interface AgentSnapshot {
  id: string;
  version: number;
  file: string;
}

export interface AgentUsage {
  beltId: string;
  beltName: string;
  version: number;
}

export interface AgentsResponse {
  agents: AgentRecordResponse[];
  choices: AgentChoice[];
  actions: AgentAction[];
  browser?: LibraryBrowser;
}

export interface ApprovalPrompt {
  noteKey: string;
  label: string;
  placeholder: string;
  help: string;
  contextLabel: string;
  context: MarkdownBlock[];
  includeLabel: string;
  include: boolean;
}

export interface ArtifactLinkResponse {
  url: string;
  expiresAt: string;
}

export interface AssistantAction {
  id: string;
  enabled: boolean;
  label: string;
  reason: string | null;
}

export interface AssistantConfiguration {
  backend: Backend;
  model: string;
  thinking?: Thinking;
}

export interface AssistantConfigurationResponse {
  backend: Backend;
  model: string;
  thinking: Thinking | null;
}

export interface AssistantResponse {
  running: boolean;
  busy: boolean;
  session: string;
  command: string[] | null;
  configuration: AssistantConfigurationResponse;
  checkCommand: string[] | null;
  problems: string[];
  actions: AssistantAction[];
}

export interface AssistantStartRequest {
  request: string;
}

export interface Attempt {
  id: string;
  stageId: string;
  number: number;
  startedAt: string;
  endedAt?: string;
  outcome?: string;
  tone?: Tone;
  route?: string;
  note?: string;
  session?: string;
  sessionId?: string;
  settings?: SessionSettings;
  continues?: string;
  replays?: string;
  agentName?: string;
  logMark?: number;
  usage?: Usage;
  usageMark?: number;
  copilotInteractive?: boolean;
  launchMode?: AttemptLaunchMode;
  completionToken?: string;
  usageCheckpoint?: UsageCheckpoint;
  folder: string;
  leg?: string;
  detail?: string;
  afkDecision?: string;
  interruption?: AttemptInterruption;
  readOnlyBaseline?: string;
}

export interface AttemptArtifact {
  name: string;
  sizeBytes: number;
  url: string;
  expiresAt: string;
}

export interface AttemptArtifactsResponse {
  runId: string;
  attemptId: string;
  artifacts: AttemptArtifact[];
  warning: string;
}

export interface AttemptInterruption {
  operation: string;
  legId: string;
  confirmation: AttemptInterruptionConfirmation;
  reason?: string;
  conversation?: string;
  transcriptMark?: number;
  usageMark?: number;
  usageCheckpoint?: UsageCheckpoint;
  continuation?: AttemptInterruptionContinuation;
}

export interface AttemptInterruptionContinuation {
  operation: string;
  status: AttemptInterruptionContinuationStatus;
  reason?: string;
  prompt: string;
}

export interface AttemptOutputResponse {
  runId: string;
  attemptId: string;
  availability: string;
  blocks: MarkdownBlock[];
  message?: string;
}

export interface Belt {
  id: string;
  name: string;
  version: number;
  triggers: TriggerEntry[];
  base: string;
  repositoryBases?: Record<string, string>;
  policies: Policies;
  stages: Stage[];
  forks: Fork[];
  afk: AfkMode;
  origin?: LibraryOrigin;
}

export interface BeltAction {
  id: string;
  enabled: boolean;
  reason?: string;
}

export interface BeltAfkControl {
  mode: AfkMode;
  next: AfkMode;
  on: boolean;
  label: string;
}

export interface BeltAfkRequest {
  afk: AfkMode;
}

export interface BeltContext {
  actions: BeltAction[];
  warnings: string[];
  placeholders?: PlaceholderHelp;
  routes: BeltContextPresentationRouteItem[];
  stages: BeltContextPresentationStageItem[];
  outcomes: BeltContextPresentationOutcomePreview[];
  schedules: BeltContextPresentationSchedulePreview[];
  policies: BeltContextPresentationPolicyOption[];
  policySection?: BeltContextPresentationPolicySection;
  triggerForm?: TriggerFormPresentation;
  stage?: StageMenu;
  notes?: EditorNotes;
  agent?: PinnedAgent;
  line?: StageLinePresentation;
  outdatedStages?: number;
  task?: TaskDraftSelection;
  taskEntries: TaskDraftEntry[];
  modelStatus?: StageModelStatus;
  shadowDifferences?: ShadowStageDifference[];
}

export interface BeltContextPresentationOutcomePreview {
  stage: string;
  outcome: string;
  label: string;
}

export interface BeltContextPresentationPolicyOption {
  key: string;
  label: string;
  detail: string;
  enabled: boolean;
}

export interface BeltContextPresentationPolicySection {
  heading: string;
  options: BeltContextPresentationPolicyOption[];
  note?: string;
}

export interface BeltContextPresentationRouteItem {
  id: string;
  name: string;
  fork: string;
}

export interface BeltContextPresentationSchedulePreview {
  entry: string;
  summary: string;
  next?: string;
  note: string;
}

export interface BeltContextPresentationStageItem {
  id: string;
  name: string;
}

export interface BeltContextRequest {
  draft: Belt;
  stageId?: string;
  shadow?: boolean;
}

export interface BeltDraftOnlyRequest {
  draft: Belt;
}

export interface BeltDraftResponse {
  draft: Belt;
  warnings: string[];
  actions: BeltAction[];
  placeholders?: PlaceholderHelp;
  stash?: TriggerStash;
}

export interface BeltEmptyRequest {
}

export interface BeltPauseRequest {
  paused: boolean;
}

export interface BeltProblem {
  message: string;
  location?: ProblemLocation;
}

export interface BeltResourceResponse {
  resource: BeltSnapshot;
  warnings: string[];
  problems: BeltProblem[];
}

export interface BeltSaveRequest {
  belt: Belt;
}

export interface BeltSnapshot {
  belt?: Belt;
  paused: boolean;
  archived: boolean;
  effectivelyPaused: boolean;
  created: string;
  lastFired: string | null;
  actions: BeltAction[];
  warnings: string[];
  revision: string;
  detail: string;
  afk: BeltAfkControl;
  draft: Belt;
  origin: LibraryOrigin | null;
  shadow?: ShadowSummary;
}

export interface BeltStageTemplateRequest {
  draft: Belt;
  type: StageType;
  agent?: AgentRef;
  route?: string;
  index?: number;
}

export interface BeltStageTemplateResponse {
  stage: Stage;
}

export interface BeltUpdateStageRequest {
  draft: Belt;
  stageId?: string;
  index?: number;
}

export interface BeltsResponse {
  belts: BeltSnapshot[];
  liveIds: string[];
  browser?: LibraryBrowser;
}

export interface BoxMap {
  columns: number;
  nodes: BoxMapNode[];
  edges: BoxMapEdge[];
  routes: BoxMapRoutePill[];
}

export interface BoxMapEdge {
  from: string;
  to: string;
  via: BoxMapTurn[];
  route?: string;
}

export interface BoxMapNode {
  id: string;
  kind: BoxMapKind;
  number: number;
  fork?: string;
  route?: string;
  column: number;
  row: number;
  icon: StageIcon;
  tint?: Tint;
}

export interface BoxMapRoutePill {
  fork: string;
  route: string;
  name: string;
  column: number;
  row: number;
  stageIds: string[];
  feeds: boolean;
}

export interface BoxMapTurn {
  column: number;
  row: number;
}

export interface ContinueSessionResponse {
  run: RunSnapshot;
  command: string[];
  target: string;
}

export interface DateInterval {
  start: string;
  duration: number;
}

export interface DesktopSuccess {
  ok: boolean;
}

export interface EditorNotes {
  whileEditing?: string;
  whenSaved?: string;
}

export interface EraseResponse {
  id: string;
  erased: boolean;
}

export interface ErrorResponse {
  message: string;
  problems: string[];
  locations?: (ProblemLocation | null)[];
}

export interface FactorySummaryCounts {
  running: number;
  needYou: number;
  inProgress: number;
  shippedToday: number;
}

export interface FactoryTrackTrack {
  from: FactoryTrackNode;
  to: FactoryTrackNode;
  belts: string[];
}

export interface FloorTrack {
  layers: FactoryTrackNode[][];
  tracks: FactoryTrackTrack[];
  returns: FloorTrackReturn[];
}

export interface FloorTrackReturn {
  from: FactoryTrackNode;
  to: FactoryTrackNode;
  belts: string[];
  label: string;
}

export interface Fork {
  id: string;
  routes: Route[];
  parent?: string;
  at?: number;
}

export interface GlobalAction {
  id: string;
  label: string;
  enabled: boolean;
  reason: string | null;
  help?: string;
}

export interface HeldAnswerCopy {
  stage: string;
  buttonTitle: string;
  buttonHelp: string;
  note: string;
}

export interface InlineSpan {
  type: InlineSpanKind;
  text: string;
}

export interface InsightsBelt {
  id: string;
  label: string;
}

export interface InsightsBeltRow {
  id: string;
  name: string;
  runs: number;
  shipRate: InsightsRate;
  firstTry: InsightsRate;
  leadTime: string;
  costPerRun: string;
  trend: InsightsSeries;
}

export interface InsightsBin {
  from: number;
  to: number;
  label: string;
  before: number;
  recent: number;
}

export interface InsightsColumn {
  label: string;
  runs: number;
  changes: number;
  changeCaption: string;
}

export interface InsightsColumnStat {
  value: number | null;
  low: number | null;
  high: number | null;
  display: string;
  runs: number;
}

export interface InsightsDelta {
  label: string;
  tone: InsightsDeltaTone;
  caption: string;
}

export interface InsightsDot {
  run: number;
  value: number;
  display: string;
  partial?: boolean;
}

export interface InsightsDropRow {
  stage: string;
  before: number;
  recent: number;
  reason: string;
}

export interface InsightsEndings {
  funnel: InsightsFunnelRow[];
  drops: InsightsDropRow[];
}

export interface InsightsEvolutionRun {
  id: string;
  ref: string;
  title: string;
  version: number;
  outcome: InsightsEvolutionOutcome;
  finished: string;
  column: number;
}

export interface InsightsEvolutionView {
  belt: string;
  belts: InsightsBelt[];
  title: string;
  caption: string;
  axis: InsightsEvolutionAxis;
  separate: boolean;
  columns: InsightsColumn[];
  from: number;
  split: string;
  runs: InsightsEvolutionRun[];
  figures: InsightsFigure[];
  time: InsightsStack;
  touches: InsightsStack;
  sendsBack: InsightsGrid;
  cost: InsightsStack;
  endings: InsightsEndings;
}

export interface InsightsFigure {
  id: InsightsFigureID;
  label: string;
  value: string;
  change: string;
  caption: string;
  share: boolean;
  scalable: boolean;
  columns: InsightsColumnStat[];
  dots: InsightsDot[];
  min: number;
  logMin: number;
  max: number;
  ticks: InsightsTick[];
  logTicks: InsightsTick[];
  spread?: InsightsSpread;
}

export interface InsightsFigures {
  visits: number;
  visitsPerRun: string;
  lowSample: boolean;
  medianTime: string;
  firstTry: InsightsRate;
  tries: string;
  cost: string;
  costCaption: string;
}

export interface InsightsFunnelRow {
  label: string;
  before: InsightsRate;
  recent: InsightsRate;
}

export interface InsightsGrid {
  rows: InsightsGridRow[];
  max: number;
}

export interface InsightsGridRow {
  label: string;
  values: number[];
  displays: string[];
}

export interface InsightsModel {
  model: string;
  runner: string;
  best: boolean;
  figures: InsightsFigures;
}

export interface InsightsPeriodChoice {
  value: InsightsPeriod;
  label: string;
}

export interface InsightsPoint {
  label: string;
  value: number | null;
  display: string;
  count: number;
}

export interface InsightsRate {
  count: number;
  of: number;
  percent: string;
}

export interface InsightsSeries {
  points: InsightsPoint[];
  delta: InsightsDelta;
  max: number;
}

export interface InsightsShadow {
  test: string;
  running: boolean;
  result?: ShadowResult;
  dates: string;
  caption: string;
  live: InsightsShadowArm;
  shadow: InsightsShadowArm;
  figures: InsightsShadowFigure[];
  stages: InsightsShadowStage[];
  onlyShadow: InsightsStage[];
  onlyLive: InsightsStage[];
}

export interface InsightsShadowArm {
  label: string;
  finished: number;
  going: number;
  caption: string;
}

export interface InsightsShadowFigure {
  id: InsightsKpiID;
  label: string;
  live: InsightsShadowValue;
  shadow: InsightsShadowValue;
  delta: InsightsDelta;
}

export interface InsightsShadowStage {
  key: string;
  name: string;
  kindLabel: string;
  changed: boolean;
  live: InsightsFigures;
  shadow: InsightsFigures;
  tones: InsightsShadowTones;
}

export interface InsightsShadowTones {
  visitsPerRun: InsightsDeltaTone;
  medianTime: InsightsDeltaTone;
  firstTry: InsightsDeltaTone;
  tries: InsightsDeltaTone;
  cost: InsightsDeltaTone;
}

export interface InsightsShadowValue {
  value: string;
  caption: string;
  bar: number;
}

export interface InsightsSpread {
  bins: InsightsBin[];
  logBins: InsightsBin[];
  before: number[];
  recent: number[];
  headers: string[];
  rows: InsightsSpreadRow[];
}

export interface InsightsSpreadRow {
  label: string;
  runs: number;
  cells: string[];
}

export interface InsightsStack {
  series: InsightsStackSeries[];
  line?: InsightsStackLine;
  max: number;
  ticks: InsightsTick[];
  runs: string[];
}

export interface InsightsStackLine {
  label: string;
  values: number[];
  displays: string[];
}

export interface InsightsStackSeries {
  label: string;
  values: number[];
  displays: string[];
}

export interface InsightsStage {
  key: string;
  name: string;
  caption: string;
  kindLabel: string;
  figures: InsightsFigures;
  models: InsightsModel[];
}

export interface InsightsTick {
  value: number;
  label: string;
}

export interface InsightsVersionChoice {
  value: number;
  shadow?: string;
  label: string;
}

export interface InsightsView {
  belt: string;
  version: number;
  versions: InsightsVersionChoice[];
  period: InsightsPeriod;
  belts: InsightsBelt[];
  periods: InsightsPeriodChoice[];
  title: string;
  caption: string;
  lowSampleThreshold: number;
  beltRows: InsightsBeltRow[];
  stages: InsightsStage[];
  shadow?: InsightsShadow;
}

export interface IssueFilter {
  label: string;
  assignee: TriggerAssignee;
  poll: number;
}

export interface JiraQuery {
  jql: string;
  poll: number;
}

export interface Join {
  id: string;
  forkId: string;
  routes: string[];
  parent?: string;
  legs: string[];
  escaped: boolean;
  into?: string;
  checkout?: string;
  joined: boolean;
  afterStage?: string;
  afterRoutes: Record<string, string[]>;
}

export interface Leg {
  id: string;
  route?: string;
  stageId?: string;
  state: RunState;
  reason?: string;
  note?: string;
  continues?: string;
  replays?: string;
  branch?: string;
  join?: string;
  depart?: string;
  departJoins?: string[];
}

export interface LibraryAction {
  id: string;
  enabled: boolean;
  reason?: string;
}

export interface LibraryBackupResponse {
  commit: string | null;
  changed: boolean;
}

export interface LibraryBrowser {
  items: LibraryCard[];
  filters: LibraryFilter[];
  totalCount: number;
  matchCount: number;
  actions: LibraryBrowserAction[];
}

export interface LibraryBrowserAction {
  id: string;
  label: string;
  enabled: boolean;
  reason?: string;
}

export interface LibraryCard {
  id: string;
  name: string;
  version: number;
  summary: string;
  detail: string;
  status: string;
  statusTone: LibraryCardStatusTone;
  icon?: StageIcon;
  tint?: Tint;
  stages: LibraryCardStage[];
  shadow?: string;
}

export interface LibraryCardStage {
  icon: StageIcon;
  tint: Tint;
}

export interface LibraryFilter {
  id: string;
  label: string;
  count: number;
}

export interface LibraryInstallResponse {
  written: string[];
}

export interface LibraryItem {
  kind: string;
  id: string;
  name: string;
  version: number;
  localVersion: number | null;
  state: LibraryItemState;
  reason: string | null;
  actions: LibraryAction[];
}

export interface LibraryItemFile {
  path: string;
  repo: string | null;
  local: string | null;
}

export interface LibraryItemRequest {
  repository: string;
  path: string;
  kind: string;
  id: string;
}

export interface LibraryItemResponse {
  files: LibraryItemFile[];
}

export interface LibraryNavigation {
  offered: boolean;
  changedSinceBackup: boolean | null;
}

export interface LibraryOrigin {
  repository: string;
  path: string;
}

export interface LibraryRepoRequest {
  repository: string;
  path: string;
}

export interface LibraryRepoResponse {
  repository: string;
  path: string;
  role: LibraryRole;
  publish: LibraryPublish;
  syncedAt: string | null;
  commit: string | null;
  problem: string | null;
  changedSinceBackup?: boolean;
  items: LibraryItem[];
  actions: LibraryAction[];
}

export interface LibraryReposResponse {
  repos: LibraryRepoResponse[];
}

export interface LibraryRepository {
  repository: string;
  path: string;
  role: LibraryRole;
  publish: LibraryPublish;
}

export interface LibraryRestoreResponse {
  savedTo: string;
}

export interface LibraryShareResponse {
  pullRequest: string | null;
  commit: string | null;
  changed: boolean;
}

export interface Merge {
  branches: string[];
  into?: string;
  checkout?: string;
  leg?: string;
  context?: string;
}

export interface MissingResource {
}

export interface ModelActionDecision {
  available: boolean;
  refusalReason: string | null;
}

export interface ModelAdmission {
  schemaVersion: number;
  models: Record<string, ModelChoice[]>;
  legacyRepairs: string[];
  legacyAttemptSettings?: Record<string, SessionSettings>;
}

export interface ModelAllowRequest {
  model: string;
  allowed: boolean;
}

export interface ModelCandidateActionDecision {
  id: string;
  allowed: boolean;
  available: boolean;
  refusalReason: string | null;
}

export interface ModelCatalogSnapshot {
  runner: Backend;
  executable: string;
  refreshedAt: string;
  models: ModelChoice[];
}

export interface ModelChoice {
  id: string;
  name: string;
  variants?: Record<string, string>;
  contextTokens?: number;
}

export interface ModelListing {
  id: string;
  label: string;
  allowed: boolean;
}

export interface ModelRunnerActions {
  refresh: ModelActionDecision;
  candidates: ModelCandidateActionDecision[];
}

export interface ModelRunnerResource {
  runner: Backend;
  executable: string;
  catalog: ModelCatalogSnapshot | null;
  usable: boolean;
  loading: boolean;
  error: string | null;
  allowedModels: string[];
  choices: ModelChoice[];
  unavailableModels: string[];
  thinking: ThinkingChoice[];
  thinkingNote: string | null;
  listing: ModelListing[];
  actions: ModelRunnerActions;
}

export interface NavigationSummary {
  belts: number;
  agents: number;
  afkToCheck: number;
  library: LibraryNavigation;
}

export interface Notice {
  id: string;
  date: string;
  text: string;
  runId: string | null;
}

export interface Outcome {
  id: string;
  name: string;
  when: string;
  tone: Tone;
  then: Then;
}

export interface OverviewFloor {
  total: FactorySummaryCounts;
  belts: OverviewFloorBelt[];
  boxes: OverviewFloorBox[];
  track: FloorTrack;
}

export interface OverviewFloorBelt {
  id: string;
  name: string;
  summary: string;
  paused: boolean;
  counts: FactorySummaryCounts;
}

export interface OverviewFloorBox {
  id: string;
  name: string;
  caption: string;
  icon: StageIcon;
  tint: Tint;
  belts: string[];
  status: OverviewFloorBoxStatus;
  runCount: number;
  chips: OverviewFloorChip[];
  more: number;
}

export interface OverviewFloorBoxStatus {
  text: string;
  look: StageStatusLook;
}

export interface OverviewFloorChip {
  runId: string;
  ref: string;
  belt?: string;
  state: RunState;
  paused: boolean;
  openStage?: string;
}

export interface OverviewResponse {
  summary: RunCounts;
  caption: string;
  greeting: string;
  today: OverviewSummary;
  floor: OverviewFloor;
  runs: OverviewRun[];
}

export interface OverviewRun {
  id: string;
  ref: string;
  title: string;
  stageName: string;
  repository: string;
  beltId: string;
  beltName: string;
  location: StatusWhere;
  needsYou: boolean;
  openStage?: string;
}

export interface OverviewSummary {
  spend: OverviewSummarySpend;
  sessions: OverviewSummarySessions;
  running: OverviewSummaryRunning;
  shipped: OverviewSummaryShipped;
  inProgress: OverviewSummaryRows;
  needsYou: OverviewSummaryRows;
}

export interface OverviewSummaryChart {
  startLabel: string;
  endLabel: string;
  latestLabel: string;
}

export interface OverviewSummaryRow {
  runId: string;
  glyph: string;
  look: StageStatusLook;
  title: string;
  chip: string;
  openStage?: string;
}

export interface OverviewSummaryRows {
  count: number;
  rows: OverviewSummaryRow[];
  empty: string;
}

export interface OverviewSummaryRunning {
  count: number;
  runIds: string[];
  remaining: number;
  empty: string;
  busy: string;
}

export interface OverviewSummarySessions {
  today: number;
  running: number;
  start: string;
  startLabel: string;
  shades: number[];
  runningCaption: string;
}

export interface OverviewSummaryShipped {
  weeks: number[];
  latest: number;
  median: string;
  chart: OverviewSummaryChart;
}

export interface OverviewSummarySpend {
  amount: string;
  caption: string;
  note?: string;
}

export interface Parking {
  routes: ParkingRoute[];
  stopping: string[];
  stopError?: string;
  unparkPending: boolean;
  at: string;
}

export interface ParkingRoute {
  leg: string;
  stageId?: string;
  state: RunState;
  reason?: string;
  note?: string;
  continues?: string;
  replays?: string;
  interrupted?: string;
  session?: string;
}

export interface PauseRequest {
  paused: boolean;
}

export interface PinnedAgent {
  id: string;
  version: number;
  name: string;
  brief: string;
  key?: string;
  latestVersion?: number;
  archived: boolean;
  missing: boolean;
}

export interface PlaceholderHelp {
  entries: PlaceholderHelpEntry[];
  qualification?: string;
  legacy: boolean;
  prose: PlaceholderHelpProse;
}

export interface PlaceholderHelpEntry {
  name: string;
  description: string;
}

export interface PlaceholderHelpProse {
  introduction: string;
  resolution: string;
  warnings: string;
  syntax: string;
  example: string;
  renderedExample: string;
  exampleNote: string;
  resolutionNote: string;
}

export interface PlaceholdersResponse {
  placeholders: PlaceholderHelp;
}

export interface Policies {
  retriageOnEdit: boolean;
  stopOnClose: boolean;
  commentOnHold: boolean;
  waitForBlockers: boolean;
}

export interface ProblemLocation {
  kind: string;
  id?: string;
  field?: string;
}

export interface PullRequest {
  repo: string;
  number: number;
  url: string;
  state: PullRequestState;
  headSha?: string;
  baseRef?: string;
  checks?: PullRequestCheckState;
  checkFailures?: number;
  checkTotal?: number;
  conflicted?: boolean;
  behind?: number;
  reviews?: PullRequestReview[];
}

export interface PullRequestReview {
  id?: string;
  author: string;
  state: PullRequestReviewState;
  commit?: string;
  submittedAt?: string;
}

export interface RateProvenance {
  revision: string;
  source: string;
  models: string[];
}

export interface RestartIntent {
  decisionId: string;
  attemptId: string;
  stageId: string;
  comments: string;
  at: string;
  phase: RestartIntentPhase;
  detail?: string;
}

export interface Result {
  name: string;
  purpose: string;
  ok: boolean;
  detail: string;
  optional: boolean;
}

export interface ReviewFilter {
  reviewers: TriggerReviewers;
  poll: number;
}

export interface Route {
  id: string;
  name: string;
  feeds?: boolean;
}

export interface Run {
  id: string;
  beltId: string;
  beltName: string;
  beltVersion: number;
  shadow?: RunShadow;
  stages: Stage[];
  forks: Fork[];
  adjustments: Adjustment[];
  policies: Policies;
  base: string;
  baseFromLabel?: boolean;
  repositoryBases?: Record<string, string>;
  repos: string[];
  source?: RunSource;
  issue: Subject;
  includeSubject?: boolean;
  state: RunState;
  stageId: string;
  reason?: string;
  shipping?: boolean;
  note?: string;
  continues?: string;
  replays?: string;
  uses: Record<string, number>;
  routes: RouteSet;
  legs: Leg[];
  mergeBranches: string[];
  mergeInto?: string;
  mergeCheckout?: string;
  mergeLeg?: string;
  mergeContext?: string;
  mergesWaiting: Merge[];
  mergeNote?: string;
  mergeNoteLeg?: string;
  joins: Join[];
  attempts: Attempt[];
  branch?: string;
  pullRequests: PullRequest[];
  firstPullRequestAt?: string;
  agents: AgentSnapshot[];
  references?: number;
  modelAdmission?: ModelAdmission;
  watchObservations: Record<string, WatchObservation>;
  ticketObservations: Record<string, TicketObservation>;
  afkEnabled?: boolean;
  generation: number;
  operation: number;
  restart?: RestartIntent;
  pause?: RunPause;
  pauseTime: RunPauseInterval[];
  parking?: Parking;
  createdAt: string;
  updatedAt: string;
}

export interface RunAction {
  id: string;
  enabled: boolean;
  reason?: string;
  stage?: string;
  leg?: string;
  outcome?: string;
  attempt?: string;
  decision?: string;
  help?: string;
  previous?: boolean;
  needsNote?: boolean;
  noteLabel?: string;
  thenHelp?: string;
}

export interface RunAdjustRunner {
  runner: string;
  label: string;
  models: ModelChoice[];
  efforts: string[];
  thinkingNote?: string;
  emptyNote?: string;
}

export interface RunAdjustStageRequest {
  stage: string;
  model: string;
  thinking?: string;
  brief?: string;
  runner?: string;
}

export interface RunAdjustSwitchRequest {
  stage: string;
  runner: string;
  model?: string;
  thinking?: string;
}

export interface RunAdjustSwitchResponse {
  runner: string;
  model: string;
  thinking?: string;
}

export interface RunAfk {
  mode: string;
  label: string;
  on: boolean;
  next: string;
  help: string;
}

export interface RunAfkRequest {
  mode: string;
}

export interface RunAnswerRequest {
  answer?: string;
}

export interface RunAttemptRequest {
  attempt: string;
}

export interface RunBoxes {
  map: BoxMap;
  nodes: RunBoxesNode[];
  edges: RunBoxesEdge[];
  takenRoutes: string[];
  caption: string;
}

export interface RunBoxesChip {
  text: string;
  look: string;
}

export interface RunBoxesEdge {
  from: string;
  to: string;
  state: RunBoxesTrack;
  route?: string;
}

export interface RunBoxesNode {
  id: string;
  state: RunBoxesState;
  caption: string;
  title: string;
  details: string[];
  chip?: RunBoxesChip;
  dot?: RunBoxesDot;
}

export interface RunCost {
  nanos?: number;
  estimated: boolean;
  partial: boolean;
  endedAgents: number;
  liveAgents: boolean;
  line?: string;
  estimateExplanation: string;
}

export interface RunCounts {
  total: number;
  active: number;
  watching: number;
  running: number;
  needsYou: number;
  shippedToday: number;
}

export interface RunDecideRequest {
  outcome: string;
  note?: string;
  includeSubject?: boolean;
}

export interface RunEmptyRequest {
}

export interface RunLegRequest {
  leg?: string;
}

export interface RunLink {
  id: string;
  title: string;
  reference: string;
  detail: string;
  url: string;
}

export interface RunMenuRow {
  command: RunCommand;
  label: string;
  icon: string;
  danger: boolean;
  opens: RunMenuOpens;
  divided: boolean;
  stage?: string;
  targets?: RunMenuTarget[];
  action?: RunAction;
}

export interface RunMenuTarget {
  stage: string;
  name: string;
}

export interface RunOverruleAfkRequest {
  decision: string;
  stage: string;
  comments: string;
  acceptedWarning: boolean;
}

export interface RunPause {
  phase: RunPausePhase;
  operation: string;
  requestedAt: string;
  confirmedAt?: string;
  reason?: string;
  snapshots: RunPauseSnapshot[];
  deferred: RunPauseDeferred[];
}

export interface RunPauseInterval {
  start: string;
  end: string;
}

export interface RunPauseSnapshot {
  id: string;
  state: RunState;
  stageId?: string;
  reason?: string;
}

export interface RunPresentationAttemptEntry {
  id: string;
  number: number;
  caption: string;
  content: RunPresentationAttemptEntryContent;
  runningAttempt?: number;
  outcome?: string;
  tone?: Tone;
  route?: string;
  note?: string;
  detail?: string;
  isOpen: boolean;
}

export interface RunPresentationWatchNote {
  id: string;
  reference: string;
  note: string;
}

export interface RunResponse {
  run: RunSnapshot;
}

export interface RunResumeRequest {
  stage?: string;
  note?: string;
  earlier?: boolean;
  leg?: string;
}

export interface RunShadow {
  test: string;
  arm: ShadowArm;
}

export interface RunSource {
  entry: string;
  trigger: Trigger;
  repos: string[];
}

export interface RunStage {
  id: string;
  name: string;
  number: number;
  kind: StageKind;
  status: StageStatus;
  defaultAttempt?: string;
  attempts: RunPresentationAttemptEntry[];
  emptyMessage?: string;
  liveWatchSummary?: string;
  worker?: string;
  step: string;
  adjust?: RunStageAdjust;
  facts?: StagePanelFacts;
}

export interface RunStageAdjust {
  placeholders: PlaceholderHelp;
  note: string;
  agentBrief?: string;
  runner: string;
  model: string;
  thinking?: string;
  brief: string;
  runners: RunAdjustRunner[];
  repairNote?: string;
  freshSessionNote: string;
}

export interface RunTaskRequest {
  belt: string;
  entry?: string;
  repo: string;
  title: string;
  body: string;
}

export interface RunViewResponse {
  run: Run;
  boxes: RunBoxes;
  metadata: string;
  heading: string;
  reference: string;
  firstVerdict?: string;
  shadow?: string;
  pauseCopy?: string;
  erasureCopy: string;
  cost: RunCost;
  sessionCount: number;
  links: RunLink[];
  worktrees: RunWorktree[];
  worktreeAction: string;
  worktreeUnavailable: string;
  watchNotes: RunPresentationWatchNote[];
  stages: RunStage[];
  heldAnswer?: HeldAnswerCopy;
  legs: StatusLegRow[];
  actions: RunAction[];
  afk: RunAfk;
  active: boolean;
  quickActions: RunCommand[];
  menu: RunMenuRow[];
}

export interface RunWorktree {
  id: string;
  title: string;
  detail: string;
}

export interface RunsResponse {
  runs: RunSnapshot[];
}

export interface Schedule {
  days: ScheduleDays;
  weekday: number;
  hour: number;
  minute: number;
}

export interface ScheduledTask {
  schedule: Schedule;
  title: string;
  task: string;
}

export interface SessionOffer {
  enabled: boolean;
  reason?: string;
}

export interface SessionSettings {
  runner?: Backend;
  model: string;
  thinking?: Thinking;
}

export interface SessionSidebarProjectionRow {
  attemptId: string;
  stageId: string;
  title: string;
  caption: string;
  status: SessionSidebarProjectionStatus;
  session: string;
  selected: boolean;
}

export interface SessionSidebarProjectionSelection {
  stageId: string;
  attemptId?: string;
  fallback?: string;
}

export interface SessionUsage {
  tokens?: string;
  estimate?: string;
}

export interface SessionsView {
  runId: string;
  rows: SessionSidebarProjectionRow[];
  selection: SessionSidebarProjectionSelection;
  emptyMessage?: string;
  terminal?: SessionsViewTerminal;
  log?: string;
  usage?: SessionUsage;
  takeOver?: SessionOffer;
  ghostty: SessionOffer;
  continueSession?: SessionOffer;
}

export interface SessionsViewTerminal {
  target: string;
  readOnly: boolean;
  continued: boolean;
}

export interface SettingsResponse {
  login: string;
  notifications: boolean;
  libraryRepositories: LibraryRepository[];
  configurationBackend: Backend;
  configurationModel: string;
  configurationThinking: Thinking | null;
  paused: boolean;
  pausedAt: string | null;
  pauses: DateInterval[];
  allowedModels: Record<string, string[]>;
  modelCatalogs: Record<string, ModelCatalogSnapshot>;
  afk: AfkSettings;
  showInsights: boolean;
  dataLocation: string;
  demo: boolean;
}

export interface ShadowRunCounts {
  live: number;
  shadow: number;
}

export interface ShadowStageDifference {
  stageId: string;
  name: string;
  change: ShadowStageChange;
  label: string;
}

export interface ShadowSummary {
  test: string;
  version: number;
  live: number;
  startedAt: string;
  runs: ShadowRunCounts;
  draft: Belt;
  warnings: string[];
  differences: ShadowStageDifference[];
  policiesDiffer: boolean;
  afkDiffers: boolean;
  label: string;
  caption: string;
}

export interface Stage {
  id: string;
  type: StageType;
  name: string;
  agent?: AgentRef;
  key?: string;
  runner?: Backend;
  model: string;
  thinking?: Thinking;
  alternates?: SessionSettings[];
  workspace: Workspace;
  writes: string;
  slots: number;
  brief: string;
  outcomes: Outcome[];
  fallback: Fallback;
  route?: string;
  watch?: WatchTarget;
}

export interface StageLinePresentation {
  trigger: StageLinePresentationTrigger;
  items: StageLinePresentationItem[];
  addEnd: number;
  agents: StageLinePresentationAgentChoice[];
  watchAllowed: boolean;
}

export interface StageLinePresentationAgentChoice {
  id: string;
  version: number;
  name: string;
}

export interface StageLinePresentationBlock {
  id: string;
  kind: string;
  subtitle: string[];
  update?: number;
}

export interface StageLinePresentationForkBox {
  id: string;
  heading: string;
  feeding: boolean;
  routes: StageLinePresentationRouteRow[];
}

export interface StageLinePresentationItem {
  addAt: number;
  stage?: StageLinePresentationBlock;
  fork?: StageLinePresentationForkBox;
}

export interface StageLinePresentationRouteRow {
  id: string;
  caption: string;
  help: string;
  removable: boolean;
  items: StageLinePresentationItem[];
  addEnd: number;
  empty?: string;
}

export interface StageLinePresentationTrigger {
  kind: string;
  title: string;
  lines: string[];
  help?: string;
}

export interface StageMenu {
  stage: string;
  number: number;
  kind: string;
  canMoveEarlier: boolean;
  canMoveLater: boolean;
  placement?: StageMenuPlacement;
  workspaces: StageMenuOption[];
  watchTargets: StageMenuOption[];
  watchEvents: StageMenuOption[];
  watchNote?: string;
  actions: StageMenuOption[];
  routesAfter: StageMenuOption[];
  routePaths: StageMenuOption[];
  gotoTargets: StageMenuGotoTarget[];
  runners: StageMenuRunner[];
  defaultRunner: string;
  outcomesIntro: string;
  whenTitle: string;
  preview: StageMenuPreview;
  alternatesNote: string;
}

export interface StageMenuGotoRoute {
  route: StageMenuOption;
  stages: StageMenuOption[];
}

export interface StageMenuGotoTarget {
  id: string;
  label: string;
  counterparts: string[];
  others: StageMenuGotoRoute[];
}

export interface StageMenuOption {
  id: string;
  label: string;
}

export interface StageMenuPlacement {
  off: string;
  routes: StageMenuOption[];
}

export interface StageMenuPreview {
  title: string;
  lines: string[];
}

export interface StageMenuRunner {
  id: string;
  label: string;
  thinking: StageMenuOption[];
  thinkingNote?: string;
}

export interface StageModelStatus {
  id: string;
  label: string;
  reason: string;
}

export interface StagePanelAfkNote {
  title: string;
  overruleLine?: string;
  comments?: string;
  why?: string;
  deciding: boolean;
}

export interface StagePanelAttempt {
  id: string;
  ranOn: string;
  file?: string;
  duration?: string;
  decision?: string;
  decidedBy?: string;
  decidedAt?: string;
  decisionLine?: string;
  result?: StagePanelAttemptResult;
  afk?: StagePanelAfkNote;
}

export interface StagePanelAttemptResult {
  outcome?: string;
  tone?: string;
  route?: string;
  note?: string;
  detail?: string;
  problem: boolean;
}

export interface StagePanelChip {
  text: string;
  tone: string;
  clockSince?: string;
}

export interface StagePanelFacts {
  kindChip: string;
  working: string;
  state: StagePanelChip;
  canAdjust: boolean;
  attempts: StagePanelAttempt[];
  deciding?: StagePanelAfkNote;
  watchCards: StagePanelWatchCard[];
  watchLooking?: string;
  approval?: StagePanelFactsApproval;
  adjustments: StagePanelFactsAdjustment[];
  send: StageSendWords;
}

export interface StagePanelFactsAdjustment {
  date: string;
  text: string;
}

export interface StagePanelFactsApproval {
  blocks?: MarkdownBlock[];
  emptyDocument: string;
  willHeading?: string;
  will: InlineSpan[][];
  primary?: string;
  outcomesHelp: string;
  outcomes: StagePanelFactsOutcome[];
  prompt?: ApprovalPrompt;
}

export interface StagePanelFactsOutcome {
  id: string;
  name: string;
  label: string;
  tone: Tone;
}

export interface StagePanelWatchCard {
  id: string;
  title: string;
  subtitle: InlineSpan[];
  state: string;
  url: string;
}

export interface StageSendWords {
  earlier?: string;
  note?: string;
  again: string;
}

export interface StageStatus {
  text: string;
  look: StageStatusLook;
  paused: boolean;
}

export interface StateResponse {
  globallyPaused: boolean;
  checking: boolean;
  lastCheck: string | null;
  counts: RunCounts;
  notices: Notice[];
  settings: StateSettings;
  actions: GlobalAction[];
  navigation: NavigationSummary;
  clockLabel: string;
  clockTooltip: string;
}

export interface StateSettings {
  login: string;
  notifications: boolean;
  afkEnabled: boolean;
}

export interface StatusLegRow {
  id: string;
  route?: string;
  stage: string;
  state: RunState;
  look: StageStatusLook;
  paused: boolean;
}

export interface StatusWhere {
  text: string;
  look: StageStatusLook;
  glyph: string;
}

export interface Subject {
  kind: SubjectKind;
  repo: string;
  number: number;
  key: string;
  title: string;
  body: string;
  labels: string[];
  url: string;
  isOpen: boolean;
  assignees: string[];
  base?: string;
  occurrence?: string;
  reviewer?: string;
  parent?: string;
  blockers: string[];
}

export interface TaskDraftEntry {
  id: string;
  position: number;
  repos: string[];
}

export interface TaskDraftSelection {
  entry?: TaskDraftEntry;
  repo: string;
  canStart: boolean;
  problem?: string;
}

export interface ThinkingChoice {
  id: Thinking;
  label: string;
}

export interface TicketObservation {
  status: string;
  comments: string[];
  children: Record<string, TicketObservationSeen>;
  pendingSince?: string;
}

export interface TicketObservationSeen {
  status: string;
  isOpen: boolean;
}

export interface TokenTotals {
  input: number;
  cacheRead: number;
  output: number;
  reasoning: number;
  total: number;
  cacheWrite?: number;
}

export interface ToolsResponse {
  tools: Result[];
  tmuxSocket: string;
}

export interface TriggerEntry {
  id: string;
  trigger: Trigger;
  repos: string[];
}

export interface TriggerFormPresentation {
  kinds: TriggerFormPresentationKind[];
  pollChoices: TriggerFormPresentationChoice[];
  assignees: TriggerFormPresentationChoice[];
  reviewers: TriggerFormPresentationChoice[];
  scheduleDays: TriggerFormPresentationChoice[];
  weekdays: TriggerFormPresentationChoice[];
  hours: TriggerFormPresentationChoice[];
  minutes: TriggerFormPresentationChoice[];
  heading: string;
  empty: string;
  showsBase: boolean;
  repositoryBases?: TriggerFormRepositoryBases;
}

export interface TriggerFormPresentationChoice {
  value: string;
  label: string;
}

export interface TriggerFormPresentationKind {
  id: string;
  title: string;
  heading: string;
  polls: boolean;
  oneRepo: boolean;
  repositoriesTitle: string;
  note?: string;
}

export interface TriggerFormRepositoryBaseField {
  repository: string;
  label: string;
}

export interface TriggerFormRepositoryBases {
  heading: string;
  help: string;
  placeholder: string;
  fields: TriggerFormRepositoryBaseField[];
}

export interface TriggerStash {
  entry: string;
  triggers: Record<string, Trigger>;
}

export interface Usage {
  costNanos?: number;
  provenance?: CostProvenance;
  coverage: UsageCoverage;
  input?: number;
  output?: number;
  cacheRead?: number;
  cacheWrite?: number;
  reasoning?: number;
  totalTokens?: number;
  rate?: RateProvenance;
}

export interface UsageCheckpoint {
  sessionId?: string;
  lines?: number;
  totals?: TokenTotals;
  preexisting?: string[];
}

export interface WaitingResponse {
  waiting: WaitingSubject[];
}

export interface WaitingSubject {
  beltId: string;
  subject: Subject;
  reference: string;
  reason: string;
}

export interface WatchObservation {
  pullNumber: number;
  headSha?: string;
  arrival?: string;
  reported: WatchEvent[];
  behindSeen: boolean;
  approvals: string[];
  comments: string[];
  requesting: string[];
  reviewAuthors: string[];
  pendingHead?: string;
  approvalVisit?: string;
}

export interface WorkBelt {
  id: string;
  name: string;
  archived: boolean;
  label: string;
}

export interface WorkGroupPresentation {
  dimension: GroupBy;
  value: string;
  title: string;
  label: string;
  count: number;
  rows: WorkRowPresentation[];
}

export interface WorkResponse {
  summary: RunCounts;
  caption: string;
  stage?: WorkStage;
  belts: WorkBelt[];
  mainEmptyMessage?: string;
  mainGroups: WorkGroupPresentation[];
  watchingGroups: WorkGroupPresentation[];
  shadowGroups: WorkGroupPresentation[];
  shownRows: number;
  watchingRows: number;
  shadowRows: number;
  waiting: WaitingSubject[];
}

export interface WorkRowPresentation {
  run: WorkRowRun;
  openStage?: string;
  repository: string;
  progress: number;
  location: StatusWhere;
  pausedByYou: boolean;
  pauseWord?: string;
  chip?: string;
  needsYou: boolean;
  reference: string;
  updatedAgo: string;
  shadow?: string;
}

export interface WorkRowRun {
  id: string;
  beltId: string;
  beltName: string;
  stageId: string;
  state: RunState;
  issue: WorkRowRunIssue;
  updatedAt: string;
}

export interface WorkRowRunIssue {
  title: string;
}

export interface WorkStage {
  id: string;
  name: string;
}

export type EventMessage = { type: "hello"; apiVersion: number } | { type: "changed"; resource: EventResource; revision: number } | { type: "notification"; title: string; body: string };
export type EventResource = (typeof eventResourceValues)[number];
export type FactoryTrackNode = { kind: "entrance" | "stage"; value: string } | { kind: "waypoint"; value: number };

export type MarkdownBlock = { type: "heading"; level: number; text: string } | { type: "item"; marker: string; text: string } | { type: "code" | "paragraph"; text: string };

export type RouteSet = Record<string, string | string[]>;

export type RunPauseDeferred = { merged: { _0: string[] } } | { conflict: { _0: string[] } };

export type RunSnapshot = Run & { actions: RunAction[] };

export type Then = { do: ThenAction; to?: string | string[]; route?: string | string[]; withOutput?: boolean; max?: number; comment?: boolean };

export type Trigger = ({ kind: "issues" } & IssueFilter) | ({ kind: "reviews" } & ReviewFilter) | ({ kind: "jira" } & JiraQuery) | ({ kind: "schedule" } & ScheduledTask) | { kind: "manual" };

export type VersionResponse = { apiVersion: number; platform: "macos" | "linux"; version: string };