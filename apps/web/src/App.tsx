import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import {
  ApiError,
  AuditEventFilters,
  AskConversationResponse,
  ConnectorConfig,
  CurrentUserResponse,
  DataSourceDetailResponse,
  DocumentDetailResponse,
  Health,
  ListConversationMessagesResponse,
  ListConversationsResponse,
  ListAuditEventsResponse,
  ListDataSourcesResponse,
  ListDocumentsResponse,
  ListJobsResponse,
  ListTenantMembersResponse,
  ModelTarget,
  Readiness,
  RegisterDocumentResponse,
  RetrievalStrategy,
  SearchDocumentsResponse,
  SourcePolicyProfile as PersistedSourcePolicyProfile,
  SourceView,
  addTenantMember,
  archiveDataSource,
  askConversationStream,
  apiBase,
  cancelDataSourceScan,
  checkModelTarget,
  createDataSource,
  createSourcePolicyProfile,
  createSourceView,
  createTenant,
  dataSourceScanEntriesExportUrl,
  deleteConversation,
  deleteDocument,
  deleteSourceView,
  deleteSourcePolicyProfile,
  deleteTenant,
  deleteTenantMember,
  downloadDocument,
  getDataSource,
  getDocument,
  getCurrentUser,
  getHealth,
  getModelTargets,
  getReadiness,
  listDataSources,
  listConversationMessages,
  listConversations,
  listAuditEvents,
  listDocuments,
  listJobs,
  listSourceViews,
  listSourcePolicyProfiles,
  listTenantMembers,
  planDataSource,
  preflightDataSource,
  reindexDataSource,
  retryFailedDataSourceDocuments,
  retryDocument,
  scanDataSource,
  searchDocuments,
  shutdownRuntime,
  updateSourcePolicyProfile,
  updateSourceView,
  updateDataSource,
  uploadDocument,
} from './api';

type View = 'ask' | 'documents' | 'search' | 'activity' | 'history' | 'status' | 'settings';

type AskPhase = 'idle' | 'connecting' | 'retrieving' | 'generating' | 'streaming' | 'complete' | 'failed';

type AskProgressHint = {
  detail: string;
  label: string;
  tone: 'slow' | 'risk';
};

const askPhaseSteps: Array<{ phase: AskPhase; label: string }> = [
  { phase: 'connecting', label: 'Connect' },
  { phase: 'retrieving', label: 'Context' },
  { phase: 'generating', label: 'Model' },
  { phase: 'streaming', label: 'Answer' },
];

type ScanEntryFilter = 'all' | 'imported' | 'skipped' | 'failed' | 'deleted';
type SourceFilterValues = {
  health: string;
  query: string;
  schedule: string;
  type: string;
};
type SourceBulkAction = 'preflight' | 'plan' | 'scan' | 'retry_failures' | 'reindex';
type SourceSavedView = SourceView;
type SourcePlanSampleFilter = 'all' | 'would_import' | 'skipped' | 'failed';

type ScanMetric = {
  key: ScanEntryFilter;
  label: string;
  count: number;
  filter: ScanEntryFilter;
};

type SourcePlanSummary = {
  total_entries: number;
  files_seen: number;
  would_import: number;
  skipped: number;
  failed: number;
  estimated_bytes: number;
  reasons: Record<string, number>;
  samples?: SourcePlanSample[];
};

type SourcePlanSample = {
  path: string;
  outcome: string;
  reason?: string;
  size_bytes?: number;
  message?: string;
};

type SourceScanRunSummary = {
  job_id: string;
  source_id: string;
  started_at: string;
  finished_at: string;
  duration_ms: number;
  imported: number;
  skipped: number;
  deleted: number;
  skipped_unsupported: number;
  skipped_policy: number;
  skipped_too_large: number;
  failed: number;
};

type SourceHealthState = 'ready' | 'active' | 'review' | 'blocked' | 'neutral';

type SourceHealthItem = {
  key: string;
  label: string;
  value: string;
  detail: string;
  state: SourceHealthState;
};

type TargetCheckState = {
  state: 'ok' | 'loading' | 'failed';
  detail: string;
  checked_at?: string;
  error_class?: string;
  latency_ms?: number;
  model?: string;
  retryable?: boolean;
};

type ModelPerformanceRow = {
  target: ModelTarget;
  check?: TargetCheckState;
  askCount: number;
  failureCount: number;
  latestLatencyMS: number | null;
  p95LatencyMS: number | null;
  successRate: number | null;
  lastError: string;
  lastErrorClass: string;
  lastEventAt: string;
};

type SetupCheckStatus = 'checking' | 'ok' | 'warning' | 'blocked';

type SetupCheck = {
  id: string;
  label: string;
  detail: string;
  status: SetupCheckStatus;
  blocking: boolean;
};

type SourceFormValues = {
  type: string;
  name: string;
  root_path: string;
  connector_provider: string;
  connector_resource_id: string;
  connector_credential_ref: string;
  connector_notes: string;
  include_patterns: string;
  exclude_patterns: string;
  scan_interval_minutes: string;
};

type SourceTemplate = Omit<
  SourceFormValues,
  'connector_provider' | 'connector_resource_id' | 'connector_credential_ref' | 'connector_notes'
> &
  Partial<
    Pick<
      SourceFormValues,
      'connector_provider' | 'connector_resource_id' | 'connector_credential_ref' | 'connector_notes'
    >
  > & {
  id: string;
  label: string;
  detail: string;
};

type SourcePolicyProfile = Pick<
  SourceFormValues,
  'include_patterns' | 'exclude_patterns' | 'scan_interval_minutes'
> & {
  id: string;
  label: string;
  detail: string;
  persisted?: boolean;
};

const setupWizardStorageKey = 'nexus-local.setupWizardAcknowledged';
const setupAutoRecheckDelayMs = 15000;
const scanEntryPageSize = 100;
const sourcePlanSamplePageSize = 12;
const sourcePlanLargeImportFileThreshold = 500;
const sourcePlanLargeImportBytesThreshold = 500 * 1024 * 1024;
const sourcePlanSkippedFileThreshold = 100;
const sourcePlanSkippedRatioThreshold = 0.5;
const retrievalStrategies: Array<{ value: RetrievalStrategy; label: string }> = [
  { value: 'hybrid', label: 'Hybrid' },
  { value: 'vector', label: 'Vector only' },
];

const initialAsk = {
  conversation_id: '',
  document_id: '',
  model_target: 'general',
  question: '',
  limit: 5,
  strategy: 'hybrid' as RetrievalStrategy,
};

const initialMemberForm = {
  user_id: '',
  email: '',
  name: '',
  role: 'member',
};

const initialAuditFilters = {
  action: '',
  actor_user_id: '',
  from: '',
  outcome: '',
  query: '',
  to: '',
};

const initialSourceFilters: SourceFilterValues = {
  health: '',
  query: '',
  schedule: '',
  type: '',
};

const sourcePresetViews: Array<{ id: string; label: string; filters: SourceFilterValues }> = [
  { id: 'all', label: 'All', filters: initialSourceFilters },
  { id: 'review', label: 'Needs review', filters: { ...initialSourceFilters, health: 'review' } },
  { id: 'blocked', label: 'Blocked', filters: { ...initialSourceFilters, health: 'blocked' } },
  { id: 'overdue', label: 'Overdue', filters: { ...initialSourceFilters, schedule: 'overdue' } },
  { id: 'scheduled', label: 'Scheduled', filters: { ...initialSourceFilters, schedule: 'scheduled' } },
];

const initialSourceForm: SourceFormValues = {
  type: 'synced_folder',
  name: '',
  root_path: '',
  connector_provider: '',
  connector_resource_id: '',
  connector_credential_ref: '',
  connector_notes: '',
  include_patterns: '',
  exclude_patterns: '',
  scan_interval_minutes: '0',
};

const broadDocumentIncludes = [
  '**/*.txt',
  '**/*.md',
  '**/*.json',
  '**/*.html',
  '**/*.htm',
  '**/*.csv',
  '**/*.tsv',
  '**/*.vtt',
  '**/*.pdf',
  '**/*.docx',
  '**/*.pptx',
  '**/*.xlsx',
].join('\n');

const commonDocumentExcludes = [
  '**/~$*',
  '**/.DS_Store',
  '**/archive/**',
  '**/Archive/**',
  '**/backup/**',
  '**/Backup/**',
].join('\n');

const sourceTemplates: SourceTemplate[] = [
  {
    id: 'sharepoint-connector',
    label: 'SharePoint connector',
    detail: 'Future direct connector handoff',
    type: 'connector',
    name: 'SharePoint Connector',
    root_path: 'connector://sharepoint/site',
    connector_provider: 'sharepoint',
    connector_resource_id: 'tenant/site/library',
    connector_credential_ref: 'secret/sharepoint/read-only',
    connector_notes: 'Store OAuth/client details in the deployment secret manager.',
    include_patterns: broadDocumentIncludes,
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '0',
  },
  {
    id: 'onedrive-connector',
    label: 'OneDrive connector',
    detail: 'Future direct drive connector',
    type: 'connector',
    name: 'OneDrive Connector',
    root_path: 'connector://onedrive/drive',
    connector_provider: 'onedrive',
    connector_resource_id: 'user-or-group/drive',
    connector_credential_ref: 'secret/onedrive/read-only',
    connector_notes: 'Reference customer-owned credentials; do not paste secrets here.',
    include_patterns: broadDocumentIncludes,
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '0',
  },
  {
    id: 'ticket-system-connector',
    label: 'Ticket connector',
    detail: 'Future case export connector',
    type: 'connector',
    name: 'Ticket Connector',
    root_path: 'connector://tickets/project',
    connector_provider: 'ticketing',
    connector_resource_id: 'workspace/project',
    connector_credential_ref: 'secret/tickets/read-only',
    connector_notes: 'Use exported files until direct connector workers are implemented.',
    include_patterns: ['**/*.json', '**/*.csv', '**/*.html', '**/*.htm', '**/*.txt', '**/*.md'].join(
      '\n',
    ),
    exclude_patterns: ['**/attachments/**', '**/raw/**', '**/tmp/**', '**/temp/**'].join('\n'),
    scan_interval_minutes: '0',
  },
  {
    id: 'sharepoint-sync',
    label: 'SharePoint sync',
    detail: 'Teams and SharePoint folders',
    type: 'synced_folder',
    name: 'SharePoint Docs',
    root_path: '/sources/primary/SharePoint',
    include_patterns: broadDocumentIncludes,
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '1440',
  },
  {
    id: 'onedrive-sync',
    label: 'OneDrive sync',
    detail: 'User or department drive',
    type: 'synced_folder',
    name: 'OneDrive Docs',
    root_path: '/sources/primary/OneDrive',
    include_patterns: broadDocumentIncludes,
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '1440',
  },
  {
    id: 'network-share',
    label: 'Network share',
    detail: 'SMB or NFS mounted host path',
    type: 'network_share',
    name: 'Shared Drive',
    root_path: '/sources/primary',
    include_patterns: broadDocumentIncludes,
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '1440',
  },
  {
    id: 'ticket-export',
    label: 'Ticket export',
    detail: 'Cases, HAR files, CSV, JSON',
    type: 'export',
    name: 'Ticket Export',
    root_path: '/sources/primary/exports',
    include_patterns: ['**/*.json', '**/*.csv', '**/*.html', '**/*.htm', '**/*.txt', '**/*.md'].join(
      '\n',
    ),
    exclude_patterns: ['**/attachments/**', '**/raw/**', '**/tmp/**', '**/temp/**'].join('\n'),
    scan_interval_minutes: '0',
  },
  {
    id: 'knowledge-base-export',
    label: 'Knowledge base',
    detail: 'Docs portal or help center export',
    type: 'export',
    name: 'Knowledge Base Export',
    root_path: '/sources/primary/kb-export',
    include_patterns: ['**/*.html', '**/*.htm', '**/*.md', '**/*.pdf', '**/*.docx'].join('\n'),
    exclude_patterns: ['**/assets/**', '**/images/**', '**/static/**'].join('\n'),
    scan_interval_minutes: '0',
  },
  {
    id: 'runbooks',
    label: 'Runbooks',
    detail: 'Procedures and internal notes',
    type: 'synced_folder',
    name: 'Runbooks',
    root_path: '/sources/primary/runbooks',
    include_patterns: ['**/*.md', '**/*.txt', '**/*.pdf', '**/*.docx', '**/*.csv', '**/*.json'].join(
      '\n',
    ),
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '10080',
  },
];

const builtinSourcePolicyProfiles: SourcePolicyProfile[] = [
  {
    id: 'general-docs',
    label: 'General docs',
    detail: 'Office files, PDFs, notes, tables, transcripts',
    include_patterns: broadDocumentIncludes,
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '1440',
  },
  {
    id: 'support-exports',
    label: 'Support exports',
    detail: 'Cases, logs, CSV, HTML, JSON exports',
    include_patterns: ['**/*.json', '**/*.csv', '**/*.tsv', '**/*.html', '**/*.htm', '**/*.txt', '**/*.md'].join(
      '\n',
    ),
    exclude_patterns: [
      '**/attachments/**',
      '**/raw/**',
      '**/tmp/**',
      '**/temp/**',
      '**/node_modules/**',
    ].join('\n'),
    scan_interval_minutes: '0',
  },
  {
    id: 'knowledge-base',
    label: 'Knowledge base',
    detail: 'Docs portals, help centers, runbooks',
    include_patterns: ['**/*.html', '**/*.htm', '**/*.md', '**/*.txt', '**/*.pdf', '**/*.docx'].join(
      '\n',
    ),
    exclude_patterns: ['**/assets/**', '**/images/**', '**/static/**', '**/archive/**'].join('\n'),
    scan_interval_minutes: '1440',
  },
  {
    id: 'runbooks',
    label: 'Runbooks',
    detail: 'Procedures with slower weekly refresh',
    include_patterns: ['**/*.md', '**/*.txt', '**/*.pdf', '**/*.docx', '**/*.csv', '**/*.json'].join(
      '\n',
    ),
    exclude_patterns: commonDocumentExcludes,
    scan_interval_minutes: '10080',
  },
  {
    id: 'governance-records',
    label: 'Governance',
    detail: 'Policy, audit, spreadsheet, and PDF records',
    include_patterns: ['**/*.pdf', '**/*.docx', '**/*.xlsx', '**/*.csv', '**/*.md', '**/*.txt'].join(
      '\n',
    ),
    exclude_patterns: ['**/draft/**', '**/Draft/**', '**/archive/**', '**/Archive/**', '**/~$*'].join(
      '\n',
    ),
    scan_interval_minutes: '1440',
  },
];

const supportedDocumentExtensions = [
  '.txt',
  '.md',
  '.json',
  '.html',
  '.htm',
  '.csv',
  '.tsv',
  '.vtt',
  '.pdf',
  '.docx',
  '.pptx',
  '.xlsx',
];

const supportedDocumentAccept = supportedDocumentExtensions.join(',');
const folderImportConcurrency = 3;

const sampleDocumentContent = `# Nexus Local Sample

Nexus Local can ingest documents, index their text, search retrieved chunks, and answer questions with cited sources.

Sample phrase: cedar signal atlas.

Try searching for cedar signal atlas, then ask what phrase appears in the sample document.
`;
const sampleDocumentName = 'nexus-local-sample.md';
const sampleSearchPhrase = 'cedar signal atlas';
const sampleQuestion = 'What phrase appears in the sample document?';
const sampleDocumentWaitMs = 90000;

type SampleFlowState = 'idle' | 'creating' | 'uploading' | 'indexing' | 'ready' | 'failed';

export function App() {
  const [activeView, setActiveView] = useState<View>('ask');
  const [health, setHealth] = useState<Health | null>(null);
  const [readiness, setReadiness] = useState<Readiness | null>(null);
  const [currentUser, setCurrentUser] = useState<CurrentUserResponse | null>(null);
  const [targets, setTargets] = useState<ModelTarget[]>([]);
  const [targetChecks, setTargetChecks] = useState<Record<string, TargetCheckState>>({});
  const [setupGatewayCheck, setSetupGatewayCheck] = useState<TargetCheckState | null>(null);
  const [setupChecking, setSetupChecking] = useState(false);
  const [setupLastChecked, setSetupLastChecked] = useState('');
  const [setupAutoRecheckAt, setSetupAutoRecheckAt] = useState<number | null>(null);
  const [setupAutoRecheckTick, setSetupAutoRecheckTick] = useState(Date.now());
  const [setupAcknowledgedKey, setSetupAcknowledgedKey] = useState(
    () => window.localStorage.getItem(setupWizardStorageKey) ?? '',
  );
  const [tenantID, setTenantID] = useState('');
  const [tenantName, setTenantName] = useState('Personal Workspace');
  const [file, setFile] = useState<File | null>(null);
  const folderInputRef = useRef<HTMLInputElement | null>(null);
  const [folderFiles, setFolderFiles] = useState<File[]>([]);
  const [folderImporting, setFolderImporting] = useState(false);
  const [folderImportProgress, setFolderImportProgress] = useState('');
  const [registration, setRegistration] = useState<RegisterDocumentResponse | null>(null);
  const [documents, setDocuments] = useState<ListDocumentsResponse | null>(null);
  const [dataSources, setDataSources] = useState<ListDataSourcesResponse | null>(null);
  const [sourceDetail, setSourceDetail] = useState<DataSourceDetailResponse | null>(null);
  const [scanEntryFilter, setScanEntryFilter] = useState<ScanEntryFilter>('all');
  const [scanEntryOffset, setScanEntryOffset] = useState(0);
  const [documentDetail, setDocumentDetail] = useState<DocumentDetailResponse | null>(null);
  const [jobs, setJobs] = useState<ListJobsResponse | null>(null);
  const [auditEvents, setAuditEvents] = useState<ListAuditEventsResponse | null>(null);
  const [modelAuditEvents, setModelAuditEvents] = useState<ListAuditEventsResponse | null>(null);
  const [auditFilters, setAuditFilters] = useState(initialAuditFilters);
  const [tenantMembers, setTenantMembers] = useState<ListTenantMembersResponse | null>(null);
  const [conversations, setConversations] = useState<ListConversationsResponse | null>(null);
  const [conversationMessages, setConversationMessages] =
    useState<ListConversationMessagesResponse | null>(null);
  const [selectedConversationID, setSelectedConversationID] = useState('');
  const [searchForm, setSearchForm] = useState({
    document_id: '',
    query: '',
    limit: 5,
    strategy: 'hybrid' as RetrievalStrategy,
  });
  const [sourceForm, setSourceForm] = useState(initialSourceForm);
  const [contextSourceOpen, setContextSourceOpen] = useState(false);
  const [sourceConnectNotice, setSourceConnectNotice] = useState('');
  const [sourceEditForm, setSourceEditForm] = useState(initialSourceForm);
  const [sourceFilters, setSourceFilters] = useState(initialSourceFilters);
  const [sourceSavedViews, setSourceSavedViews] = useState<SourceSavedView[]>([]);
  const [customSourcePolicyProfiles, setCustomSourcePolicyProfiles] = useState<SourcePolicyProfile[]>([]);
  const [sourcePolicyName, setSourcePolicyName] = useState('');
  const [sourcePolicyDetail, setSourcePolicyDetail] = useState('');
  const [sourcePolicyNotice, setSourcePolicyNotice] = useState('');
  const [savingSourcePolicyProfile, setSavingSourcePolicyProfile] = useState(false);
  const [deletingSourcePolicyProfileID, setDeletingSourcePolicyProfileID] = useState('');
  const [sourceViewName, setSourceViewName] = useState('');
  const [sourceViewNotice, setSourceViewNotice] = useState('');
  const [savingSourceView, setSavingSourceView] = useState(false);
  const [deletingSourceViewID, setDeletingSourceViewID] = useState('');
  const [sourceBulkAction, setSourceBulkAction] = useState<SourceBulkAction | ''>('');
  const [sourceBulkResult, setSourceBulkResult] = useState('');
  const [searchResult, setSearchResult] = useState<SearchDocumentsResponse | null>(null);
  const [selectedSearchSourceKey, setSelectedSearchSourceKey] = useState('');
  const [memberForm, setMemberForm] = useState(initialMemberForm);
  const [askForm, setAskForm] = useState(initialAsk);
  const [askResult, setAskResult] = useState<AskConversationResponse | null>(null);
  const [selectedAskSourceKey, setSelectedAskSourceKey] = useState('');
  const [streamAnswer, setStreamAnswer] = useState('');
  const [streamStatus, setStreamStatus] = useState('');
  const [askPhase, setAskPhase] = useState<AskPhase>('idle');
  const [askStartedAt, setAskStartedAt] = useState<number | null>(null);
  const [askElapsedSeconds, setAskElapsedSeconds] = useState(0);
  const askAbortRef = useRef<AbortController | null>(null);
  const [currentAskQuestion, setCurrentAskQuestion] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [creatingSource, setCreatingSource] = useState(false);
  const [archivingSourceID, setArchivingSourceID] = useState('');
  const [deletingSourceDocumentsID, setDeletingSourceDocumentsID] = useState('');
  const [preflightingSourceID, setPreflightingSourceID] = useState('');
  const [planningSourceID, setPlanningSourceID] = useState('');
  const [scanningSourceID, setScanningSourceID] = useState('');
  const [cancelingSourceScanID, setCancelingSourceScanID] = useState('');
  const [reindexingSourceID, setReindexingSourceID] = useState('');
  const [retryingSourceFailuresID, setRetryingSourceFailuresID] = useState('');
  const [uploadingSample, setUploadingSample] = useState(false);
  const [sampleFlowState, setSampleFlowState] = useState<SampleFlowState>('idle');
  const [sampleFlowDocumentID, setSampleFlowDocumentID] = useState('');
  const [creatingTenant, setCreatingTenant] = useState(false);
  const [savingMember, setSavingMember] = useState(false);
  const [removingMemberID, setRemovingMemberID] = useState('');
  const [deletingTenantID, setDeletingTenantID] = useState('');
  const [deletingDocumentID, setDeletingDocumentID] = useState('');
  const [downloadingDocumentID, setDownloadingDocumentID] = useState('');
  const [loadingSourceID, setLoadingSourceID] = useState('');
  const [refreshingSourceID, setRefreshingSourceID] = useState('');
  const [savingSourceID, setSavingSourceID] = useState('');
  const [loadingDocumentID, setLoadingDocumentID] = useState('');
  const [retryingDocumentID, setRetryingDocumentID] = useState('');
  const [deletingConversationID, setDeletingConversationID] = useState('');
  const [searching, setSearching] = useState(false);
  const [asking, setAsking] = useState(false);
  const [checkingTarget, setCheckingTarget] = useState('');
  const [loadingAudit, setLoadingAudit] = useState(false);
  const [ingestionSyncing, setIngestionSyncing] = useState(false);
  const [lastIngestionSync, setLastIngestionSync] = useState('');
  const [shuttingDown, setShuttingDown] = useState(false);
  const [shutdownNotice, setShutdownNotice] = useState('');

  const selectedTenant = useMemo(
    () => currentUser?.memberships.find((membership) => membership.tenant.id === tenantID),
    [currentUser, tenantID],
  );
  const workspaceReady = tenantID !== '';
  const needsWorkspace = currentUser !== null && currentUser.memberships.length === 0;
  const workspaceLabel = selectedTenant?.tenant.name ?? (tenantID || 'No workspace');
  const canManageTenant = selectedTenant?.role === 'owner' || selectedTenant?.role === 'admin';
  const canManageMembers = canManageTenant;
  const canWriteKnowledge = roleCanWriteKnowledge(selectedTenant?.role);
  const canUploadDocuments = canWriteKnowledge;
  const canManageSources = canWriteKnowledge;
  const canImportSources = canWriteKnowledge;
  const ownerCount = tenantMembers?.members.filter((member) => member.role === 'owner').length ?? 0;
  const activeJobs = useMemo(
    () => jobs?.jobs.filter((job) => isActiveJobState(job.state)) ?? [],
    [jobs],
  );
  const activeIngestionJobs = useMemo(
    () =>
      activeJobs.filter(
        (job) => job.type === 'document_ingestion' && job.resource_type === 'document',
      ),
    [activeJobs],
  );
  const activeIngestionDocumentIDs = useMemo(
    () => new Set(activeIngestionJobs.map((job) => job.resource_id)),
    [activeIngestionJobs],
  );
  const activeSourceScanJobs = useMemo(
    () =>
      new Map(
        activeJobs
          .filter((job) => job.type === 'source_scan' && job.resource_type === 'data_source')
          .map((job) => [job.resource_id, job]),
      ),
    [activeJobs],
  );
  const activeSourcePreflightJobs = useMemo(
    () =>
      new Map(
        activeJobs
          .filter((job) => job.type === 'source_preflight' && job.resource_type === 'data_source')
          .map((job) => [job.resource_id, job]),
      ),
    [activeJobs],
  );
  const activeSourcePlanJobs = useMemo(
    () =>
      new Map(
        activeJobs
          .filter((job) => job.type === 'source_plan' && job.resource_type === 'data_source')
          .map((job) => [job.resource_id, job]),
      ),
    [activeJobs],
  );
  const latestSourcePlanJobs = useMemo(() => {
    const latest = new Map<string, ListJobsResponse['jobs'][number]>();
    for (const job of jobs?.jobs ?? []) {
      if (job.type !== 'source_plan' || job.resource_type !== 'data_source') {
        continue;
      }
      const current = latest.get(job.resource_id);
      if (!current || Date.parse(job.updated_at) > Date.parse(current.updated_at)) {
        latest.set(job.resource_id, job);
      }
    }
    return latest;
  }, [jobs]);
  const latestSourcePreflightJobs = useMemo(() => {
    const latest = new Map<string, ListJobsResponse['jobs'][number]>();
    for (const job of jobs?.jobs ?? []) {
      if (job.type !== 'source_preflight' || job.resource_type !== 'data_source') {
        continue;
      }
      const current = latest.get(job.resource_id);
      if (!current || Date.parse(job.updated_at) > Date.parse(current.updated_at)) {
        latest.set(job.resource_id, job);
      }
    }
    return latest;
  }, [jobs]);
  const filteredSources = useMemo(
    () =>
      filterDataSources(
        dataSources?.sources ?? [],
        sourceFilters,
        activeSourcePreflightJobs,
        activeSourcePlanJobs,
        activeSourceScanJobs,
        latestSourcePreflightJobs,
        latestSourcePlanJobs,
      ),
    [
      activeSourcePlanJobs,
      activeSourcePreflightJobs,
      activeSourceScanJobs,
      dataSources,
      latestSourcePlanJobs,
      latestSourcePreflightJobs,
      sourceFilters,
    ],
  );
  const sourceFiltersActive = sourceFilterSetIsActive(sourceFilters);
  const activeSourceSavedView = useMemo(
    () => sourceSavedViews.find((view) => sourceFiltersEqual(view.filters, sourceFilters)),
    [sourceSavedViews, sourceFilters],
  );
  const sourcePolicyProfiles = useMemo(
    () => [...builtinSourcePolicyProfiles, ...customSourcePolicyProfiles],
    [customSourcePolicyProfiles],
  );
  const sourceContainerRoot = sourceRootPath(readiness);
  const quickSourceTemplates = useMemo(
    () => dashboardSourceTemplates(sourceContainerRoot),
    [sourceContainerRoot],
  );
  const sourceBulkActionCounts = useMemo(
    () =>
      sourceBulkEligibilityCounts(
        filteredSources,
        activeSourcePreflightJobs,
        activeSourcePlanJobs,
        activeSourceScanJobs,
        latestSourcePreflightJobs,
        latestSourcePlanJobs,
      ),
    [
      activeSourcePlanJobs,
      activeSourcePreflightJobs,
      activeSourceScanJobs,
      filteredSources,
      latestSourcePlanJobs,
      latestSourcePreflightJobs,
    ],
  );
  const visibleSourceScanEntries = useMemo(
    () => sourceDetail?.scan_entries ?? [],
    [sourceDetail],
  );
  const failedJobs = useMemo(
    () => jobs?.jobs.filter((job) => job.state === 'failed') ?? [],
    [jobs],
  );
  const failedDocuments = useMemo(
    () => documents?.documents.filter((document) => document.status === 'failed') ?? [],
    [documents],
  );
  const activeDocuments = useMemo(
    () => documents?.documents.filter((document) => isActiveDocumentStatus(document.status)) ?? [],
    [documents],
  );
  const documentCount = documents?.documents.length ?? 0;
  const readyDocumentCount =
    documents?.documents.filter((document) => document.status === 'ready').length ?? 0;
  const sampleDocument = useMemo(
    () =>
      documents?.documents.find((document) => document.id === sampleFlowDocumentID) ??
      documents?.documents.find((document) => document.name === sampleDocumentName) ??
      null,
    [documents, sampleFlowDocumentID],
  );
  const sampleSearchReady = Boolean(
    searchResult &&
      sampleDocument &&
      searchForm.document_id === sampleDocument.id &&
      searchForm.query === sampleSearchPhrase,
  );
  const processingDocumentCount = activeDocuments.length;
  const failedDocumentCount = failedDocuments.length;
  const activeJobCount = activeJobs.length;
  const sampleFlowActive =
    sampleFlowState !== 'idle' && sampleFlowState !== 'ready' && sampleFlowState !== 'failed';
  const activeSourceScanCount = activeSourceScanJobs.size;
  const activeSourcePreflightCount = activeSourcePreflightJobs.size;
  const activeSourcePlanCount = activeSourcePlanJobs.size;
  const trackedSourceID = sourceDetail?.source.id ?? '';
  const trackedDocumentID = documentDetail?.document.id ?? registration?.document.id ?? '';
  const trackingIngestion = useMemo(
    () =>
      activeIngestionJobs.length > 0 ||
      activeSourceScanCount > 0 ||
      activeSourcePreflightCount > 0 ||
      activeSourcePlanCount > 0 ||
      Boolean(documentDetail && isActiveDocumentStatus(documentDetail.document.status)) ||
      Boolean(documentDetail && hasActiveIngestionJob(documentDetail)) ||
      Boolean(
        registration &&
          (isActiveDocumentStatus(registration.document.status) ||
            isActiveJobState(registration.job.state)),
      ),
    [
      activeIngestionJobs.length,
      activeSourcePlanCount,
      activeSourcePreflightCount,
      activeSourceScanCount,
      documentDetail,
      registration,
    ],
  );
  const ingestionLabel = ingestionSyncing
    ? 'Updating'
    : trackingIngestion
      ? activeIngestionJobs.length > 0
        ? `${activeIngestionJobs.length} active`
        : activeSourceScanCount > 0
          ? `${activeSourceScanCount} scan`
          : activeSourcePreflightCount > 0
            ? `${activeSourcePreflightCount} check`
            : activeSourcePlanCount > 0
              ? `${activeSourcePlanCount} plan`
        : 'Tracking'
      : lastIngestionSync
        ? `Synced ${formatTimeOnly(lastIngestionSync)}`
        : 'Idle';
  const freshWorkspace = workspaceReady && documentCount === 0 && !trackingIngestion;
  const rawAskAnswer = askResult?.assistant_message.content || streamAnswer;
  const visibleAskAnswer = askPhase === 'failed' ? '' : rawAskAnswer;
  const visibleAskTitle =
    askResult?.conversation.title ||
    askResult?.conversation.id ||
    currentAskQuestion ||
    streamStatus ||
    'Working';
  const visibleAskModel = askResult?.completion.model || askForm.model_target;
  const showAskProgress = asking || askPhase === 'failed';
  const showAskResult = Boolean(askResult || visibleAskAnswer || showAskProgress || streamStatus);
  const askRetryAvailable = askPhase === 'failed' && Boolean(currentAskQuestion) && workspaceReady;
  const selectedAskSource = useMemo(
    () => selectedResultHit(askResult?.hits ?? [], selectedAskSourceKey),
    [askResult, selectedAskSourceKey],
  );
  const selectedSearchSource = useMemo(
    () => selectedResultHit(searchResult?.hits ?? [], selectedSearchSourceKey),
    [searchResult, selectedSearchSourceKey],
  );
  const askProgressDetail = askProgressDetailLabel({
    answer: visibleAskAnswer,
    elapsedSeconds: askElapsedSeconds,
    phase: askPhase,
    status: streamStatus,
  });
  const askProgressHint = askProgressHintLabel({
    answer: visibleAskAnswer,
    asking,
    elapsedSeconds: askElapsedSeconds,
    phase: askPhase,
  });
  const auditActionOptions = useMemo(
    () => uniqueSorted(auditEvents?.events.map((event) => event.action) ?? []),
    [auditEvents],
  );
  const auditOutcomeOptions = useMemo(
    () => uniqueSorted(auditEvents?.events.map((event) => event.outcome) ?? []),
    [auditEvents],
  );
  const auditActorOptions = useMemo(
    () => uniqueSorted(auditEvents?.events.map((event) => event.actor_user_id) ?? []),
    [auditEvents],
  );
  const filteredAuditEvents = useMemo(
    () => filterAuditEvents(auditEvents?.events ?? [], auditFilters),
    [auditEvents, auditFilters],
  );
  const modelPerformanceRows = useMemo(
    () => buildModelPerformanceRows(targets, targetChecks, modelAuditEvents?.events ?? []),
    [targets, targetChecks, modelAuditEvents],
  );
  const modelPerformanceSummary = useMemo(
    () => summarizeModelPerformance(modelPerformanceRows),
    [modelPerformanceRows],
  );
  const auditFiltersActive = Boolean(
    auditFilters.action ||
      auditFilters.outcome ||
      auditFilters.actor_user_id.trim() ||
      auditFilters.from ||
      auditFilters.to ||
      auditFilters.query.trim(),
  );
  const contentTitle =
    activeView === 'ask'
      ? 'Dashboard'
      : activeView === 'documents' || activeView === 'search'
        ? 'Library'
        : titleCase(activeView);
  const setupPrimaryTarget = targets.find((target) => target.name === 'general') ?? targets[0];
  const setupFingerprint = useMemo(
    () =>
      [
        apiBase(),
        readiness?.model_gateway ?? '',
        readiness?.provider_preset ?? '',
        targets
          .map((target) => `${target.name}:${target.provider}:${target.base_url}:${target.model}`)
          .join('|'),
      ].join('::'),
    [readiness, targets],
  );
  const setupChecks = useMemo(
    () =>
      buildSetupChecks({
        health,
        readiness,
        targets,
        gatewayCheck: setupGatewayCheck,
        checking: setupChecking,
      }),
    [health, readiness, setupChecking, setupGatewayCheck, targets],
  );
  const setupCanEnter = setupChecks.every(
    (check) => !check.blocking || check.status === 'ok',
  );
  const showSetupWizard = !setupCanEnter || setupAcknowledgedKey !== setupFingerprint;
  const gatewaySetupCheck = setupChecks.find((check) => check.id === 'gateway');
  const setupGatewayOnlyPending =
    (gatewaySetupCheck?.status === 'blocked' || gatewaySetupCheck?.status === 'checking') &&
    setupChecks.every(
      (check) => check.id === 'gateway' || !check.blocking || check.status === 'ok',
    );
  const setupShouldAutoRecheck =
    showSetupWizard &&
    setupGatewayOnlyPending &&
    !setupChecking &&
    setupGatewayCheck?.state === 'loading';
  const setupAutoRecheckSeconds =
    setupAutoRecheckAt === null
      ? null
      : Math.max(0, Math.ceil((setupAutoRecheckAt - setupAutoRecheckTick) / 1000));

  useEffect(() => {
    let canceled = false;

    async function boot() {
      try {
        const result = await loadBootstrapData();
        if (!canceled) {
          await checkSetupGateway(result.targets);
        }
      } catch (err) {
        if (!canceled) {
          const message = messageFromError(err);
          setError(message);
          setSetupGatewayCheck({ state: 'failed', detail: message });
        }
      }
    }

    void boot();

    return () => {
      canceled = true;
    };
  }, []);

  useEffect(() => {
    const input = folderInputRef.current;
    if (!input) {
      return;
    }
    input.setAttribute('directory', '');
    input.setAttribute('webkitdirectory', '');
  }, [activeView, workspaceReady]);

  useEffect(() => {
    if (!setupShouldAutoRecheck) {
      setSetupAutoRecheckAt(null);
      return;
    }

    const nextCheckAt = Date.now() + setupAutoRecheckDelayMs;
    setSetupAutoRecheckAt(nextCheckAt);
    const timeoutID = window.setTimeout(() => {
      void runSetupChecks({ silent: true });
    }, setupAutoRecheckDelayMs);

    return () => {
      window.clearTimeout(timeoutID);
    };
  }, [setupShouldAutoRecheck, setupLastChecked, setupFingerprint]);

  useEffect(() => {
    if (setupAutoRecheckAt === null) {
      return;
    }

    setSetupAutoRecheckTick(Date.now());
    const intervalID = window.setInterval(() => {
      setSetupAutoRecheckTick(Date.now());
    }, 1000);

    return () => {
      window.clearInterval(intervalID);
    };
  }, [setupAutoRecheckAt]);

  useEffect(() => {
    if (!asking || askStartedAt === null) {
      return;
    }

    const updateElapsed = () => {
      setAskElapsedSeconds(Math.max(0, Math.floor((Date.now() - askStartedAt) / 1000)));
    };

    updateElapsed();
    const interval = window.setInterval(updateElapsed, 1000);
    return () => window.clearInterval(interval);
  }, [asking, askStartedAt]);

  useEffect(() => {
    if (!trackingIngestion) {
      return;
    }

    let canceled = false;

    async function refreshIngestionState() {
      setIngestionSyncing(true);
      try {
        const [documentsResult, dataSourcesResult, jobsResult, sourceDetailResult, detailResult] =
          await Promise.all([
            listDocuments(tenantID),
            listDataSources(tenantID),
            listJobs(tenantID),
            trackedSourceID ? getDataSource(tenantID, trackedSourceID) : Promise.resolve(null),
            trackedDocumentID ? getDocument(tenantID, trackedDocumentID) : Promise.resolve(null),
          ]);
        if (canceled) {
          return;
        }

        setDocuments(documentsResult);
        setDataSources(dataSourcesResult);
        setJobs(jobsResult);
        if (sourceDetailResult) {
          setSourceDetail(sourceDetailResult);
        }
        if (detailResult) {
          setDocumentDetail(detailResult);
        }
        setRegistration((current) =>
          current ? mergeRegistrationProgress(current, documentsResult, jobsResult) : current,
        );
        setLastIngestionSync(new Date().toISOString());
      } catch (err) {
        if (!canceled) {
          setError(messageFromError(err));
        }
      } finally {
        if (!canceled) {
          setIngestionSyncing(false);
        }
      }
    }

    void refreshIngestionState();
    const interval = window.setInterval(() => void refreshIngestionState(), 3000);
    return () => {
      canceled = true;
      window.clearInterval(interval);
    };
  }, [tenantID, trackedDocumentID, trackedSourceID, trackingIngestion]);

  useEffect(() => {
    if (activeView !== 'settings' || !tenantID || !canManageMembers) {
      setTenantMembers(null);
      return;
    }

    void refreshTenantMembers(tenantID);
  }, [activeView, tenantID, canManageMembers]);

  useEffect(() => {
    if (activeView !== 'settings' || !tenantID || !canManageTenant) {
      setAuditEvents(null);
      setModelAuditEvents(null);
      return;
    }

    void refreshAuditEvents(tenantID);
    void refreshModelAuditEvents(tenantID);
  }, [activeView, tenantID, canManageTenant]);

  async function loadBootstrapData() {
    const [healthResult, readinessResult, targetsResult, currentUserResult] = await Promise.all([
      getHealth(),
      getReadiness(),
      getModelTargets(),
      getCurrentUser(),
    ]);
    setHealth(healthResult);
    setReadiness(readinessResult);
    setTargets(targetsResult.targets);
    setCurrentUser(currentUserResult);

    const initialTenantID = currentUserResult.memberships[0]?.tenant.id ?? '';
    setTenantID(initialTenantID);
    if (!initialTenantID) {
      setDocuments({ documents: [] });
      setDataSources({ sources: [] });
      setSourceSavedViews([]);
      setCustomSourcePolicyProfiles([]);
      setJobs({ jobs: [] });
      setAuditEvents(null);
      setModelAuditEvents(null);
      setConversations({ conversations: [] });
      setActiveView('ask');
      return { targets: targetsResult.targets };
    }

    const [
      documentsResult,
      dataSourcesResult,
      sourceViewsResult,
      sourcePolicyProfilesResult,
      jobsResult,
      conversationsResult,
    ] = await Promise.all([
      listDocuments(initialTenantID),
      listDataSources(initialTenantID),
      listSourceViews(initialTenantID),
      listSourcePolicyProfiles(initialTenantID),
      listJobs(initialTenantID),
      listConversations(initialTenantID),
    ]);
    setDocuments(documentsResult);
    setDataSources(dataSourcesResult);
    setSourceSavedViews(sourceViewsResult.views);
    setCustomSourcePolicyProfiles(sortSourcePolicyProfiles(sourcePolicyProfilesResult.profiles.map(sourcePolicyProfileToOption)));
    setJobs(jobsResult);
    setConversations(conversationsResult);
    return { targets: targetsResult.targets };
  }

  async function checkSetupGateway(nextTargets = targets) {
    setSetupChecking(true);
    setSetupGatewayCheck(null);
    try {
      const target = nextTargets.find((item) => item.name === 'general') ?? nextTargets[0];
      if (!target) {
        setSetupGatewayCheck({
          state: 'failed',
          detail: 'No model target is configured.',
        });
        return;
      }

      const result = await checkModelTarget({ tenant_id: '', target: target.name });
      setSetupGatewayCheck({
        state: 'ok',
        detail: `${result.model || result.route.model} ${result.latency_ms}ms`,
      });
    } catch (err) {
      const apiError = err instanceof ApiError ? err : null;
      setSetupGatewayCheck({
        state: apiError?.checkState === 'loading' || apiError?.retryable ? 'loading' : 'failed',
        detail: messageFromError(err),
        error_class: apiError?.errorClass,
        retryable: apiError?.retryable,
      });
    } finally {
      setSetupChecking(false);
      setSetupLastChecked(new Date().toISOString());
    }
  }

  async function runSetupChecks(options: { silent?: boolean } = {}) {
    if (!options.silent) {
      setError(null);
    }
    setSetupChecking(true);
    try {
      const result = await loadBootstrapData();
      await checkSetupGateway(result.targets);
    } catch (err) {
      const message = messageFromError(err);
      const apiError = err instanceof ApiError ? err : null;
      if (!options.silent) {
        setError(message);
      }
      setSetupGatewayCheck({
        state: apiError?.checkState === 'loading' || apiError?.retryable ? 'loading' : 'failed',
        detail: message,
        error_class: apiError?.errorClass,
        retryable: apiError?.retryable,
      });
      setSetupLastChecked(new Date().toISOString());
    } finally {
      setSetupChecking(false);
    }
  }

  function enterConfiguredApp() {
    if (!setupCanEnter) {
      return;
    }
    window.localStorage.setItem(setupWizardStorageKey, setupFingerprint);
    setSetupAcknowledgedKey(setupFingerprint);
  }

  function applySourceFilters(nextFilters: SourceFilterValues) {
    setSourceFilters(normalizeSourceFilters(nextFilters));
    setSourceViewNotice('');
    setSourceBulkResult('');
  }

  async function saveSourceView() {
    if (!canManageSources) {
      setError('Member access is required to save source views');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    const name = sourceViewName.trim();
    if (!name) {
      setSourceViewNotice('Name the view');
      return;
    }
    if (!sourceFiltersActive) {
      setSourceViewNotice('Set filters first');
      return;
    }

    const matchingView = sourceSavedViews.find(
      (view) => view.name.trim().toLowerCase() === name.toLowerCase(),
    );
    setSavingSourceView(true);
    setError(null);
    try {
      const input = {
        tenant_id: tenantID,
        name,
        filters: normalizeSourceFilters(sourceFilters),
      };
      const result = matchingView
        ? await updateSourceView(matchingView.id, input)
        : await createSourceView(input);
      setSourceSavedViews((current) => {
        const withoutCurrent = current.filter((view) => view.id !== result.view.id);
        return sortSourceViews([result.view, ...withoutCurrent]);
      });
      setSourceViewName('');
      setSourceViewNotice(`Saved ${name}`);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSavingSourceView(false);
    }
  }

  async function removeSourceView(viewID: string) {
    if (!canManageSources) {
      setError('Member access is required to remove source views');
      return;
    }
    const view = sourceSavedViews.find((savedView) => savedView.id === viewID);
    setDeletingSourceViewID(viewID);
    setError(null);
    try {
      await deleteSourceView(tenantID, viewID);
      setSourceSavedViews((current) => current.filter((savedView) => savedView.id !== viewID));
      setSourceViewNotice(view ? `Removed ${sourceSavedViewLabel(view)}` : 'Removed view');
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setDeletingSourceViewID('');
    }
  }

  async function saveSourcePolicyProfile(form: SourceFormValues) {
    if (!canManageSources) {
      setError('Member access is required to save source policies');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    const name = sourcePolicyName.trim();
    if (!name) {
      setSourcePolicyNotice('Name the policy');
      return;
    }
    const includePatterns = patternLinesToList(form.include_patterns);
    const excludePatterns = patternLinesToList(form.exclude_patterns);
    const scanIntervalMinutes = Number(form.scan_interval_minutes);
    const matchingProfile = customSourcePolicyProfiles.find(
      (profile) => profile.label.trim().toLowerCase() === name.toLowerCase(),
    );
    setSavingSourcePolicyProfile(true);
    setError(null);
    try {
      const input = {
        tenant_id: tenantID,
        name,
        detail: sourcePolicyDetail.trim(),
        include_patterns: includePatterns,
        exclude_patterns: excludePatterns,
        scan_interval_minutes: scanIntervalMinutes,
      };
      const result =
        matchingProfile && matchingProfile.persisted
          ? await updateSourcePolicyProfile(matchingProfile.id, input)
          : await createSourcePolicyProfile(input);
      const option = sourcePolicyProfileToOption(result.profile);
      setCustomSourcePolicyProfiles((current) => {
        const withoutCurrent = current.filter((profile) => profile.id !== option.id);
        return sortSourcePolicyProfiles([option, ...withoutCurrent]);
      });
      setSourcePolicyName('');
      setSourcePolicyDetail('');
      setSourcePolicyNotice(`Saved ${name}`);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSavingSourcePolicyProfile(false);
    }
  }

  async function removeSourcePolicyProfile(profileID: string) {
    if (!canManageSources) {
      setError('Member access is required to remove source policies');
      return;
    }
    const profile = customSourcePolicyProfiles.find((item) => item.id === profileID);
    setDeletingSourcePolicyProfileID(profileID);
    setError(null);
    try {
      await deleteSourcePolicyProfile(tenantID, profileID);
      setCustomSourcePolicyProfiles((current) => current.filter((item) => item.id !== profileID));
      setSourcePolicyNotice(profile ? `Removed ${profile.label}` : 'Removed policy');
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setDeletingSourcePolicyProfileID('');
    }
  }

  async function refreshDocuments(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setDocuments({ documents: [] });
      return;
    }
    try {
      setDocuments(await listDocuments(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshDataSources(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setDataSources({ sources: [] });
      return;
    }
    try {
      setDataSources(await listDataSources(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshSourceViews(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setSourceSavedViews([]);
      return;
    }
    try {
      const result = await listSourceViews(nextTenantID);
      setSourceSavedViews(result.views);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshSourcePolicyProfiles(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setCustomSourcePolicyProfiles([]);
      return;
    }
    try {
      const result = await listSourcePolicyProfiles(nextTenantID);
      setCustomSourcePolicyProfiles(sortSourcePolicyProfiles(result.profiles.map(sourcePolicyProfileToOption)));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshTenantMembers(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setTenantMembers(null);
      return;
    }
    try {
      setTenantMembers(await listTenantMembers(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshJobs(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setJobs({ jobs: [] });
      return;
    }
    try {
      setJobs(await listJobs(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshAuditEvents(nextTenantID = tenantID, filters = auditFilters) {
    setError(null);
    if (!nextTenantID) {
      setAuditEvents(null);
      return;
    }
    setLoadingAudit(true);
    try {
      setAuditEvents(await listAuditEvents(nextTenantID, auditApiFilters(filters)));
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setLoadingAudit(false);
    }
  }

  async function refreshModelAuditEvents(nextTenantID = tenantID) {
    if (!nextTenantID || !canManageTenant) {
      setModelAuditEvents(null);
      return;
    }
    try {
      setModelAuditEvents(
        await listAuditEvents(nextTenantID, { action: 'conversation.ask', limit: 100 }),
      );
    } catch {
      setModelAuditEvents(null);
    }
  }

  async function refreshConversations(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setConversations({ conversations: [] });
      return;
    }
    try {
      setConversations(await listConversations(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshWorkspaceOverview(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setDocuments({ documents: [] });
      setDataSources({ sources: [] });
      setSourceSavedViews([]);
      setCustomSourcePolicyProfiles([]);
      setJobs({ jobs: [] });
      setConversations({ conversations: [] });
      return;
    }
    try {
      const [
        documentsResult,
        dataSourcesResult,
        sourceViewsResult,
        sourcePolicyProfilesResult,
        jobsResult,
        conversationsResult,
      ] = await Promise.all([
        listDocuments(nextTenantID),
        listDataSources(nextTenantID),
        listSourceViews(nextTenantID),
        listSourcePolicyProfiles(nextTenantID),
        listJobs(nextTenantID),
        listConversations(nextTenantID),
      ]);
      setDocuments(documentsResult);
      setDataSources(dataSourcesResult);
      setSourceSavedViews(sourceViewsResult.views);
      setCustomSourcePolicyProfiles(sortSourcePolicyProfiles(sourcePolicyProfilesResult.profiles.map(sourcePolicyProfileToOption)));
      setJobs(jobsResult);
      setConversations(conversationsResult);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshRuntime() {
    setError(null);
    try {
      const [
        healthResult,
        readinessResult,
        targetsResult,
        jobsResult,
        auditResult,
        modelAuditResult,
      ] =
        await Promise.all([
          getHealth(),
          getReadiness(),
          getModelTargets(),
          tenantID ? listJobs(tenantID) : Promise.resolve({ jobs: [] }),
          tenantID && canManageTenant
            ? listAuditEvents(tenantID, auditApiFilters(auditFilters)).catch(() => null)
            : Promise.resolve(null),
          tenantID && canManageTenant
            ? listAuditEvents(tenantID, { action: 'conversation.ask', limit: 100 }).catch(() => null)
            : Promise.resolve(null),
        ]);
      setHealth(healthResult);
      setReadiness(readinessResult);
      setTargets(targetsResult.targets);
      setJobs(jobsResult);
      setAuditEvents(auditResult);
      setModelAuditEvents(modelAuditResult);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function requestRuntimeShutdown() {
    if (!tenantID) {
      setError('Open a workspace first');
      return;
    }
    if (!canManageTenant) {
      setError('Owner or admin access is required to shut down the stack');
      return;
    }
    if (!readiness?.shutdown_available) {
      setError('UI shutdown is not available for this deployment');
      return;
    }
    const confirmation = window.prompt('Type SHUTDOWN to stop Nexus Local containers.');
    if (confirmation !== 'SHUTDOWN') {
      return;
    }

    setShuttingDown(true);
    setShutdownNotice('');
    setError(null);
    try {
      const result = await shutdownRuntime({
        tenant_id: tenantID,
        confirmation,
      });
      const count = result.containers?.length ?? 0;
      setShutdownNotice(
        count > 0
          ? `${count} containers are stopping. This page will disconnect.`
          : result.message,
      );
    } catch (err) {
      setError(messageFromError(err));
      setShuttingDown(false);
    }
  }

  async function testModelTarget(targetName: string) {
    setError(null);
    setCheckingTarget(targetName);
    try {
      const result = await checkModelTarget({
        tenant_id: tenantID,
        target: targetName,
      });
      setTargetChecks((current) => ({
        ...current,
        [targetName]: {
          state: 'ok',
          detail: `${result.model || result.route.model} ${result.latency_ms}ms`,
          checked_at: new Date().toISOString(),
          latency_ms: result.latency_ms,
          model: result.model || result.route.model,
        },
      }));
    } catch (err) {
      setTargetChecks((current) => ({
        ...current,
        [targetName]: {
          state: 'failed',
          detail: messageFromError(err),
          checked_at: new Date().toISOString(),
        },
      }));
    } finally {
      setCheckingTarget('');
    }
  }

  async function switchTenant(nextTenantID: string) {
    setTenantID(nextTenantID);
    setRegistration(null);
    setSourceDetail(null);
    setDocumentDetail(null);
    setDataSources(null);
    setFolderFiles([]);
    setFolderImportProgress('');
    setContextSourceOpen(false);
    setSourceConnectNotice('');
    if (folderInputRef.current) {
      folderInputRef.current.value = '';
    }
    setSourceSavedViews([]);
    setCustomSourcePolicyProfiles([]);
    setSearchResult(null);
    setSelectedSearchSourceKey('');
    setTenantMembers(null);
    setAuditEvents(null);
    setModelAuditEvents(null);
    setSelectedConversationID('');
    setConversationMessages(null);
    setAskForm((current) => ({ ...current, conversation_id: '', document_id: '' }));
    setSearchForm((current) => ({ ...current, document_id: '' }));
    setError(null);
    if (!nextTenantID) {
      setDocuments({ documents: [] });
      setDataSources({ sources: [] });
      setSourceSavedViews([]);
      setCustomSourcePolicyProfiles([]);
      setSourceDetail(null);
      setJobs({ jobs: [] });
      setTenantMembers(null);
      setAuditEvents(null);
      setModelAuditEvents(null);
      setConversations({ conversations: [] });
      return;
    }
    try {
      const [
        documentsResult,
        dataSourcesResult,
        sourceViewsResult,
        sourcePolicyProfilesResult,
        jobsResult,
        conversationsResult,
        auditResult,
        modelAuditResult,
      ] = await Promise.all([
        listDocuments(nextTenantID),
        listDataSources(nextTenantID),
        listSourceViews(nextTenantID),
        listSourcePolicyProfiles(nextTenantID),
        listJobs(nextTenantID),
        listConversations(nextTenantID),
        listAuditEvents(nextTenantID, auditApiFilters(auditFilters)).catch(() => null),
        canManageTenant
          ? listAuditEvents(nextTenantID, { action: 'conversation.ask', limit: 100 }).catch(() => null)
          : Promise.resolve(null),
      ]);
      setDocuments(documentsResult);
      setDataSources(dataSourcesResult);
      setSourceSavedViews(sourceViewsResult.views);
      setCustomSourcePolicyProfiles(sortSourcePolicyProfiles(sourcePolicyProfilesResult.profiles.map(sourcePolicyProfileToOption)));
      setJobs(jobsResult);
      setConversations(conversationsResult);
      setAuditEvents(auditResult);
      setModelAuditEvents(modelAuditResult);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshCurrentUserAfterMemberChange(nextTenantID = tenantID) {
    const nextUser = await getCurrentUser();
    setCurrentUser(nextUser);
    if (nextTenantID && !nextUser.memberships.some((membership) => membership.tenant.id === nextTenantID)) {
      const replacementTenantID = nextUser.memberships[0]?.tenant.id ?? '';
      await switchTenant(replacementTenantID);
    }
  }

  async function submitUpload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canUploadDocuments) {
      setError('Member access is required to upload documents');
      return;
    }
    if (!file) {
      setError('Choose a file');
      return;
    }
    await uploadWorkspaceFile(file);
  }

  async function uploadSampleDocument() {
    if (!canUploadDocuments) {
      setError('Member access is required to add sample documents');
      return;
    }
    await runSampleFlow(tenantID);
  }

  async function runSampleFlow(nextTenantID = tenantID) {
    if (!nextTenantID) {
      setError('Create a workspace first');
      return;
    }
    const existingSample = nextTenantID === tenantID ? sampleDocument : null;
    setUploadingSample(true);
    setError(null);
    try {
      if (existingSample && existingSample.status !== 'failed') {
        configureSampleWorkspace(existingSample.id);
        if (existingSample.status !== 'ready') {
          setSampleFlowState('indexing');
          await waitForDocumentReady(nextTenantID, existingSample.id);
        }
        if (await searchSampleDocument(nextTenantID, existingSample.id)) {
          setSampleFlowState('ready');
        } else {
          setSampleFlowState('failed');
        }
        return;
      }

      setSampleFlowState('uploading');
      const sampleFile = new File([sampleDocumentContent], sampleDocumentName, {
        type: 'text/markdown',
      });
      const result = await uploadWorkspaceFile(sampleFile, nextTenantID);
      if (!result) {
        setSampleFlowState('failed');
        return;
      }

      const nextDocumentID = result.document.id;
      setSampleFlowDocumentID(nextDocumentID);
      configureSampleWorkspace(nextDocumentID);
      setSampleFlowState('indexing');
      await waitForDocumentReady(nextTenantID, nextDocumentID);
      if (await searchSampleDocument(nextTenantID, nextDocumentID)) {
        setSampleFlowState('ready');
      } else {
        setSampleFlowState('failed');
      }
    } catch (err) {
      setSampleFlowState('failed');
      setError(messageFromError(err));
    } finally {
      setUploadingSample(false);
    }
  }

  function configureSampleWorkspace(documentID: string) {
    setActiveView('ask');
    setSearchResult(null);
    setSelectedSearchSourceKey('');
    setAskResult(null);
    setSelectedAskSourceKey('');
    setStreamAnswer('');
    setStreamStatus('');
    setAskPhase('idle');
    setAskForm((current) => ({
      ...current,
      document_id: documentID,
      question: current.question.trim() ? current.question : sampleQuestion,
    }));
    setSearchForm((current) => ({
      ...current,
      document_id: documentID,
      query: sampleSearchPhrase,
      limit: 5,
    }));
  }

  async function waitForDocumentReady(nextTenantID: string, documentID: string) {
    const deadline = Date.now() + sampleDocumentWaitMs;
    let latestDetail: DocumentDetailResponse | null = null;
    while (Date.now() < deadline) {
      latestDetail = await getDocument(nextTenantID, documentID);
      setDocumentDetail(latestDetail);
      if (latestDetail.document.status === 'ready') {
        await Promise.all([refreshDocuments(nextTenantID), refreshJobs(nextTenantID)]);
        return latestDetail;
      }
      if (latestDetail.document.status === 'failed') {
        throw new Error(`Sample ingestion failed for ${latestDetail.document.name}`);
      }
      await waitForMs(2000);
    }
    throw new Error(`Sample ingestion is still ${latestDetail?.document.status ?? 'pending'}`);
  }

  async function searchSampleDocument(nextTenantID = tenantID, documentID = sampleDocument?.id ?? '') {
    if (!nextTenantID || !documentID) {
      return false;
    }
    setSearching(true);
    setError(null);
    try {
      setSearchForm((current) => ({
        ...current,
        document_id: documentID,
        query: sampleSearchPhrase,
        limit: 5,
      }));
      const result = await searchDocuments({
        tenant_id: nextTenantID,
        document_id: documentID,
        query: sampleSearchPhrase,
        limit: 5,
      });
      setSearchResult(result);
      setSelectedSearchSourceKey(result.hits[0] ? resultHitKey(result.hits[0]) : '');
      if (canManageTenant && nextTenantID === tenantID) {
        await refreshAuditEvents(nextTenantID);
      }
      return true;
    } catch (err) {
      setError(messageFromError(err));
      return false;
    } finally {
      setSearching(false);
    }
  }

  async function uploadWorkspaceFile(nextFile: File, nextTenantID = tenantID) {
    if (!canUploadDocuments) {
      setError('Member access is required to upload documents');
      return null;
    }
    if (!nextTenantID) {
      setError('Create a workspace first');
      return null;
    }
    setSubmitting(true);
    setError(null);
    try {
      const result = await uploadDocument({ tenant_id: nextTenantID, file: nextFile });
      setRegistration(result);
      setDocumentDetail({ document: result.document, jobs: [result.job] });
      await Promise.all([
        refreshDocuments(nextTenantID),
        refreshJobs(nextTenantID),
        canManageTenant && nextTenantID === tenantID
          ? refreshAuditEvents(nextTenantID)
          : Promise.resolve(),
      ]);
      return result;
    } catch (err) {
      setError(messageFromError(err));
      return null;
    } finally {
      setSubmitting(false);
    }
  }

  async function submitFolderImport(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canUploadDocuments) {
      setError('Member access is required to import folders');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    if (folderFiles.length === 0) {
      setError('Choose a folder first');
      return;
    }

    const uploadableFiles = folderFiles.filter(isSupportedUploadFile);
    const skippedCount = folderFiles.length - uploadableFiles.length;
    if (uploadableFiles.length === 0) {
      setError('No supported documents were found in that folder');
      return;
    }

    setFolderImporting(true);
    setFolderImportProgress(`Starting 0/${uploadableFiles.length}`);
    setSourceConnectNotice('');
    setError(null);

    let nextIndex = 0;
    let uploadedCount = 0;
    let failedCount = 0;
    let firstError = '';
    const uploadResults: RegisterDocumentResponse[] = [];

    async function uploadNext() {
      while (nextIndex < uploadableFiles.length) {
        const currentIndex = nextIndex;
        nextIndex += 1;
        const nextFile = uploadableFiles[currentIndex];
        const displayName = folderRelativeName(nextFile);
        setFolderImportProgress(`${currentIndex + 1}/${uploadableFiles.length} ${displayName}`);
        try {
          const result = await uploadDocument({
            tenant_id: tenantID,
            file: nextFile,
            name: displayName,
          });
          uploadResults.push(result);
          uploadedCount += 1;
        } catch (err) {
          failedCount += 1;
          if (!firstError) {
            firstError = messageFromError(err);
          }
        }
      }
    }

    try {
      const workers = Array.from(
        { length: Math.min(folderImportConcurrency, uploadableFiles.length) },
        () => uploadNext(),
      );
      await Promise.all(workers);

      const latestResult = uploadResults[uploadResults.length - 1];
      if (latestResult) {
        setRegistration(latestResult);
        setDocumentDetail({ document: latestResult.document, jobs: [latestResult.job] });
      }

      await Promise.all([
        refreshDocuments(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);

      const resultParts = [`${uploadedCount} imported`];
      if (skippedCount > 0) {
        resultParts.push(`${skippedCount} skipped`);
      }
      if (failedCount > 0) {
        resultParts.push(`${failedCount} failed`);
      }
      setSourceConnectNotice(resultParts.join(', '));
      if (failedCount > 0) {
        setError(firstError || 'Some files could not be imported');
      }
      setFolderFiles([]);
      if (folderInputRef.current) {
        folderInputRef.current.value = '';
      }
    } finally {
      setFolderImporting(false);
      setFolderImportProgress('');
    }
  }

  function applySourceTemplate(template: SourceTemplate) {
    setSourceForm({
      type: template.type,
      name: template.name,
      root_path: template.root_path,
      connector_provider: template.connector_provider ?? '',
      connector_resource_id: template.connector_resource_id ?? '',
      connector_credential_ref: template.connector_credential_ref ?? '',
      connector_notes: template.connector_notes ?? '',
      include_patterns: template.include_patterns,
      exclude_patterns: template.exclude_patterns,
      scan_interval_minutes: template.scan_interval_minutes,
    });
  }

  function openContextSourcePanel() {
    setContextSourceOpen(true);
    setSourceConnectNotice('');
    if (!sourceForm.root_path.trim() && quickSourceTemplates[0]) {
      applySourceTemplate(quickSourceTemplates[0]);
    }
  }

  function sourceFormWithPolicy(
    form: SourceFormValues,
    profile: SourcePolicyProfile,
  ): SourceFormValues {
    return {
      ...form,
      include_patterns: profile.include_patterns,
      exclude_patterns: profile.exclude_patterns,
      scan_interval_minutes: profile.scan_interval_minutes,
    };
  }

  function applySourcePolicy(profile: SourcePolicyProfile) {
    setSourceForm((current) => sourceFormWithPolicy(current, profile));
  }

  function applySourceEditPolicy(profile: SourcePolicyProfile) {
    setSourceEditForm((current) => sourceFormWithPolicy(current, profile));
  }

  function renderSourcePolicyProfile(
    profile: SourcePolicyProfile,
    onApply: (profile: SourcePolicyProfile) => void,
    disabled = false,
  ) {
    const removing = deletingSourcePolicyProfileID === profile.id;
    return (
      <div className="sourcePolicyCard" key={profile.id}>
        <button
          className="sourcePolicyApply"
          disabled={disabled || removing}
          onClick={() => onApply(profile)}
          type="button"
        >
          <strong>{profile.label}</strong>
          <span>{sourceScheduleLabel(Number(profile.scan_interval_minutes))}</span>
          <small>{profile.detail || (profile.persisted ? 'Custom policy' : 'Built in')}</small>
        </button>
        {profile.persisted && (
          <button
            className="sourcePolicyRemove"
            disabled={!canManageSources || removing || savingSourcePolicyProfile}
            onClick={() => void removeSourcePolicyProfile(profile.id)}
            type="button"
          >
            {removing ? 'Removing' : 'Remove'}
          </button>
        )}
      </div>
    );
  }

  async function submitDataSource(
    event: FormEvent<HTMLFormElement>,
    options: { closePanel?: boolean; scanAfterCreate?: boolean } = {},
  ) {
    event.preventDefault();
    if (!canManageSources) {
      setError('Member access is required to add sources');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    if (!sourceForm.name.trim() || !sourceForm.root_path.trim()) {
      setError('Enter a source name and path');
      return;
    }
    setCreatingSource(true);
    setSourceConnectNotice('');
    setError(null);
    try {
      const createResult = await createDataSource({
        tenant_id: tenantID,
        type: sourceForm.type,
        name: sourceForm.name.trim(),
        root_path: sourceForm.root_path.trim(),
        connector_config: connectorConfigFromSourceForm(sourceForm),
        include_patterns: patternLinesToList(sourceForm.include_patterns),
        exclude_patterns: patternLinesToList(sourceForm.exclude_patterns),
        scan_interval_minutes: Number(sourceForm.scan_interval_minutes),
      });
      const connectorSource = sourceIsConnector(createResult.source);
      let detailSource = createResult.source;
      const detailJobs: ListJobsResponse['jobs'] = [];
      let preflightError = '';
      let scanError = '';
      if (!connectorSource) {
        try {
          const preflightResult = await preflightDataSource(tenantID, createResult.source.id);
          detailSource = preflightResult.source;
          detailJobs.push(preflightResult.job);
        } catch (err) {
          preflightError = messageFromError(err);
        }
        if (options.scanAfterCreate && !preflightError) {
          try {
            const scanResult = await scanDataSource(tenantID, createResult.source.id);
            detailSource = scanResult.source;
            detailJobs.push(scanResult.job);
          } catch (err) {
            scanError = messageFromError(err);
          }
        }
      }
      setSourceDetail({
        source: detailSource,
        jobs: detailJobs,
        scan_entries: [],
        scan_summary: emptyDataSourceScanSummary(),
        scan_entries_page: emptyDataSourceScanEntryPage(),
        failed_documents: 0,
      });
      setSourceEditForm(sourceFormFromSource(detailSource));
      setSourceForm(initialSourceForm);
      await Promise.all([
        refreshDocuments(tenantID),
        refreshDataSources(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);
      if (options.closePanel && !preflightError && !scanError) {
        setContextSourceOpen(false);
      }
      if (connectorSource) {
        setSourceConnectNotice(`Saved ${createResult.source.name}`);
      } else if (!preflightError && !scanError) {
        setSourceConnectNotice(
          options.scanAfterCreate
            ? `Scan queued for ${createResult.source.name}`
            : `Path check queued for ${createResult.source.name}`,
        );
      }
      if (scanError) {
        setError(`Source added. Scan did not start: ${scanError}`);
      } else if (preflightError) {
        setError(`Source added. Path check did not start: ${preflightError}`);
      }
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setCreatingSource(false);
    }
  }

  async function submitSourceUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canManageSources) {
      setError('Member access is required to edit sources');
      return;
    }
    if (!tenantID || !sourceDetail) {
      setError('Open a source first');
      return;
    }
    if (!sourceEditForm.name.trim() || !sourceEditForm.root_path.trim()) {
      setError('Enter a source name and path');
      return;
    }
    setSavingSourceID(sourceDetail.source.id);
    setError(null);
    try {
      const result = await updateDataSource(sourceDetail.source.id, {
        tenant_id: tenantID,
        type: sourceEditForm.type,
        name: sourceEditForm.name.trim(),
        root_path: sourceEditForm.root_path.trim(),
        connector_config: connectorConfigFromSourceForm(sourceEditForm),
        include_patterns: patternLinesToList(sourceEditForm.include_patterns),
        exclude_patterns: patternLinesToList(sourceEditForm.exclude_patterns),
        scan_interval_minutes: Number(sourceEditForm.scan_interval_minutes),
      });
      setSourceDetail((current) =>
        current && current.source.id === result.source.id
          ? { ...current, source: result.source }
          : current,
      );
      setSourceEditForm(sourceFormFromSource(result.source));
      await Promise.all([
        refreshDataSources(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSavingSourceID('');
    }
  }

  async function removeDataSource(
    source: ListDataSourcesResponse['sources'][number],
    deleteDocuments = false,
  ) {
    if (!canManageSources) {
      setError('Member access is required to archive sources');
      return;
    }
    const prompt = deleteDocuments
      ? `Archive ${source.name} and delete documents imported from this source?`
      : `Archive ${source.name}?`;
    if (!window.confirm(prompt)) {
      return;
    }
    if (deleteDocuments) {
      setDeletingSourceDocumentsID(source.id);
    } else {
      setArchivingSourceID(source.id);
    }
    setError(null);
    try {
      const result = await archiveDataSource(tenantID, source.id, {
        delete_documents: deleteDocuments,
      });
      setSourceDetail((current) =>
        current && current.source.id === source.id ? { ...current, source: result.source } : current,
      );
      await Promise.all([
        refreshDataSources(tenantID),
        deleteDocuments ? refreshDocuments(tenantID) : Promise.resolve(),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setArchivingSourceID('');
      setDeletingSourceDocumentsID('');
    }
  }

  async function requestDataSourcePreflight(source: ListDataSourcesResponse['sources'][number]) {
    if (!canImportSources) {
      setError('Member access is required to check source paths');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    setPreflightingSourceID(source.id);
    setError(null);
    try {
      const result = await preflightDataSource(tenantID, source.id);
      setSourceDetail((current) =>
        current && current.source.id === source.id
          ? {
              ...current,
              source: result.source,
              jobs: [result.job, ...current.jobs.filter((job) => job.id !== result.job.id)],
            }
          : current,
      );
      await Promise.all([
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
        sourceDetail?.source.id === source.id ? refreshSourceDetail(source.id) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setPreflightingSourceID('');
    }
  }

  async function requestDataSourcePlan(source: ListDataSourcesResponse['sources'][number]) {
    if (!canImportSources) {
      setError('Member access is required to plan imports');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    setPlanningSourceID(source.id);
    setError(null);
    try {
      const result = await planDataSource(tenantID, source.id);
      setSourceDetail((current) =>
        current && current.source.id === source.id
          ? {
              ...current,
              source: result.source,
              jobs: [result.job, ...current.jobs.filter((job) => job.id !== result.job.id)],
            }
          : current,
      );
      await Promise.all([
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
        sourceDetail?.source.id === source.id ? refreshSourceDetail(source.id) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setPlanningSourceID('');
    }
  }

  async function requestDataSourceScan(source: ListDataSourcesResponse['sources'][number]) {
    if (!canImportSources) {
      setError('Member access is required to run source imports');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    const planJob =
      sourceDetail?.source.id === source.id
        ? sourceLatestRelevantPlanJob(sourceDetail)
        : latestSourcePlanJobs.get(source.id);
    if (sourcePlanBlocksScan(source, planJob)) {
      setError('Import plan failed. Run Plan again after fixing the source.');
      return;
    }
    const reviewPrompt = sourceFirstScanReviewPrompt(source, planJob);
    if (reviewPrompt && !window.confirm(reviewPrompt)) {
      return;
    }
    setScanningSourceID(source.id);
    setError(null);
    try {
      const result = await scanDataSource(tenantID, source.id);
      setSourceDetail((current) => {
        if (!current || current.source.id !== source.id) {
          return {
            source: result.source,
            jobs: [result.job],
            scan_entries: [],
            scan_summary: emptyDataSourceScanSummary(),
            scan_entries_page: emptyDataSourceScanEntryPage(),
            failed_documents: 0,
          };
        }
        return {
          source: result.source,
          jobs: [result.job, ...current.jobs.filter((job) => job.id !== result.job.id)],
          scan_entries: current.scan_entries,
          scan_summary: current.scan_summary,
          scan_entries_page: current.scan_entries_page,
          failed_documents: current.failed_documents,
        };
      });
      await Promise.all([
        refreshDataSources(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setScanningSourceID('');
    }
  }

  async function runSourceBulkAction(action: SourceBulkAction) {
    if (!canImportSources) {
      setError('Member access is required to run source actions');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    const eligibleSources = filteredSources.filter((source) =>
      sourceBulkActionIsEligible(
        action,
        source,
        activeSourcePreflightJobs,
        activeSourcePlanJobs,
        activeSourceScanJobs,
        latestSourcePreflightJobs,
        latestSourcePlanJobs,
      ),
    );
    if (eligibleSources.length === 0) {
      setSourceBulkResult(`No eligible sources for ${sourceBulkActionVerb(action).toLowerCase()}`);
      return;
    }

    setSourceBulkAction(action);
    setSourceBulkResult('');
    setError(null);
    let queued = 0;
    const failures: string[] = [];
    try {
      for (const source of eligibleSources) {
        try {
          if (action === 'preflight') {
            await preflightDataSource(tenantID, source.id);
          } else if (action === 'plan') {
            await planDataSource(tenantID, source.id);
          } else if (action === 'scan') {
            await scanDataSource(tenantID, source.id);
          } else if (action === 'retry_failures') {
            await retryFailedDataSourceDocuments(tenantID, source.id);
          } else {
            await reindexDataSource(tenantID, source.id);
          }
          queued++;
        } catch (err) {
          failures.push(`${source.name}: ${messageFromError(err)}`);
        }
      }

      const refreshes: Array<Promise<void>> = [
        refreshDataSources(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
        sourceDetail ? refreshSourceDetail(sourceDetail.source.id) : Promise.resolve(),
      ];
      if (action === 'retry_failures' || action === 'reindex') {
        refreshes.push(refreshDocuments(tenantID));
      }
      await Promise.all(refreshes);

      setSourceBulkResult(
        `${queued} ${sourceBulkActionResultLabel(action, queued)}${
          failures.length > 0 ? ` / ${failures.length} failed` : ''
        }`,
      );
      if (failures.length > 0) {
        setError(
          failures.length === 1
            ? failures[0]
            : `${failures[0]} (${failures.length - 1} more failed)`,
        );
      }
    } finally {
      setSourceBulkAction('');
    }
  }

  async function cancelSourceScan(source: ListDataSourcesResponse['sources'][number]) {
    if (!canImportSources) {
      setError('Member access is required to cancel source imports');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    setCancelingSourceScanID(source.id);
    setError(null);
    try {
      const result = await cancelDataSourceScan(tenantID, source.id);
      setSourceDetail((current) =>
        current && current.source.id === source.id
          ? {
              ...current,
              source: result.source,
              jobs: [result.job, ...current.jobs.filter((job) => job.id !== result.job.id)],
            }
          : current,
      );
      await Promise.all([
        refreshDataSources(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
        sourceDetail?.source.id === source.id ? refreshSourceDetail(source.id) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setCancelingSourceScanID('');
    }
  }

  async function requestDataSourceReindex(source: ListDataSourcesResponse['sources'][number]) {
    if (!canImportSources) {
      setError('Member access is required to reindex source documents');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    setReindexingSourceID(source.id);
    setError(null);
    try {
      const result = await reindexDataSource(tenantID, source.id);
      setSourceDetail((current) =>
        current && current.source.id === source.id ? { ...current, source: result.source } : current,
      );
      await Promise.all([
        refreshDataSources(tenantID),
        refreshDocuments(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setReindexingSourceID('');
    }
  }

  async function requestRetryFailedSourceDocuments(
    source: ListDataSourcesResponse['sources'][number],
  ) {
    if (!canImportSources) {
      setError('Member access is required to retry failed source documents');
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    setRetryingSourceFailuresID(source.id);
    setError(null);
    try {
      const result = await retryFailedDataSourceDocuments(tenantID, source.id);
      setSourceDetail((current) =>
        current && current.source.id === source.id
          ? {
              ...current,
              source: result.source,
              jobs: [
                ...result.jobs,
                ...current.jobs.filter(
                  (job) => !result.jobs.some((queued) => queued.id === job.id),
                ),
              ],
            }
          : current,
      );
      await Promise.all([
        refreshDocuments(tenantID),
        refreshDataSources(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
        sourceDetail?.source.id === source.id ? refreshSourceDetail(source.id) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setRetryingSourceFailuresID('');
    }
  }

  async function openSource(source: ListDataSourcesResponse['sources'][number]) {
    setLoadingSourceID(source.id);
    setError(null);
    try {
      const detail = await getDataSource(tenantID, source.id, sourceScanEntryOptions('all', 0));
      setSourceDetail(detail);
      setSourceEditForm(sourceFormFromSource(detail.source));
      setScanEntryFilter('all');
      setScanEntryOffset(detail.scan_entries_page.offset);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setLoadingSourceID('');
    }
  }

  async function refreshSourceDetail(sourceID = sourceDetail?.source.id ?? '') {
    if (!tenantID || !sourceID) {
      return;
    }
    setRefreshingSourceID(sourceID);
    setError(null);
    try {
      const [detail, dataSourcesResult, jobsResult, auditResult] = await Promise.all([
        getDataSource(tenantID, sourceID, sourceScanEntryOptions(scanEntryFilter, scanEntryOffset)),
        listDataSources(tenantID),
        listJobs(tenantID),
        canManageTenant
          ? listAuditEvents(tenantID, auditApiFilters(auditFilters)).catch(() => null)
          : Promise.resolve(null),
      ]);
      setSourceDetail(detail);
      setSourceEditForm(sourceFormFromSource(detail.source));
      setScanEntryOffset(detail.scan_entries_page.offset);
      setDataSources(dataSourcesResult);
      setJobs(jobsResult);
      if (auditResult) {
        setAuditEvents(auditResult);
      }
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setRefreshingSourceID('');
    }
  }

  async function loadSourceScanEntries(filter: ScanEntryFilter, offset: number) {
    if (!tenantID || !sourceDetail) {
      return;
    }
    const safeOffset = Math.max(0, offset);
    setRefreshingSourceID(sourceDetail.source.id);
    setError(null);
    try {
      const detail = await getDataSource(
        tenantID,
        sourceDetail.source.id,
        sourceScanEntryOptions(filter, safeOffset),
      );
      setSourceDetail(detail);
      setSourceEditForm(sourceFormFromSource(detail.source));
      setScanEntryFilter(filter);
      setScanEntryOffset(detail.scan_entries_page.offset);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setRefreshingSourceID('');
    }
  }

  function exportSourceScanEntries(outcome: ScanEntryFilter = scanEntryFilter) {
    if (!tenantID || !sourceDetail) {
      return;
    }
    window.location.href = dataSourceScanEntriesExportUrl(tenantID, sourceDetail.source.id, {
      outcome: outcome === 'all' ? undefined : outcome,
    });
  }

  async function submitTenant(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await createWorkspace(false);
  }

  async function createWorkspace(seedSample: boolean) {
    if (!tenantName.trim()) {
      setError('Enter a workspace name');
      return;
    }
    setCreatingTenant(true);
    setError(null);
    if (seedSample) {
      setSampleFlowState('creating');
    }
    try {
      const hadNoWorkspace = needsWorkspace;
      const created = await createTenant({ name: tenantName });
      setCurrentUser(await getCurrentUser());
      await switchTenant(created.tenant.id);
      if (hadNoWorkspace) {
        setActiveView('ask');
      }
      if (seedSample) {
        await runSampleFlow(created.tenant.id);
      }
    } catch (err) {
      if (seedSample) {
        setSampleFlowState('failed');
      }
      setError(messageFromError(err));
    } finally {
      setCreatingTenant(false);
    }
  }

  async function removeTenant(membership: CurrentUserResponse['memberships'][number]) {
    const confirmed = window.confirm(
      `Delete workspace "${membership.tenant.name}"?\n\nDelete documents and archive sources first. This removes the workspace record, conversations, jobs, and audit rows.`,
    );
    if (!confirmed) {
      return;
    }
    setDeletingTenantID(membership.tenant.id);
    setError(null);
    try {
      await deleteTenant(membership.tenant.id);
      const nextUser = await getCurrentUser();
      setCurrentUser(nextUser);
      if (membership.tenant.id === tenantID) {
        await switchTenant(nextUser.memberships[0]?.tenant.id ?? '');
      }
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setDeletingTenantID('');
    }
  }

  async function submitMember(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    if (!memberForm.user_id.trim() || !memberForm.email.trim()) {
      setError('Enter a user ID and email');
      return;
    }
    setSavingMember(true);
    setError(null);
    try {
      await addTenantMember(tenantID, {
        user_id: memberForm.user_id.trim(),
        email: memberForm.email.trim(),
        name: memberForm.name.trim(),
        role: memberForm.role,
      });
      setMemberForm(initialMemberForm);
      await Promise.all([
        refreshTenantMembers(tenantID),
        refreshCurrentUserAfterMemberChange(tenantID),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSavingMember(false);
    }
  }

  async function removeTenantMember(userID: string, email: string) {
    if (!window.confirm(`Remove ${email || userID}?`)) {
      return;
    }
    setRemovingMemberID(userID);
    setError(null);
    try {
      await deleteTenantMember(tenantID, userID);
      await Promise.all([
        refreshTenantMembers(tenantID),
        refreshCurrentUserAfterMemberChange(tenantID),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setRemovingMemberID('');
    }
  }

  async function openConversation(conversation: ListConversationsResponse['conversations'][number]) {
    setSelectedConversationID(conversation.id);
    setAskForm((current) => ({
      ...current,
      conversation_id: conversation.id,
      model_target: conversation.model_target,
    }));
    setError(null);
    try {
      setConversationMessages(await listConversationMessages(tenantID, conversation.id));
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function openDocument(document: ListDocumentsResponse['documents'][number]) {
    setLoadingDocumentID(document.id);
    setError(null);
    try {
      setDocumentDetail(await getDocument(tenantID, document.id));
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setLoadingDocumentID('');
    }
  }

  function resumeConversation(conversation: ListConversationsResponse['conversations'][number]) {
    setAskForm((current) => ({
      ...current,
      conversation_id: conversation.id,
      model_target: conversation.model_target,
      question: '',
    }));
    setActiveView('ask');
  }

  async function removeDocument(documentID: string, name: string) {
    if (!canUploadDocuments) {
      setError('Member access is required to delete documents');
      return;
    }
    if (!window.confirm(`Delete ${name}?`)) {
      return;
    }
    setDeletingDocumentID(documentID);
    setError(null);
    try {
      await deleteDocument(tenantID, documentID);
      await Promise.all([refreshDocuments(tenantID), refreshJobs(tenantID)]);
      if (documentDetail?.document.id === documentID) {
        setDocumentDetail(null);
      }
      setAskForm((current) =>
        current.document_id === documentID ? { ...current, document_id: '' } : current,
      );
      setSearchForm((current) =>
        current.document_id === documentID ? { ...current, document_id: '' } : current,
      );
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setDeletingDocumentID('');
    }
  }

  async function retryDocumentIngestion(documentID: string) {
    if (!canUploadDocuments) {
      setError('Member access is required to retry document ingestion');
      return;
    }
    setRetryingDocumentID(documentID);
    setError(null);
    try {
      const result = await retryDocument(tenantID, documentID);
      setDocumentDetail((current) => {
        if (!current || current.document.id !== documentID) {
          return { document: result.document, jobs: [result.job] };
        }
        return {
          document: result.document,
          jobs: [result.job, ...current.jobs.filter((job) => job.id !== result.job.id)],
        };
      });
      await Promise.all([refreshDocuments(tenantID), refreshJobs(tenantID)]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setRetryingDocumentID('');
    }
  }

  async function downloadDocumentSource(source: DocumentDetailResponse['document']) {
    setDownloadingDocumentID(source.id);
    setError(null);
    try {
      const blob = await downloadDocument(tenantID, source.id);
      saveBlob(blob, source.name || source.id);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setDownloadingDocumentID('');
    }
  }

  async function removeConversation(conversation: ListConversationsResponse['conversations'][number]) {
    if (!window.confirm(`Delete ${conversation.title || conversation.id}?`)) {
      return;
    }
    setDeletingConversationID(conversation.id);
    setError(null);
    try {
      await deleteConversation(tenantID, conversation.id);
      await refreshConversations(tenantID);
      if (selectedConversationID === conversation.id) {
        setSelectedConversationID('');
        setConversationMessages(null);
      }
      if (askForm.conversation_id === conversation.id) {
        setAskForm((current) => ({ ...current, conversation_id: '' }));
        setAskResult(null);
        setSelectedAskSourceKey('');
        setStreamAnswer('');
      }
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setDeletingConversationID('');
    }
  }

  async function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    if (!searchForm.query.trim()) {
      setError('Enter a query');
      return;
    }
    setSearching(true);
    setError(null);
    try {
      const result = await searchDocuments({
        tenant_id: tenantID,
        document_id: searchForm.document_id || undefined,
        query: searchForm.query,
        limit: Number(searchForm.limit),
        strategy: searchForm.strategy,
      });
      setSearchResult(result);
      setSelectedSearchSourceKey(result.hits[0] ? resultHitKey(result.hits[0]) : '');
      if (canManageTenant) {
        await refreshAuditEvents(tenantID);
      }
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSearching(false);
    }
  }

  async function submitAsk(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (asking) {
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    const question = askForm.question.trim();
    if (!question) {
      setError('Enter a question');
      return;
    }
    await runAsk(question);
  }

  async function runAsk(question: string) {
    if (asking) {
      return;
    }
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    question = question.trim();
    if (!question) {
      setError('Enter a question');
      return;
    }
    setAsking(true);
    setError(null);
    setAskResult(null);
    setSelectedAskSourceKey('');
    setStreamAnswer('');
    setCurrentAskQuestion(question);
    setAskPhase('connecting');
    setAskStartedAt(Date.now());
    setAskElapsedSeconds(0);
    setStreamStatus('Starting');
    let failed = false;
    const controller = new AbortController();
    askAbortRef.current = controller;
    try {
      let streamedResult: AskConversationResponse | undefined;
      await askConversationStream({
        tenant_id: tenantID,
        conversation_id: askForm.conversation_id || undefined,
        document_id: askForm.document_id || undefined,
        model_target: askForm.model_target,
        question,
        limit: Number(askForm.limit),
        strategy: askForm.strategy,
      }, {
        signal: controller.signal,
        onStatus: (message) => {
          setStreamStatus(message);
          setAskPhase(askPhaseFromStatus(message));
        },
        onDelta: (content) => {
          setAskPhase('streaming');
          setStreamStatus('Streaming');
          setStreamAnswer((current) => current + content);
        },
        onDone: (response) => {
          streamedResult = response;
          setAskResult(response);
          setSelectedAskSourceKey(response.hits[0] ? resultHitKey(response.hits[0]) : '');
          setStreamAnswer(response.assistant_message.content);
          setAskPhase('complete');
        },
      });
      if (!streamedResult) {
        return;
      }
      const result = streamedResult;
      setAskResult(result);
      setSelectedAskSourceKey(result.hits[0] ? resultHitKey(result.hits[0]) : '');
      setAskForm((current) => ({
        ...current,
        conversation_id: result.conversation.id,
        question: '',
      }));
      setSelectedConversationID(result.conversation.id);
      await refreshConversations(tenantID);
      if (canManageTenant) {
        await Promise.all([refreshAuditEvents(tenantID), refreshModelAuditEvents(tenantID)]);
      }
      if (conversationMessages && selectedConversationID === result.conversation.id) {
        setConversationMessages(await listConversationMessages(tenantID, result.conversation.id));
      }
    } catch (err) {
      failed = true;
      const message = messageFromError(err);
      setAskPhase('failed');
      setStreamStatus(message);
      if (message !== 'Request canceled.') {
        setError(message);
      }
      if (canManageTenant) {
        await refreshModelAuditEvents(tenantID);
      }
    } finally {
      if (askAbortRef.current === controller) {
        askAbortRef.current = null;
      }
      setAsking(false);
      setAskStartedAt(null);
      if (!failed) {
        setStreamStatus('');
      }
    }
  }

  function cancelAsk() {
    setStreamStatus('Canceling');
    askAbortRef.current?.abort();
  }

  function retryAsk() {
    if (!currentAskQuestion || asking) {
      return;
    }
    void runAsk(currentAskQuestion);
  }

  if (showSetupWizard) {
    return (
      <SetupWizard
        apiURL={apiBase()}
        checks={setupChecks}
        canEnter={setupCanEnter}
        gatewayCheck={setupGatewayCheck}
        lastChecked={setupLastChecked}
        onEnter={enterConfiguredApp}
        onRecheck={() => void runSetupChecks()}
        primaryTarget={setupPrimaryTarget}
        readiness={readiness}
        setupAutoRecheckSeconds={setupAutoRecheckSeconds}
        setupChecking={setupChecking}
      />
    );
  }

  return (
    <main className="appShell">
      <aside className="sidebar">
        <div className="sideBrand">
          <p className="eyebrow">Nexus Local</p>
          <h1>Workspace</h1>
          <span className={health ? 'sideStatus sideStatusReady' : 'sideStatus'}>
            {health ? 'Online' : 'Connecting'}
          </span>
        </div>

        <nav className="sideNav" aria-label="Primary">
          <button
            aria-pressed={activeView === 'ask'}
            className={activeView === 'ask' ? 'sideNavButton sideNavActive' : 'sideNavButton'}
            onClick={() => setActiveView('ask')}
            type="button"
          >
            Dashboard
          </button>
          <button
            aria-pressed={activeView === 'documents' || activeView === 'search'}
            className={
              activeView === 'documents' || activeView === 'search'
                ? 'sideNavButton sideNavActive'
                : 'sideNavButton'
            }
            onClick={() => setActiveView('documents')}
            type="button"
          >
            Library
          </button>
          <button
            aria-pressed={activeView === 'activity' || activeView === 'history'}
            className={
              activeView === 'activity' || activeView === 'history'
                ? 'sideNavButton sideNavActive'
                : 'sideNavButton'
            }
            onClick={() => setActiveView('activity')}
            type="button"
          >
            Activity
          </button>
          <button
            aria-pressed={activeView === 'settings'}
            className={activeView === 'settings' ? 'sideNavButton sideNavActive' : 'sideNavButton'}
            onClick={() => setActiveView('settings')}
            type="button"
          >
            Settings
          </button>
        </nav>

        <section className="sideSection">
          <div className="sideSectionHeader">
            <span>Workspace</span>
          </div>
          <select
            aria-label="Workspace"
            className="tenantSelect"
            onChange={(event) => void switchTenant(event.target.value)}
            value={tenantID}
          >
            {currentUser?.memberships.map((membership) => (
              <option key={membership.tenant.id} value={membership.tenant.id}>
                {membership.tenant.name}
              </option>
            ))}
            {(!currentUser || currentUser.memberships.length === 0) && (
              <option value="">No workspace</option>
            )}
          </select>
        </section>

        <footer className="sideFooter">
          <span>{currentUser?.user.email ?? 'dev@example.local'}</span>
        </footer>
      </aside>

      <section className="contentShell">
        <header className="contentHeader">
          <div>
            <p className="eyebrow">{workspaceLabel}</p>
            <h2>{contentTitle}</h2>
          </div>
          <span className={trackingIngestion ? 'syncStatus syncActive' : 'syncStatus'}>
            {activeView === 'settings' ? readiness?.provider_preset ?? 'starter' : ingestionLabel}
          </span>
        </header>

        {error && <div className="toast">{error}</div>}

        <section className="workspace">
          {activeView === 'ask' && (
            workspaceReady ? (
              <div className="dashboardStack">
                <DashboardAttentionPanel
                  activeDocuments={activeDocuments}
                  activeJobs={activeJobs}
                  failedDocuments={failedDocuments}
                  failedJobs={failedJobs}
                  onConnectSource={openContextSourcePanel}
                  onOpenDocument={(document) => {
                    setActiveView('documents');
                    void openDocument(document);
                  }}
                  onRefresh={() => void refreshWorkspaceOverview()}
                  onViewActivity={() => setActiveView('activity')}
                  onViewLibrary={() => setActiveView('documents')}
                  processingDocumentCount={processingDocumentCount}
                  readyDocumentCount={readyDocumentCount}
                />

                {(freshWorkspace ||
                  sampleFlowState !== 'idle' ||
                  (sampleDocument && documentCount <= 1)) && (
                  <section className="workSurface sampleStartPanel">
                    <div className="surfaceHeader">
                      <h2>First run</h2>
                      <span>{sampleFlowStatusLabel(sampleFlowState, sampleDocument)}</span>
                    </div>
                    <div className="sampleStepGrid">
                      <button
                        className={sampleStepClass(
                          sampleFlowState,
                          Boolean(sampleDocument && sampleDocument.status === 'ready'),
                          Boolean(sampleDocument && sampleDocument.status === 'failed'),
                        )}
                        disabled={
                          submitting || uploadingSample || !workspaceReady || !canUploadDocuments
                        }
                        onClick={() => void runSampleFlow(tenantID)}
                        type="button"
                      >
                        <strong>{sampleFlowPrimaryLabel(sampleFlowState, sampleDocument)}</strong>
                        <span>{sampleDocument?.name ?? sampleDocumentName}</span>
                      </button>
                      <button
                        className={
                          sampleSearchReady
                            ? 'sampleStepButton sampleStepButtonReady'
                            : 'sampleStepButton'
                        }
                        disabled={!sampleDocument || sampleDocument.status !== 'ready' || searching}
                        onClick={() => void searchSampleDocument(tenantID, sampleDocument?.id ?? '')}
                        type="button"
                      >
                        <strong>{searching ? 'Searching' : 'Search sample'}</strong>
                        <span>{sampleSearchPhrase}</span>
                      </button>
                      <button
                        className={
                          askForm.document_id === sampleDocument?.id
                            ? 'sampleStepButton sampleStepButtonReady'
                            : 'sampleStepButton'
                        }
                        disabled={!sampleDocument || sampleDocument.status !== 'ready'}
                        onClick={() => {
                          if (sampleDocument) {
                            configureSampleWorkspace(sampleDocument.id);
                          }
                        }}
                        type="button"
                      >
                        <strong>Ask sample</strong>
                        <span>{sampleQuestion}</span>
                      </button>
                    </div>
                  </section>
                )}

                <div className="workspaceSplit">
                  <div className="workSurface chatSurface">
                    <div className="surfaceHeader">
                      <h2>Ask</h2>
                      <span>{workspaceLabel}</span>
                    </div>
                    <form className="askComposer" onSubmit={submitAsk}>
                      <textarea
                        aria-label="Question"
                        disabled={asking}
                        onChange={(event) =>
                          setAskForm((current) => ({ ...current, question: event.target.value }))
                        }
                        placeholder="Ask a question"
                        value={askForm.question}
                      />
                      <div className="composerActions">
                        <details className="menuPanel">
                          <summary>Options</summary>
                          <div className="menuFields">
                            <label>
                              Target
                              <select
                                disabled={asking}
                                value={askForm.model_target}
                                onChange={(event) =>
                                  setAskForm((current) => ({
                                    ...current,
                                    model_target: event.target.value,
                                  }))
                                }
                              >
                                {targets.length === 0 && <option value="general">general</option>}
                                {targets.map((target) => (
                                  <option key={target.name} value={target.name}>
                                    {target.name}
                                  </option>
                                ))}
                              </select>
                            </label>
                            <label>
                              Source
                              <DocumentSelect
                                disabled={asking}
                                documents={documents?.documents ?? []}
                                value={askForm.document_id}
                                onChange={(value) =>
                                  setAskForm((current) => ({ ...current, document_id: value }))
                                }
                              />
                            </label>
                            <label>
                              Limit
                              <input
                                disabled={asking}
                                max="20"
                                min="1"
                                type="number"
                                value={askForm.limit}
                                onChange={(event) =>
                                  setAskForm((current) => ({
                                    ...current,
                                    limit: Number(event.target.value),
                                  }))
                                }
                              />
                            </label>
                            <label>
                              Retrieval
                              <select
                                disabled={asking}
                                value={askForm.strategy}
                                onChange={(event) =>
                                  setAskForm((current) => ({
                                    ...current,
                                    strategy: event.target.value as RetrievalStrategy,
                                  }))
                                }
                              >
                                {retrievalStrategies.map((strategy) => (
                                  <option key={strategy.value} value={strategy.value}>
                                    {strategy.label}
                                  </option>
                                ))}
                              </select>
                            </label>
                            <label>
                              Conversation
                              <input
                                disabled={asking}
                                value={askForm.conversation_id}
                                onChange={(event) =>
                                  setAskForm((current) => ({
                                    ...current,
                                    conversation_id: event.target.value,
                                  }))
                                }
                              />
                            </label>
                          </div>
                        </details>
                        <div className="composerSubmit">
                          {asking && (
                            <button className="secondaryButton" onClick={cancelAsk} type="button">
                              Cancel
                            </button>
                          )}
                          <button
                            disabled={asking || !workspaceReady || !askForm.question.trim()}
                            type="submit"
                          >
                            {asking ? 'Asking' : 'Ask'}
                          </button>
                        </div>
                      </div>
                    </form>

                {showAskResult && (
                  <div className={asking ? 'answerBox answerBoxActive' : 'answerBox'}>
                    <div className="answerMeta">
                      <strong>{visibleAskTitle}</strong>
                      <span>{visibleAskModel}</span>
                    </div>
                    {showAskProgress && (
                      <div className="answerProgress" role="status" aria-live="polite">
                        {asking ? (
                          <span className="spinner" aria-hidden="true"></span>
                        ) : (
                          <span className="progressDot progressDotFailed" aria-hidden="true"></span>
                        )}
                        <strong>
                          {askPhase === 'failed'
                            ? 'Request failed'
                            : askPhaseLabel(askPhase, visibleAskAnswer)}
                        </strong>
                        <em>{formatElapsed(askElapsedSeconds)}</em>
                        <span
                          className={
                            askProgressHint
                              ? 'answerProgressDetail answerProgressDetailSlow'
                              : 'answerProgressDetail'
                          }
                        >
                          {askProgressDetail}
                        </span>
                        {askRetryAvailable && (
                          <button className="secondaryButton" onClick={retryAsk} type="button">
                            Retry
                          </button>
                        )}
                      </div>
                    )}
                    {showAskProgress && (
                      <div className="answerPhaseStrip" aria-label="Answer progress">
                        {askPhaseSteps.map((step) => (
                          <span className={askPhaseStepClass(askPhase, step.phase)} key={step.phase}>
                            {step.label}
                          </span>
                        ))}
                      </div>
                    )}
                    {askProgressHint && (
                      <div
                        className={
                          askProgressHint.tone === 'risk'
                            ? 'answerProgressHint answerProgressHintRisk'
                            : 'answerProgressHint'
                        }
                      >
                        <strong>{askProgressHint.label}</strong>
                        <span>{askProgressHint.detail}</span>
                      </div>
                    )}
                    {askPhase === 'failed' ? (
                      <div className="failureNotice compactFailure">
                        <strong>Answer failed</strong>
                        <span>{streamStatus || 'The request did not complete.'}</span>
                      </div>
                    ) : (
                      <div className={asking ? 'answerText answerTextStreaming' : 'answerText'}>
                        {visibleAskAnswer || askWaitingLabel(askPhase)}
                      </div>
                    )}
                    {askResult && (
                      <SourcePreviewLayout
                        emptyLabel="No sources"
                        hits={askResult.hits}
                        label="Sources"
                        onSelect={(hit) => setSelectedAskSourceKey(resultHitKey(hit))}
                        selectedHit={selectedAskSource}
                        selectedKey={selectedAskSourceKey}
                      />
                    )}
                  </div>
                )}
              </div>

              <aside className="workSurface contextPanel">
                <div className="surfaceHeader">
                  <h2>Context</h2>
                  <span>{askForm.document_id ? 'Scoped' : 'All documents'}</span>
                </div>

                <details
                  className="contextSourcePanel"
                  onToggle={(event) => {
                    const isOpen = event.currentTarget.open;
                    setContextSourceOpen(isOpen);
                    if (isOpen && !sourceForm.root_path.trim() && quickSourceTemplates[0]) {
                      applySourceTemplate(quickSourceTemplates[0]);
                    }
                  }}
                  open={contextSourceOpen}
                >
                  <summary>
                    <span>
                      <strong>Add data</strong>
                      <em>Folders, drives, exports</em>
                    </span>
                    <small>
                      {folderImporting
                        ? folderImportProgress || 'Importing'
                        : sourceConnectNotice ||
                          (readiness?.source_host_configured
                            ? sourceContainerRoot
                            : 'Import or connect')}
                    </small>
                  </summary>
                  <div className="contextSourceBody">
                    <form className="folderImportForm" onSubmit={submitFolderImport}>
                      <input
                        aria-label="Import folder"
                        accept={supportedDocumentAccept}
                        className="fileInput"
                        disabled={folderImporting || !canUploadDocuments}
                        multiple
                        ref={folderInputRef}
                        type="file"
                        onChange={(event) =>
                          setFolderFiles(Array.from(event.target.files ?? []))
                        }
                      />
                      <button
                        className="folderPicker"
                        onClick={() => folderInputRef.current?.click()}
                        type="button"
                      >
                        <strong>Import folder</strong>
                        <span>
                          {folderFiles.length > 0
                            ? `${folderFiles.length} selected`
                            : 'One-time upload'}
                        </span>
                      </button>
                      <button
                        disabled={
                          folderImporting ||
                          folderFiles.length === 0 ||
                          !workspaceReady ||
                          !canUploadDocuments
                        }
                        type="submit"
                      >
                        {folderImporting ? 'Importing' : 'Import'}
                      </button>
                    </form>

                    {folderImportProgress && (
                      <div className="sourceConnectNotice">
                        <span className="spinner" aria-hidden="true"></span>
                        <strong>{folderImportProgress}</strong>
                      </div>
                    )}

                    <div
                      className={
                        readiness?.source_host_configured
                          ? 'sourceMountStatus'
                          : 'sourceMountStatus sourceMountStatusWarning'
                      }
                    >
                      <strong>
                        {readiness?.source_host_configured
                          ? 'Mounted source ready'
                          : 'No mounted source root'}
                      </strong>
                      <span>
                        {readiness?.source_host_configured
                          ? `${sourceContainerRoot} is available to source scans.`
                          : 'Use folder import now, or choose a source folder in setup for repeat scans.'}
                      </span>
                    </div>

                    <div className="contextSourceQuick">
                      {quickSourceTemplates.map((template) => (
                        <button
                          key={template.id}
                          onClick={() => applySourceTemplate(template)}
                          type="button"
                        >
                          <strong>{template.label}</strong>
                          <span>{template.detail}</span>
                        </button>
                      ))}
                    </div>

                    <form
                      className="contextSourceForm"
                      onSubmit={(event) =>
                        void submitDataSource(event, {
                          closePanel: true,
                          scanAfterCreate: true,
                        })
                      }
                    >
                      <label>
                        <span>Name</span>
                        <input
                          aria-label="Source name"
                          onChange={(event) =>
                            setSourceForm((current) => ({
                              ...current,
                              name: event.target.value,
                            }))
                          }
                          placeholder="Knowledge folder"
                          value={sourceForm.name}
                        />
                      </label>
                      <label className="contextSourcePath">
                        <span>Path</span>
                        <input
                          aria-label="Source path"
                          onChange={(event) =>
                            setSourceForm((current) => ({
                              ...current,
                              root_path: event.target.value,
                            }))
                          }
                          placeholder={sourceContainerRoot}
                          value={sourceForm.root_path}
                        />
                      </label>
                      <label>
                        <span>Schedule</span>
                        <select
                          aria-label="Source schedule"
                          onChange={(event) =>
                            setSourceForm((current) => ({
                              ...current,
                              scan_interval_minutes: event.target.value,
                            }))
                          }
                          value={sourceForm.scan_interval_minutes}
                        >
                          <option value="0">Manual</option>
                          <option value="60">Hourly</option>
                          <option value="1440">Daily</option>
                          <option value="10080">Weekly</option>
                        </select>
                      </label>
                      <details className="inlineDetails contextSourceAdvanced">
                        <summary>Advanced</summary>
                        <div className="contextAdvancedGrid">
                          <label>
                            <span>Type</span>
                            <select
                              aria-label="Source type"
                              onChange={(event) =>
                                setSourceForm((current) => ({
                                  ...current,
                                  type: event.target.value,
                                }))
                              }
                              value={sourceForm.type}
                            >
                              <option value="synced_folder">Synced folder</option>
                              <option value="folder">Local folder</option>
                              <option value="network_share">Network share</option>
                              <option value="export">Export</option>
                            </select>
                          </label>
                          <label>
                            <span>Include</span>
                            <textarea
                              aria-label="Source include patterns"
                              onChange={(event) =>
                                setSourceForm((current) => ({
                                  ...current,
                                  include_patterns: event.target.value,
                                }))
                              }
                              value={sourceForm.include_patterns}
                            />
                          </label>
                          <label>
                            <span>Exclude</span>
                            <textarea
                              aria-label="Source exclude patterns"
                              onChange={(event) =>
                                setSourceForm((current) => ({
                                  ...current,
                                  exclude_patterns: event.target.value,
                                }))
                              }
                              value={sourceForm.exclude_patterns}
                            />
                          </label>
                        </div>
                      </details>
                      <button
                        disabled={creatingSource || !workspaceReady || !canManageSources}
                        type="submit"
                      >
                        {creatingSource ? 'Connecting' : 'Connect + scan'}
                      </button>
                    </form>

                    {sourceConnectNotice && (
                      <div className="sourceConnectNotice">
                        <strong>{sourceConnectNotice}</strong>
                      </div>
                    )}
                  </div>
                </details>

                <form className="uploadBar compactUpload" onSubmit={submitUpload}>
                  <input
                    aria-label="Document"
                    accept={supportedDocumentAccept}
                    className="fileInput"
                    id="dashboard-document-upload"
                    disabled={!canUploadDocuments}
                    type="file"
                    onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                  />
                  <label className="filePicker" htmlFor="dashboard-document-upload">
                    <strong>{file ? 'Selected' : 'Choose file'}</strong>
                    <span>{file?.name ?? 'Markdown, text, PDF, CSV, JSON'}</span>
                  </label>
                  <button
                    disabled={submitting || uploadingSample || !workspaceReady || !canUploadDocuments}
                    onClick={() => void uploadSampleDocument()}
                    type="button"
                  >
                    {uploadingSample ? 'Adding' : 'Sample'}
                  </button>
                  <button
                    disabled={submitting || uploadingSample || !workspaceReady || !canUploadDocuments}
                    type="submit"
                  >
                    {submitting ? 'Uploading' : 'Upload'}
                  </button>
                </form>

                {registration && (
                  <div className="resultBand">
                    <span>{registration.document.name}</span>
                    <strong className={stateClass(registration.document.status)}>
                      {registration.document.status}
                    </strong>
                    <em className={stateClass(registration.job.state)}>{registration.job.state}</em>
                  </div>
                )}

                <div className="contextList">
                  {askForm.document_id && (
                    <button
                      className="contextDoc"
                      onClick={() =>
                        setAskForm((current) => ({
                          ...current,
                          document_id: '',
                        }))
                      }
                      type="button"
                    >
                      <strong>All documents</strong>
                      <span>source</span>
                    </button>
                  )}
                  {documents?.documents.map((document) => (
                    <button
                      className={
                        askForm.document_id === document.id
                          ? 'contextDoc contextDocActive'
                          : 'contextDoc'
                      }
                      key={document.id}
                      onClick={() =>
                        setAskForm((current) => ({
                          ...current,
                          document_id: document.id,
                        }))
                      }
                      type="button"
                    >
                      <strong>{document.name}</strong>
                      <span className={stateClass(document.status)}>{document.status}</span>
                    </button>
                  ))}
                  {documents && documents.documents.length === 0 && (
                    <p className="muted">No documents</p>
                  )}
                </div>
              </aside>
                </div>

                <div className="dashboardSecondary">
                  <section className="workSurface">
                    <div className="surfaceHeader">
                      <h2>Search</h2>
                      <button onClick={() => setActiveView('documents')} type="button">
                        Library
                      </button>
                    </div>
                    <form className="searchBar" onSubmit={submitSearch}>
                      <input
                        aria-label="Search query"
                        onChange={(event) =>
                          setSearchForm((current) => ({ ...current, query: event.target.value }))
                        }
                        placeholder="Search documents"
                        value={searchForm.query}
                      />
                      <DocumentSelect
                        documents={documents?.documents ?? []}
                        value={searchForm.document_id}
                        onChange={(value) =>
                          setSearchForm((current) => ({ ...current, document_id: value }))
                        }
                      />
                      <button disabled={searching || !workspaceReady} type="submit">
                        {searching ? 'Searching' : 'Search'}
                      </button>
                    </form>
                    {searchResult && (
                      <SourcePreviewLayout
                        emptyLabel="No matches"
                        hits={searchResult.hits}
                        label="Matches"
                        onSelect={(hit) => setSelectedSearchSourceKey(resultHitKey(hit))}
                        selectedHit={selectedSearchSource}
                        selectedKey={selectedSearchSourceKey}
                      />
                    )}
                  </section>

                  <section className="workSurface">
                    <div className="surfaceHeader">
                      <h2>Recent</h2>
                      <span>{documents?.documents.length ?? 0} documents</span>
                    </div>
                    <div className="dashboardList">
                      {documents?.documents.slice(0, 3).map((document) => (
                        <button
                          className="contextDoc"
                          key={document.id}
                          onClick={() => {
                            setActiveView('documents');
                            void openDocument(document);
                          }}
                          type="button"
                        >
                          <strong>{document.name}</strong>
                          <span className={stateClass(document.status)}>{document.status}</span>
                        </button>
                      ))}
                      {conversations?.conversations.slice(0, 3).map((conversation) => (
                        <button
                          className="contextDoc"
                          key={conversation.id}
                          onClick={() => {
                            setActiveView('ask');
                            void openConversation(conversation);
                          }}
                          type="button"
                        >
                          <strong>{conversation.title || conversation.id}</strong>
                          <span>{formatDateTime(conversation.updated_at)}</span>
                        </button>
                      ))}
                      {documents &&
                        conversations &&
                        documents.documents.length === 0 &&
                        conversations.conversations.length === 0 && (
                          <p className="muted">No recent work</p>
                        )}
                    </div>
                  </section>
                </div>
              </div>
            ) : (
              <section className="workSurface dashboardStart">
                <div className="surfaceHeader">
                  <h2>Create workspace</h2>
                  <span>{currentUser?.user.email ?? 'dev@example.local'}</span>
                </div>
                <form className="inlineForm dashboardStartForm" onSubmit={submitTenant}>
                  <input
                    aria-label="Workspace name"
                    onChange={(event) => setTenantName(event.target.value)}
                    value={tenantName}
                  />
                  <div className="dashboardStartActions">
                    <button disabled={creatingTenant} type="submit">
                      {creatingTenant && !sampleFlowActive ? 'Creating' : 'Create'}
                    </button>
                    <button
                      className="secondaryButton"
                      disabled={creatingTenant || uploadingSample}
                      onClick={() => void createWorkspace(true)}
                      type="button"
                    >
                      {sampleFlowActive ? sampleFlowButtonLabel(sampleFlowState) : 'Create + sample'}
                    </button>
                  </div>
                </form>
              </section>
            )
          )}

        {activeView === 'documents' && (
          <div className="libraryStack">
            <section className="workSurface sourceSurface">
              <div className="surfaceHeader">
                <h2>Sources</h2>
                <div className="headerActions">
                  <span className="syncStatus">
                    {sourceListCountLabel(dataSources?.sources.length ?? 0, filteredSources.length)}
                  </span>
                  <button
                    onClick={() =>
                      void Promise.all([
                        refreshDataSources(),
                        refreshSourceViews(),
                        refreshSourcePolicyProfiles(),
                        refreshJobs(),
                      ])
                    }
                    type="button"
                  >
                    Refresh
                  </button>
                </div>
              </div>

              <details className="inlineDetails">
                <summary>Add source</summary>
                <form className="sourceForm" onSubmit={submitDataSource}>
                  <details className="sourceTemplatePicker">
                    <summary>Templates</summary>
                    <div className="sourceTemplateGrid">
                      {sourceTemplates.map((template) => (
                        <button
                          key={template.id}
                          onClick={() => applySourceTemplate(template)}
                          type="button"
                        >
                          <strong>{template.label}</strong>
                          <span>{sourceTypeLabel(template.type)}</span>
                          <small>{template.detail}</small>
                        </button>
                      ))}
                    </div>
                  </details>
                  <details className="sourcePolicyPicker">
                    <summary>Policies</summary>
                    <div className="sourcePolicySave">
                      <input
                        aria-label="Policy profile name"
                        onChange={(event) => setSourcePolicyName(event.target.value)}
                        placeholder="Policy name"
                        value={sourcePolicyName}
                      />
                      <input
                        aria-label="Policy profile note"
                        onChange={(event) => setSourcePolicyDetail(event.target.value)}
                        placeholder="Note"
                        value={sourcePolicyDetail}
                      />
                      <button
                        disabled={
                          !canManageSources ||
                          !tenantID ||
                          !sourcePolicyName.trim() ||
                          savingSourcePolicyProfile
                        }
                        onClick={() => void saveSourcePolicyProfile(sourceForm)}
                        type="button"
                      >
                        {savingSourcePolicyProfile ? 'Saving' : 'Save'}
                      </button>
                      {sourcePolicyNotice && <span className="syncStatus">{sourcePolicyNotice}</span>}
                    </div>
                    <div className="sourcePolicyGrid">
                      {sourcePolicyProfiles.map((profile) =>
                        renderSourcePolicyProfile(profile, applySourcePolicy, !canManageSources),
                      )}
                    </div>
                  </details>
                  <input
                    aria-label="Source name"
                    onChange={(event) =>
                      setSourceForm((current) => ({ ...current, name: event.target.value }))
                    }
                    placeholder="Source name"
                    value={sourceForm.name}
                  />
                  <select
                    aria-label="Source type"
                    onChange={(event) =>
                      setSourceForm((current) => ({ ...current, type: event.target.value }))
                    }
                    value={sourceForm.type}
                  >
                    <option value="synced_folder">Synced folder</option>
                    <option value="folder">Local folder</option>
                    <option value="network_share">Network share</option>
                    <option value="export">Export</option>
                    <option value="connector">Connector</option>
                  </select>
                  <input
                    aria-label="Source path"
                    onChange={(event) =>
                      setSourceForm((current) => ({ ...current, root_path: event.target.value }))
                    }
                    placeholder={sourcePathPlaceholder(sourceForm.type)}
                    value={sourceForm.root_path}
                  />
                  <select
                    aria-label="Source schedule"
                    onChange={(event) =>
                      setSourceForm((current) => ({
                        ...current,
                        scan_interval_minutes: event.target.value,
                      }))
                    }
                    value={sourceForm.scan_interval_minutes}
                  >
                    <option value="0">Manual</option>
                    <option value="15">15 min</option>
                    <option value="60">Hourly</option>
                    <option value="1440">Daily</option>
                    <option value="10080">Weekly</option>
                  </select>
                  <details className="sourceMountHelper">
                    <summary>Path helper</summary>
                    <div className="sourceMountPanel">
                      <strong>{sourceMountTitle(sourceForm.type)}</strong>
                      <span>{sourceMountDescription(sourceForm.type)}</span>
                      <div className="sourcePathChips">
                        {sourcePathExamples(sourceForm.type).map((path) => (
                          <button
                            key={path}
                            onClick={() =>
                              setSourceForm((current) => ({ ...current, root_path: path }))
                            }
                            type="button"
                          >
                            {path}
                          </button>
                        ))}
                      </div>
                    </div>
                  </details>
                  <SourceConnectorFields
                    form={sourceForm}
                    onChange={(updates) =>
                      setSourceForm((current) => ({ ...current, ...updates }))
                    }
                  />
                  <div className="sourcePatternFields">
                    <label>
                      <span>Include</span>
                      <textarea
                        aria-label="Source include patterns"
                        onChange={(event) =>
                          setSourceForm((current) => ({
                            ...current,
                            include_patterns: event.target.value,
                          }))
                        }
                        placeholder={'**/*.md\ncases/**'}
                        value={sourceForm.include_patterns}
                      />
                    </label>
                    <label>
                      <span>Exclude</span>
                      <textarea
                        aria-label="Source exclude patterns"
                        onChange={(event) =>
                          setSourceForm((current) => ({
                            ...current,
                            exclude_patterns: event.target.value,
                          }))
                        }
                        placeholder={'archive/**\n*.draft.md'}
                        value={sourceForm.exclude_patterns}
                      />
                    </label>
                  </div>
                  <button
                    disabled={creatingSource || !workspaceReady || !canManageSources}
                    type="submit"
                  >
                    {creatingSource ? 'Adding' : 'Add'}
                  </button>
                </form>
              </details>

              <div className="sourceFilterBar">
                <input
                  aria-label="Filter sources"
                  onChange={(event) =>
                    applySourceFilters({ ...sourceFilters, query: event.target.value })
                  }
                  placeholder="Filter sources"
                  value={sourceFilters.query}
                />
                <details className="menuPanel compactMenu sourceViewMenu">
                  <summary>{activeSourceSavedView ? sourceSavedViewLabel(activeSourceSavedView) : 'Views'}</summary>
                  <div className="menuFields sourceViewFields">
                    <div className="sourcePresetGrid">
                      {sourcePresetViews.map((view) => (
                        <button
                          className={sourceFiltersEqual(view.filters, sourceFilters) ? 'selected' : ''}
                          key={view.id}
                          onClick={() => applySourceFilters(view.filters)}
                          type="button"
                        >
                          {view.label}
                        </button>
                      ))}
                    </div>
                    <div className="sourceViewSave">
                      <input
                        aria-label="Source view name"
                        onChange={(event) => setSourceViewName(event.target.value)}
                        placeholder="View name"
                        value={sourceViewName}
                      />
                      <button
                        disabled={
                          !canManageSources ||
                          !sourceFiltersActive ||
                          !sourceViewName.trim() ||
                          savingSourceView
                        }
                        onClick={() => void saveSourceView()}
                        type="button"
                      >
                        {savingSourceView ? 'Saving' : 'Save'}
                      </button>
                    </div>
                    {sourceSavedViews.length > 0 && (
                      <div className="sourceSavedViewList">
                        {sourceSavedViews.map((view) => (
                          <div className="sourceSavedViewRow" key={view.id}>
                            <button
                              className={sourceFiltersEqual(view.filters, sourceFilters) ? 'selected' : ''}
                              onClick={() => applySourceFilters(view.filters)}
                              type="button"
                            >
                              {sourceSavedViewLabel(view)}
                            </button>
                            <button
                              aria-label={`Remove ${sourceSavedViewLabel(view)}`}
                              disabled={!canManageSources || deletingSourceViewID === view.id}
                              onClick={() => void removeSourceView(view.id)}
                              type="button"
                            >
                              {deletingSourceViewID === view.id ? 'Removing' : 'Remove'}
                            </button>
                          </div>
                        ))}
                      </div>
                    )}
                    {sourceViewNotice && <span className="syncStatus">{sourceViewNotice}</span>}
                  </div>
                </details>
                <details className="menuPanel compactMenu sourceFilterMenu">
                  <summary>{sourceFiltersActive ? 'Filters on' : 'Filters'}</summary>
                  <div className="menuFields">
                    <label>
                      Health
                      <select
                        aria-label="Source health filter"
                        onChange={(event) =>
                          applySourceFilters({
                            ...sourceFilters,
                            health: event.target.value,
                          })
                        }
                        value={sourceFilters.health}
                      >
                        <option value="">Any health</option>
                        <option value="active">Active</option>
                        <option value="blocked">Blocked</option>
                        <option value="review">Review</option>
                        <option value="healthy">Healthy</option>
                        <option value="archived">Archived</option>
                      </select>
                    </label>
                    <label>
                      Type
                      <select
                        aria-label="Source type filter"
                        onChange={(event) =>
                          applySourceFilters({
                            ...sourceFilters,
                            type: event.target.value,
                          })
                        }
                        value={sourceFilters.type}
                      >
                        <option value="">Any type</option>
                        <option value="synced_folder">Synced folder</option>
                        <option value="folder">Local folder</option>
                        <option value="network_share">Network share</option>
                        <option value="export">Export</option>
                        <option value="connector">Connector</option>
                      </select>
                    </label>
                    <label>
                      Schedule
                      <select
                        aria-label="Source schedule filter"
                        onChange={(event) =>
                          applySourceFilters({
                            ...sourceFilters,
                            schedule: event.target.value,
                          })
                        }
                        value={sourceFilters.schedule}
                      >
                        <option value="">Any schedule</option>
                        <option value="manual">Manual</option>
                        <option value="scheduled">Scheduled</option>
                        <option value="overdue">Overdue</option>
                      </select>
                    </label>
                    <button
                      disabled={!sourceFiltersActive}
                      onClick={() => applySourceFilters(initialSourceFilters)}
                      type="button"
                    >
                      Clear
                    </button>
                  </div>
                </details>
                <details className="menuPanel compactMenu sourceBulkMenu">
                  <summary>{sourceBulkAction ? sourceBulkActionVerb(sourceBulkAction) : 'Actions'}</summary>
                  <div className="menuFields sourceBulkFields">
                    <div className="sourceBulkSummary">
                      <strong>{sourceListCountLabel(dataSources?.sources.length ?? 0, filteredSources.length)}</strong>
                      <span>Uses the current source filters.</span>
                    </div>
                    {(
                      ['preflight', 'plan', 'scan', 'retry_failures', 'reindex'] as SourceBulkAction[]
                    ).map((action) => (
                      <button
                        disabled={
                          !canImportSources ||
                          sourceBulkAction !== '' ||
                          sourceBulkActionCounts[action] === 0
                        }
                        key={action}
                        onClick={() => void runSourceBulkAction(action)}
                        type="button"
                      >
                        <span>
                          {sourceBulkAction === action
                            ? 'Working'
                            : sourceBulkActionButtonLabel(action, sourceBulkActionCounts[action])}
                        </span>
                      </button>
                    ))}
                  </div>
                </details>
                {sourceBulkResult && <span className="syncStatus">{sourceBulkResult}</span>}
              </div>

              <div className="tableList sourceList">
                {filteredSources.map((source) => {
                  const connectorSource = sourceIsConnector(source);
                  const scanJob = activeSourceScanJobs.get(source.id);
                  const preflightJob = activeSourcePreflightJobs.get(source.id);
                  const planJob = activeSourcePlanJobs.get(source.id);
                  const latestPlanJob = latestSourcePlanJobs.get(source.id);
                  const planBlocksScan = sourcePlanBlocksScan(source, latestPlanJob);
                  const latestPreflightJob = latestSourcePreflightJobs.get(source.id);
                  const preflightBlocksScan = sourcePreflightBlocksScan(
                    source,
                    latestPreflightJob,
                  );
                  const visiblePreflightJob =
                    connectorSource
                      ? undefined
                      : preflightJob ?? (preflightBlocksScan ? latestPreflightJob : undefined);
                  const visiblePlanJob =
                    connectorSource
                      ? undefined
                      : planJob ?? (latestPlanJob?.state === 'failed' ? latestPlanJob : undefined);
                  return (
                    <div
                      className={
                        sourceDetail?.source.id === source.id ? 'sourceRow selectedRow' : 'sourceRow'
                      }
                      key={source.id}
                    >
                      <button onClick={() => void openSource(source)} type="button">
                        <strong>{source.name}</strong>
                        <small>
                          {sourceListHealthLabel(
                            source,
                            visiblePreflightJob,
                            visiblePlanJob,
                            scanJob,
                          )}
                        </small>
                      </button>
                      <span
                        className={stateClass(
                          visiblePreflightJob?.state ??
                            visiblePlanJob?.state ??
                            scanJob?.state ??
                            source.status,
                        )}
                      >
                        {visiblePreflightJob
                          ? `check ${visiblePreflightJob.state}`
                          : visiblePlanJob
                            ? `plan ${visiblePlanJob.state}`
                          : scanJob
                            ? `scan ${scanJob.state}`
                            : connectorSource
                              ? 'handoff'
                            : source.status}
                      </span>
                      <em>{sourceTypeLabel(source.type)}</em>
                      <small title={source.root_path}>{sourceScanSummary(source)}</small>
                      <details className="rowMenu">
                        <summary>More</summary>
                        <div className="rowMenuActions">
                          <button
                            disabled={loadingSourceID === source.id}
                            onClick={() => void openSource(source)}
                            type="button"
                          >
                            {loadingSourceID === source.id ? 'Loading' : 'Details'}
                          </button>
                          <button
                            disabled={
                              !canImportSources ||
                              connectorSource ||
                              Boolean(preflightJob) ||
                              preflightingSourceID === source.id ||
                              source.status === 'archived' ||
                              archivingSourceID === source.id
                            }
                            onClick={() => void requestDataSourcePreflight(source)}
                            type="button"
                          >
                            {connectorSource
                              ? 'Handoff'
                              : preflightingSourceID === source.id
                              ? 'Queuing'
                              : preflightJob
                                ? titleCase(preflightJob.state)
                                : 'Check path'}
                          </button>
                          <button
                            disabled={
                              !canImportSources ||
                              connectorSource ||
                              Boolean(planJob) ||
                              Boolean(preflightJob) ||
                              preflightBlocksScan ||
                              planBlocksScan ||
                              Boolean(scanJob) ||
                              planningSourceID === source.id ||
                              source.status === 'archived' ||
                              archivingSourceID === source.id
                            }
                            onClick={() => void requestDataSourcePlan(source)}
                            type="button"
                          >
                            {connectorSource
                              ? 'No worker'
                              : planningSourceID === source.id
                              ? 'Queuing'
                              : planJob
                                ? titleCase(planJob.state)
                                : 'Plan'}
                          </button>
                          <button
                            disabled={
                              !canImportSources ||
                              connectorSource ||
                              Boolean(planJob) ||
                              Boolean(preflightJob) ||
                              preflightBlocksScan ||
                              Boolean(scanJob) ||
                              scanningSourceID === source.id ||
                              archivingSourceID === source.id
                            }
                            onClick={() => void requestDataSourceScan(source)}
                            type="button"
                          >
                            {connectorSource
                              ? 'No worker'
                              : scanningSourceID === source.id
                              ? 'Queuing'
                              : preflightBlocksScan
                                ? 'Path blocked'
                              : planBlocksScan
                                ? 'Plan failed'
                              : scanJob
                                ? titleCase(scanJob.state)
                                : 'Rescan'}
                          </button>
                          <button
                            disabled={
                              !canImportSources ||
                              connectorSource ||
                              Boolean(planJob) ||
                              Boolean(preflightJob) ||
                              Boolean(scanJob) ||
                              reindexingSourceID === source.id ||
                              source.status === 'archived' ||
                              archivingSourceID === source.id ||
                              deletingSourceDocumentsID === source.id
                            }
                            onClick={() => void requestDataSourceReindex(source)}
                            type="button"
                          >
                            {connectorSource
                              ? 'No docs'
                              : reindexingSourceID === source.id
                                ? 'Reindexing'
                                : 'Reindex'}
                          </button>
                          <button
                            className="dangerButton"
                            disabled={
                              !canManageSources ||
                              archivingSourceID === source.id ||
                              deletingSourceDocumentsID === source.id
                            }
                            onClick={() => void removeDataSource(source)}
                            type="button"
                          >
                            {archivingSourceID === source.id ? 'Archiving' : 'Archive'}
                          </button>
                        </div>
                      </details>
                    </div>
                  );
                })}
                {dataSources && dataSources.sources.length === 0 && (
                  <p className="muted">No sources</p>
                )}
                {dataSources && dataSources.sources.length > 0 && filteredSources.length === 0 && (
                  <p className="muted">No sources match these filters</p>
                )}
                {!dataSources && <p className="muted">Loading sources</p>}
              </div>

              {sourceDetail && (
                <div className="detailPanel">
                  <div className="detailHeader">
                    <h3>{sourceDetail.source.name}</h3>
                    <div className="detailActions">
                      <span
                        className={stateClass(
                          activeSourcePreflightJobs.get(sourceDetail.source.id)?.state ??
                            activeSourcePlanJobs.get(sourceDetail.source.id)?.state ??
                            activeSourceScanJobs.get(sourceDetail.source.id)?.state ??
                            sourceDetail.source.status,
                        )}
                      >
                        {activeSourcePreflightJobs.get(sourceDetail.source.id)
                          ? `check ${activeSourcePreflightJobs.get(sourceDetail.source.id)?.state}`
                          : activeSourcePlanJobs.get(sourceDetail.source.id)
                            ? `plan ${activeSourcePlanJobs.get(sourceDetail.source.id)?.state}`
                          : activeSourceScanJobs.get(sourceDetail.source.id)
                            ? `scan ${activeSourceScanJobs.get(sourceDetail.source.id)?.state}`
                            : sourceIsConnector(sourceDetail.source)
                              ? 'handoff'
                            : sourceDetail.source.status}
                      </span>
                      <button
                        disabled={refreshingSourceID === sourceDetail.source.id}
                        onClick={() => void refreshSourceDetail(sourceDetail.source.id)}
                        type="button"
                      >
                        {refreshingSourceID === sourceDetail.source.id ? 'Refreshing' : 'Refresh'}
                      </button>
                      <button
                        disabled={
                          !canImportSources ||
                          sourceIsConnector(sourceDetail.source) ||
                          Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                          sourceHasActivePreflightJob(sourceDetail) ||
                          sourceDetail.source.status === 'archived' ||
                          preflightingSourceID === sourceDetail.source.id
                        }
                        onClick={() => void requestDataSourcePreflight(sourceDetail.source)}
                        type="button"
                      >
                        {sourceIsConnector(sourceDetail.source)
                          ? 'Handoff'
                          : preflightingSourceID === sourceDetail.source.id
                          ? 'Queuing'
                          : sourceHasActivePreflightJob(sourceDetail)
                            ? 'Queued'
                          : 'Check path'}
                      </button>
                      <button
                        disabled={
                          !canImportSources ||
                          sourceIsConnector(sourceDetail.source) ||
                          Boolean(activeSourcePlanJobs.get(sourceDetail.source.id)) ||
                          sourceHasActivePlanJob(sourceDetail) ||
                          sourceScanBlockedByPlan(sourceDetail) ||
                          Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                          sourceHasActivePreflightJob(sourceDetail) ||
                          sourceScanBlockedByPreflight(sourceDetail) ||
                          Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                          sourceHasActiveScanJob(sourceDetail) ||
                          sourceDetail.source.status === 'archived' ||
                          planningSourceID === sourceDetail.source.id
                        }
                        onClick={() => void requestDataSourcePlan(sourceDetail.source)}
                        type="button"
                      >
                        {sourceIsConnector(sourceDetail.source)
                          ? 'No worker'
                          : planningSourceID === sourceDetail.source.id
                          ? 'Queuing'
                          : sourceHasActivePlanJob(sourceDetail)
                            ? 'Queued'
                            : 'Plan'}
                      </button>
                      <button
                        disabled={
                          !canImportSources ||
                          sourceIsConnector(sourceDetail.source) ||
                          Boolean(activeSourcePlanJobs.get(sourceDetail.source.id)) ||
                          sourceHasActivePlanJob(sourceDetail) ||
                          Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                          sourceHasActivePreflightJob(sourceDetail) ||
                          sourceScanBlockedByPreflight(sourceDetail) ||
                          Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                          sourceHasActiveScanJob(sourceDetail) ||
                          sourceDetail.source.status === 'archived' ||
                          scanningSourceID === sourceDetail.source.id
                        }
                        onClick={() => void requestDataSourceScan(sourceDetail.source)}
                        type="button"
                      >
                        {sourceIsConnector(sourceDetail.source)
                          ? 'No worker'
                          : scanningSourceID === sourceDetail.source.id
                          ? 'Queuing'
                          : sourceScanBlockedByPlan(sourceDetail)
                            ? 'Plan failed'
                          : sourceScanBlockedByPreflight(sourceDetail)
                            ? 'Path blocked'
                          : sourceHasActiveScanJob(sourceDetail)
                            ? 'Queued'
                            : sourceDetail.source.status === 'failed'
                              ? 'Retry'
                              : 'Rescan'}
                      </button>
                      <details className="rowMenu detailMenu">
                        <summary>More</summary>
                        <div className="rowMenuActions">
                          <button
                            disabled={
                              !canImportSources ||
                              sourceIsConnector(sourceDetail.source) ||
                              reindexingSourceID === sourceDetail.source.id ||
                              sourceDetail.source.status === 'archived' ||
                              Boolean(activeSourcePlanJobs.get(sourceDetail.source.id)) ||
                              sourceHasActivePlanJob(sourceDetail) ||
                              Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                              sourceHasActivePreflightJob(sourceDetail) ||
                              Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                              sourceHasActiveScanJob(sourceDetail)
                            }
                            onClick={() => void requestDataSourceReindex(sourceDetail.source)}
                            type="button"
                          >
                            {sourceIsConnector(sourceDetail.source)
                              ? 'No docs'
                              : reindexingSourceID === sourceDetail.source.id
                              ? 'Reindexing'
                              : 'Reindex'}
                          </button>
                          <button
                            disabled={
                              !canImportSources ||
                              sourceIsConnector(sourceDetail.source) ||
                              retryingSourceFailuresID === sourceDetail.source.id ||
                              sourceDetail.source.status === 'archived' ||
                              Boolean(activeSourcePlanJobs.get(sourceDetail.source.id)) ||
                              sourceHasActivePlanJob(sourceDetail) ||
                              Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                              sourceHasActivePreflightJob(sourceDetail) ||
                              Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                              sourceHasActiveScanJob(sourceDetail) ||
                              sourceDetail.failed_documents === 0
                            }
                            onClick={() =>
                              void requestRetryFailedSourceDocuments(sourceDetail.source)
                            }
                            type="button"
                          >
                            {retryingSourceFailuresID === sourceDetail.source.id
                              ? 'Retrying'
                              : 'Retry failed docs'}
                          </button>
                          <button
                            className="dangerButton"
                            disabled={
                              !canManageSources ||
                              sourceDetail.source.status === 'archived' ||
                              archivingSourceID === sourceDetail.source.id ||
                              deletingSourceDocumentsID === sourceDetail.source.id
                            }
                            onClick={() => void removeDataSource(sourceDetail.source)}
                            type="button"
                          >
                            {archivingSourceID === sourceDetail.source.id ? 'Archiving' : 'Archive'}
                          </button>
                          <button
                            className="dangerButton"
                            disabled={
                              !canManageSources ||
                              sourceIsConnector(sourceDetail.source) ||
                              deletingSourceDocumentsID === sourceDetail.source.id ||
                              Boolean(activeSourcePlanJobs.get(sourceDetail.source.id)) ||
                              sourceHasActivePlanJob(sourceDetail) ||
                              Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                              sourceHasActivePreflightJob(sourceDetail) ||
                              Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                              sourceHasActiveScanJob(sourceDetail)
                            }
                            onClick={() => void removeDataSource(sourceDetail.source, true)}
                            type="button"
                          >
                            {deletingSourceDocumentsID === sourceDetail.source.id
                              ? 'Deleting'
                              : 'Delete docs'}
                          </button>
                        </div>
                      </details>
                    </div>
                  </div>
                  {sourceDetail.source.status === 'archived' && (
                    <div className="infoNotice">
                      <strong>Archived source</strong>
                      <span>Existing documents remain available.</span>
                    </div>
                  )}
                  {sourceIsConnector(sourceDetail.source) && (
                    <div className="infoNotice">
                      <strong>Connector handoff</strong>
                      <span>
                        Direct connector workers are not enabled yet. This record keeps provider,
                        resource, and credential references ready for deployment handoff.
                      </span>
                    </div>
                  )}
                  {sourceDetail.source.status === 'failed' && sourceFailureMessage(sourceDetail) && (
                    <div className="failureNotice">
                      <strong>Scan failed</strong>
                      <span>{sourceFailureMessage(sourceDetail)}</span>
                    </div>
                  )}
                  {!sourceIsConnector(sourceDetail.source) && (
                    <>
                      <SourceHealthRollup
                        activePlanJob={activeSourcePlanJobs.get(sourceDetail.source.id)}
                        activePreflightJob={activeSourcePreflightJobs.get(sourceDetail.source.id)}
                        activeScanJob={activeSourceScanJobs.get(sourceDetail.source.id)}
                        detail={sourceDetail}
                      />
                      <SourcePreflightStatus
                        activeJob={activeSourcePreflightJobs.get(sourceDetail.source.id)}
                        checking={preflightingSourceID === sourceDetail.source.id}
                        checkDisabled={sourceDetail.source.status === 'archived'}
                        detail={sourceDetail}
                        onCheck={() => void requestDataSourcePreflight(sourceDetail.source)}
                        onRefresh={() => void refreshSourceDetail(sourceDetail.source.id)}
                        refreshing={refreshingSourceID === sourceDetail.source.id}
                      />
                      <SourcePlanStatus
                        activeJob={activeSourcePlanJobs.get(sourceDetail.source.id)}
                        detail={sourceDetail}
                        onRefresh={() => void refreshSourceDetail(sourceDetail.source.id)}
                        refreshing={refreshingSourceID === sourceDetail.source.id}
                      />
                      <SourceScanRunStatus
                        activeJob={activeSourceScanJobs.get(sourceDetail.source.id)}
                        cancelDisabled={!canImportSources}
                        canceling={cancelingSourceScanID === sourceDetail.source.id}
                        detail={sourceDetail}
                        onCancel={() => void cancelSourceScan(sourceDetail.source)}
                        onRefresh={() => void refreshSourceDetail(sourceDetail.source.id)}
                        refreshing={refreshingSourceID === sourceDetail.source.id}
                      />
                      <SourceScanRunHistory detail={sourceDetail} />
                    </>
                  )}
                  <dl className="runtimeList detailList">
                    <div>
                      <dt>ID</dt>
                      <dd>{sourceDetail.source.id}</dd>
                    </div>
                    <div>
                      <dt>Type</dt>
                      <dd>{sourceTypeLabel(sourceDetail.source.type)}</dd>
                    </div>
                    <div>
                      <dt>Path</dt>
                      <dd>{sourceDetail.source.root_path}</dd>
                    </div>
                    {sourceConnectorConfigured(sourceDetail.source.connector_config) && (
                      <>
                        <div>
                          <dt>Connector</dt>
                          <dd>{sourceDetail.source.connector_config.provider || 'Configured'}</dd>
                        </div>
                        <div>
                          <dt>Resource</dt>
                          <dd>{sourceDetail.source.connector_config.resource_id || 'Not set'}</dd>
                        </div>
                        <div>
                          <dt>Credential</dt>
                          <dd>
                            {sourceDetail.source.connector_config.credential_ref || 'Not set'}
                          </dd>
                        </div>
                        {sourceDetail.source.connector_config.notes && (
                          <div>
                            <dt>Notes</dt>
                            <dd>{sourceDetail.source.connector_config.notes}</dd>
                          </div>
                        )}
                      </>
                    )}
                    {!sourceIsConnector(sourceDetail.source) && (
                      <>
                        <div>
                          <dt>Last scan</dt>
                          <dd>
                            {sourceDetail.source.last_scan_at
                              ? formatDateTime(sourceDetail.source.last_scan_at)
                              : 'Not scanned'}
                          </dd>
                        </div>
                        <div>
                          <dt>Schedule</dt>
                          <dd>{sourceScheduleLabel(sourceDetail.source.scan_interval_minutes)}</dd>
                        </div>
                        <div>
                          <dt>Next scan</dt>
                          <dd>
                            {sourceDetail.source.next_scan_at
                              ? formatDateTime(sourceDetail.source.next_scan_at)
                              : 'Manual'}
                          </dd>
                        </div>
                        <div>
                          <dt>Imported</dt>
                          <dd>{sourceDetail.source.last_scan_imported ?? 0}</dd>
                        </div>
                        <div>
                          <dt>Skipped</dt>
                          <dd>{sourceDetail.source.last_scan_skipped ?? 0}</dd>
                        </div>
                        <div>
                          <dt>Failed</dt>
                          <dd>{sourceDetail.source.last_scan_failed ?? 0}</dd>
                        </div>
                        <div>
                          <dt>Failed docs</dt>
                          <dd>{sourceDetail.failed_documents}</dd>
                        </div>
                      </>
                    )}
                  </dl>
                  {sourceDetail.scan_summary.total > 0 && (
                    <section className="sourceScanReport" aria-label="Latest scan report">
                      <div className="sourceScanReportHeader">
                        <div className="sourceScanReportTitle">
                          <strong>{sourceScanHeadline(sourceDetail)}</strong>
                          <span>{sourceScanSubline(sourceDetail)}</span>
                        </div>
                        <button onClick={() => exportSourceScanEntries()} type="button">
                          CSV
                        </button>
                      </div>
                      <div
                        className="sourceScanMix"
                        aria-label={sourceScanMixLabel(sourceDetail.scan_summary)}
                      >
                        {sourceScanMixSegments(sourceDetail.scan_summary).map((segment) => (
                          <span
                            className={`scanMixSegment scanMix-${segment.key}`}
                            key={segment.key}
                            style={{ flexGrow: segment.count }}
                            title={`${segment.label}: ${segment.count}`}
                          />
                        ))}
                      </div>
                      <div className="sourceScanMetrics">
                        {sourceScanMetrics(sourceDetail.scan_summary).map((metric) => (
                          <button
                            className={
                              scanEntryFilter === metric.filter
                                ? `scanMetric scanMetric-${metric.key} scanMetricSelected`
                                : `scanMetric scanMetric-${metric.key}`
                            }
                            key={metric.key}
                            onClick={() => void loadSourceScanEntries(metric.filter, 0)}
                            type="button"
                          >
                            <strong>{metric.count}</strong>
                            <span>{metric.label}</span>
                          </button>
                        ))}
                      </div>
                      {sourceScanReasonRows(sourceDetail.scan_summary).length > 0 && (
                        <div className="scanReasonBreakdown">
                          {sourceScanReasonRows(sourceDetail.scan_summary).map((row) => (
                            <button
                              className="scanReasonRow"
                              key={row.reason}
                              onClick={() => void loadSourceScanEntries(row.filter, 0)}
                              type="button"
                            >
                              <strong>{row.label}</strong>
                              <span>{row.count}</span>
                              <em>{titleCase(row.filter)}</em>
                            </button>
                          ))}
                        </div>
                      )}
                    </section>
                  )}
                  {sourceNeedsRecovery(sourceDetail) && (
                    <section className="sourceRecoveryPanel" aria-label="Source recovery">
                      <div className="sourceRecoveryText">
                        <strong>{sourceFailureTotal(sourceDetail)} failed</strong>
                        <span>{sourceRecoveryMessage(sourceDetail)}</span>
                      </div>
                      <div className="sourceRecoveryActions">
                        <button
                          onClick={() => void loadSourceScanEntries('failed', 0)}
                          type="button"
                        >
                          Show failed
                        </button>
                        <button onClick={() => exportSourceScanEntries('failed')} type="button">
                          Failed CSV
                        </button>
                        <button
                          disabled={
                            !canImportSources ||
                            Boolean(activeSourcePlanJobs.get(sourceDetail.source.id)) ||
                            sourceHasActivePlanJob(sourceDetail) ||
                            sourceScanBlockedByPlan(sourceDetail) ||
                            Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                            sourceHasActivePreflightJob(sourceDetail) ||
                            sourceScanBlockedByPreflight(sourceDetail) ||
                            Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                            sourceHasActiveScanJob(sourceDetail) ||
                            sourceDetail.source.status === 'archived' ||
                            scanningSourceID === sourceDetail.source.id
                          }
                          onClick={() => void requestDataSourceScan(sourceDetail.source)}
                          type="button"
                        >
                          {scanningSourceID === sourceDetail.source.id
                            ? 'Queuing'
                            : sourceScanBlockedByPlan(sourceDetail)
                              ? 'Plan failed'
                            : sourceScanBlockedByPreflight(sourceDetail)
                              ? 'Path blocked'
                            : sourceHasActiveScanJob(sourceDetail)
                              ? 'Queued'
                              : 'Retry scan'}
                        </button>
                      </div>
                    </section>
                  )}
                  {sourceDetail.failed_documents > 0 && (
                    <section className="sourceDocumentRecoveryPanel" aria-label="Document recovery">
                      <div className="sourceRecoveryText">
                        <strong>{sourceDetail.failed_documents} failed documents</strong>
                        <span>Retry imported files whose ingestion failed after the source scan.</span>
                      </div>
                      <div className="sourceRecoveryActions">
                        <button
                          disabled={
                            !canImportSources ||
                            retryingSourceFailuresID === sourceDetail.source.id ||
                            sourceDetail.source.status === 'archived' ||
                            Boolean(activeSourcePlanJobs.get(sourceDetail.source.id)) ||
                            sourceHasActivePlanJob(sourceDetail) ||
                            Boolean(activeSourcePreflightJobs.get(sourceDetail.source.id)) ||
                            sourceHasActivePreflightJob(sourceDetail) ||
                            Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                            sourceHasActiveScanJob(sourceDetail)
                          }
                          onClick={() =>
                            void requestRetryFailedSourceDocuments(sourceDetail.source)
                          }
                          type="button"
                        >
                          {retryingSourceFailuresID === sourceDetail.source.id
                            ? 'Retrying'
                            : 'Retry failed docs'}
                        </button>
                      </div>
                    </section>
                  )}
                  <details className="inlineDetails">
                    <summary>Edit</summary>
                    <form className="sourceForm sourceEditForm" onSubmit={submitSourceUpdate}>
                      <details className="sourcePolicyPicker">
                        <summary>Policies</summary>
                        <div className="sourcePolicySave">
                          <input
                            aria-label="Edit policy profile name"
                            disabled={
                              !canManageSources ||
                              sourceDetail.source.status === 'archived' ||
                              savingSourceID === sourceDetail.source.id
                            }
                            onChange={(event) => setSourcePolicyName(event.target.value)}
                            placeholder="Policy name"
                            value={sourcePolicyName}
                          />
                          <input
                            aria-label="Edit policy profile note"
                            disabled={
                              !canManageSources ||
                              sourceDetail.source.status === 'archived' ||
                              savingSourceID === sourceDetail.source.id
                            }
                            onChange={(event) => setSourcePolicyDetail(event.target.value)}
                            placeholder="Note"
                            value={sourcePolicyDetail}
                          />
                          <button
                            disabled={
                              !canManageSources ||
                              !tenantID ||
                              !sourcePolicyName.trim() ||
                              savingSourcePolicyProfile ||
                              sourceDetail.source.status === 'archived' ||
                              savingSourceID === sourceDetail.source.id
                            }
                            onClick={() => void saveSourcePolicyProfile(sourceEditForm)}
                            type="button"
                          >
                            {savingSourcePolicyProfile ? 'Saving' : 'Save'}
                          </button>
                          {sourcePolicyNotice && <span className="syncStatus">{sourcePolicyNotice}</span>}
                        </div>
                        <div className="sourcePolicyGrid">
                          {sourcePolicyProfiles.map((profile) =>
                            renderSourcePolicyProfile(
                              profile,
                              applySourceEditPolicy,
                              sourceDetail.source.status === 'archived' ||
                                savingSourceID === sourceDetail.source.id ||
                                !canManageSources,
                            ),
                          )}
                        </div>
                      </details>
                      <input
                        aria-label="Edit source name"
                        disabled={
                          !canManageSources ||
                          sourceDetail.source.status === 'archived' ||
                          savingSourceID === sourceDetail.source.id
                        }
                        onChange={(event) =>
                          setSourceEditForm((current) => ({
                            ...current,
                            name: event.target.value,
                          }))
                        }
                        value={sourceEditForm.name}
                      />
                      <select
                        aria-label="Edit source type"
                        disabled={
                          !canManageSources ||
                          sourceDetail.source.status === 'archived' ||
                          savingSourceID === sourceDetail.source.id
                        }
                        onChange={(event) =>
                          setSourceEditForm((current) => ({
                            ...current,
                            type: event.target.value,
                          }))
                        }
                        value={sourceEditForm.type}
                      >
                        <option value="synced_folder">Synced folder</option>
                        <option value="folder">Local folder</option>
                        <option value="network_share">Network share</option>
                        <option value="export">Export</option>
                        <option value="connector">Connector</option>
                      </select>
                      <input
                        aria-label="Edit source path"
                        disabled={
                          !canManageSources ||
                          sourceDetail.source.status === 'archived' ||
                          savingSourceID === sourceDetail.source.id
                        }
                        onChange={(event) =>
                          setSourceEditForm((current) => ({
                            ...current,
                            root_path: event.target.value,
                          }))
                        }
                        value={sourceEditForm.root_path}
                      />
                      <select
                        aria-label="Edit source schedule"
                        disabled={
                          !canManageSources ||
                          sourceDetail.source.status === 'archived' ||
                          savingSourceID === sourceDetail.source.id
                        }
                        onChange={(event) =>
                          setSourceEditForm((current) => ({
                            ...current,
                            scan_interval_minutes: event.target.value,
                          }))
                        }
                        value={sourceEditForm.scan_interval_minutes}
                      >
                        <option value="0">Manual</option>
                        <option value="15">15 min</option>
                        <option value="60">Hourly</option>
                        <option value="1440">Daily</option>
                        <option value="10080">Weekly</option>
                      </select>
                      <SourceConnectorFields
                        disabled={
                          !canManageSources ||
                          sourceDetail.source.status === 'archived' ||
                          savingSourceID === sourceDetail.source.id
                        }
                        form={sourceEditForm}
                        onChange={(updates) =>
                          setSourceEditForm((current) => ({ ...current, ...updates }))
                        }
                      />
                      <div className="sourcePatternFields">
                        <label>
                          <span>Include</span>
                          <textarea
                            aria-label="Edit source include patterns"
                            disabled={
                              !canManageSources ||
                              sourceDetail.source.status === 'archived' ||
                              savingSourceID === sourceDetail.source.id
                            }
                            onChange={(event) =>
                              setSourceEditForm((current) => ({
                                ...current,
                                include_patterns: event.target.value,
                              }))
                            }
                            placeholder={'**/*.md\ncases/**'}
                            value={sourceEditForm.include_patterns}
                          />
                        </label>
                        <label>
                          <span>Exclude</span>
                          <textarea
                            aria-label="Edit source exclude patterns"
                            disabled={
                              !canManageSources ||
                              sourceDetail.source.status === 'archived' ||
                              savingSourceID === sourceDetail.source.id
                            }
                            onChange={(event) =>
                              setSourceEditForm((current) => ({
                                ...current,
                                exclude_patterns: event.target.value,
                              }))
                            }
                            placeholder={'archive/**\n*.draft.md'}
                            value={sourceEditForm.exclude_patterns}
                          />
                        </label>
                      </div>
                      <button
                        disabled={
                          !canManageSources ||
                          sourceDetail.source.status === 'archived' ||
                          savingSourceID === sourceDetail.source.id
                        }
                        type="submit"
                      >
                        {savingSourceID === sourceDetail.source.id ? 'Saving' : 'Save'}
                      </button>
                    </form>
                  </details>
                  <details className="inlineDetails" open={sourceDetail.scan_summary.total > 0}>
                    <summary>Files</summary>
                    <div className="scanEntryToolbar">
                      <span className="muted">
                        {scanEntryPageStart(sourceDetail.scan_entries_page)}-
                        {scanEntryPageEnd(
                          sourceDetail.scan_entries_page,
                          visibleSourceScanEntries.length,
                        )}{' '}
                        of {sourceDetail.scan_entries_page.total}
                      </span>
                      <div className="scanEntryFilters" role="group" aria-label="File outcome filters">
                        {sourceScanMetrics(sourceDetail.scan_summary).map((metric) => (
                          <button
                            className={
                              scanEntryFilter === metric.filter
                                ? 'scanEntryFilter scanEntryFilterActive'
                                : 'scanEntryFilter'
                            }
                            key={metric.key}
                            onClick={() => void loadSourceScanEntries(metric.filter, 0)}
                            type="button"
                          >
                            {metric.label}
                            <span>{metric.count}</span>
                          </button>
                        ))}
                      </div>
                      <div className="scanEntryPager">
                        <button onClick={() => exportSourceScanEntries()} type="button">
                          Export
                        </button>
                        <button
                          disabled={sourceDetail.scan_entries_page.offset === 0}
                          onClick={() =>
                            void loadSourceScanEntries(
                              scanEntryFilter,
                              sourceDetail.scan_entries_page.offset -
                                sourceDetail.scan_entries_page.limit,
                            )
                          }
                          type="button"
                        >
                          Prev
                        </button>
                        <button
                          disabled={!sourceDetail.scan_entries_page.has_more}
                          onClick={() =>
                            void loadSourceScanEntries(
                              scanEntryFilter,
                              sourceDetail.scan_entries_page.offset +
                                sourceDetail.scan_entries_page.limit,
                            )
                          }
                          type="button"
                        >
                          Next
                        </button>
                      </div>
                    </div>
                    <div className="tableList scanEntryList">
                      {visibleSourceScanEntries.map((entry) => (
                        <div className="scanEntryRow" key={`${entry.job_id}:${entry.path}`}>
                          <strong title={entry.path}>{entry.path}</strong>
                          <span className={scanOutcomeClass(entry)}>
                            {titleCase(entry.outcome)}
                          </span>
                          <em title={scanEntryMessage(entry)}>{scanEntryReason(entry)}</em>
                          <small>
                            {entry.size_bytes > 0
                              ? formatBytes(entry.size_bytes)
                              : entry.document_id || ''}
                          </small>
                          <time dateTime={entry.created_at}>{formatDateTime(entry.created_at)}</time>
                        </div>
                      ))}
                      {sourceDetail.scan_entries_page.total === 0 && (
                        <p className="muted">No file results yet</p>
                      )}
                      {sourceDetail.scan_entries_page.total > 0 &&
                        visibleSourceScanEntries.length === 0 && (
                          <p className="muted">No files match this filter</p>
                        )}
                    </div>
                  </details>
                  <details className="inlineDetails" open>
                    <summary>Activity</summary>
                    <div className="tableList">
                      {sourceDetail.jobs.map((job) => (
                        <div className="jobRow" key={job.id}>
                          <strong>{job.type}</strong>
                          <span className={stateClass(job.state)}>{job.state}</span>
                          <em className={job.error_message ? 'jobError' : ''} title={jobTitle(job)}>
                            {jobDetail(job)}
                          </em>
                          <small>{job.attempts} tries</small>
                          <time dateTime={job.updated_at}>{formatDateTime(job.updated_at)}</time>
                        </div>
                      ))}
                      {sourceDetail.jobs.length === 0 && <p className="muted">No activity</p>}
                    </div>
                  </details>
                </div>
              )}
            </section>

            <section className="workSurface">
            <div className="surfaceHeader">
              <h2>Documents</h2>
              <div className="headerActions">
                <span className={trackingIngestion ? 'syncStatus syncActive' : 'syncStatus'}>
                  {ingestionLabel}
                </span>
                <button onClick={() => void refreshDocuments()} type="button">
                  Refresh
                </button>
              </div>
            </div>

            <form className="uploadBar" onSubmit={submitUpload}>
              <input
                aria-label="Document"
                accept={supportedDocumentAccept}
                className="fileInput"
                id="library-document-upload"
                disabled={!canUploadDocuments}
                type="file"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <label className="filePicker" htmlFor="library-document-upload">
                <strong>{file ? 'Selected' : 'Choose file'}</strong>
                <span>{file?.name ?? 'Markdown, text, PDF, CSV, JSON'}</span>
              </label>
              <button
                disabled={submitting || uploadingSample || !workspaceReady || !canUploadDocuments}
                onClick={() => void uploadSampleDocument()}
                type="button"
              >
                {uploadingSample ? 'Adding' : 'Sample'}
              </button>
              <button
                disabled={submitting || uploadingSample || !workspaceReady || !canUploadDocuments}
                type="submit"
              >
                {submitting ? 'Uploading' : 'Upload'}
              </button>
            </form>

            {registration && (
              <div className="resultBand">
                <span>{registration.document.name}</span>
                <strong className={stateClass(registration.document.status)}>
                  {registration.document.status}
                </strong>
                <em className={stateClass(registration.job.state)}>{registration.job.state}</em>
              </div>
            )}

            <div className="documentSummary">
              <button onClick={() => setActiveView('documents')} type="button">
                <strong>{documentCount}</strong>
                <span>Total</span>
              </button>
              <button onClick={() => setActiveView('documents')} type="button">
                <strong>{readyDocumentCount}</strong>
                <span>Ready</span>
              </button>
              <button onClick={() => setActiveView('activity')} type="button">
                <strong>{processingDocumentCount}</strong>
                <span>Working</span>
              </button>
              <button onClick={() => setActiveView('activity')} type="button">
                <strong>{failedDocumentCount}</strong>
                <span>Failed</span>
              </button>
            </div>

            <div className="tableList">
              {documents?.documents.map((document) => {
                const active = activeIngestionDocumentIDs.has(document.id);
                const selected = documentDetail?.document.id === document.id;
                return (
                  <div
                    className={selected ? 'documentRow selectedRow' : 'documentRow'}
                    key={document.id}
                  >
                    <button onClick={() => void openDocument(document)} type="button">
                      <strong>{document.name}</strong>
                      <small>{documentStatusHint(document, active)}</small>
                    </button>
                    <span className={stateClass(active ? 'running' : document.status)}>
                      {active ? 'working' : document.status}
                    </span>
                    <em title={document.id}>{formatDateTime(document.updated_at)}</em>
                    <small>{formatBytes(document.size_bytes)}</small>
                    <div className="documentActions">
                      <button
                        disabled={loadingDocumentID === document.id}
                        onClick={() => void openDocument(document)}
                        type="button"
                      >
                        {loadingDocumentID === document.id
                          ? 'Loading'
                          : selected
                            ? 'Open'
                            : 'Details'}
                      </button>
                      <button
                        disabled={downloadingDocumentID === document.id || active}
                        onClick={() => void downloadDocumentSource(document)}
                        type="button"
                      >
                        {downloadingDocumentID === document.id ? 'Downloading' : 'Download'}
                      </button>
                      {document.status === 'failed' && (
                        <button
                          disabled={!canUploadDocuments || retryingDocumentID === document.id || active}
                          onClick={() => void retryDocumentIngestion(document.id)}
                          type="button"
                        >
                          {retryingDocumentID === document.id
                            ? 'Retrying'
                            : active
                              ? 'Queued'
                              : 'Retry'}
                        </button>
                      )}
                      <details className="rowMenu compactRowMenu">
                        <summary>More</summary>
                        <div className="rowMenuActions">
                          <button
                            className="dangerButton"
                            disabled={!canUploadDocuments || deletingDocumentID === document.id || active}
                            onClick={() => void removeDocument(document.id, document.name)}
                            type="button"
                          >
                            {deletingDocumentID === document.id ? 'Deleting' : 'Delete'}
                          </button>
                        </div>
                      </details>
                    </div>
                  </div>
                );
              })}
              {documents && documents.documents.length === 0 && <p className="muted">No documents</p>}
            </div>

            {documentDetail && (
              <div className="detailPanel">
                <div className="detailHeader">
                  <h3>{documentDetail.document.name}</h3>
                  <div className="detailActions">
                    <span className={stateClass(documentDetail.document.status)}>
                      {documentDetail.document.status}
                    </span>
                    <button
                      disabled={downloadingDocumentID === documentDetail.document.id}
                      onClick={() => void downloadDocumentSource(documentDetail.document)}
                      type="button"
                    >
                      {downloadingDocumentID === documentDetail.document.id ? 'Downloading' : 'Download'}
                    </button>
                    {documentDetail.document.status === 'failed' && (
                      <button
                        disabled={
                          retryingDocumentID === documentDetail.document.id ||
                          hasActiveIngestionJob(documentDetail)
                        }
                        onClick={() => void retryDocumentIngestion(documentDetail.document.id)}
                        type="button"
                      >
                        {retryingDocumentID === documentDetail.document.id
                          ? 'Retrying'
                          : hasActiveIngestionJob(documentDetail)
                            ? 'Queued'
                            : 'Retry'}
                      </button>
                    )}
                  </div>
                </div>
                <div className="documentStatusPanel">
                  <strong>{documentStatusTitle(documentDetail)}</strong>
                  <span>{documentStatusDetail(documentDetail)}</span>
                </div>
                {documentDetail.document.status === 'failed' &&
                  documentFailureMessage(documentDetail) && (
                    <div className="failureNotice">
                      <strong>Ingestion failed</strong>
                      <span>{documentFailureMessage(documentDetail)}</span>
                    </div>
                  )}
                <dl className="runtimeList detailList">
                  <div>
                    <dt>ID</dt>
                    <dd>{documentDetail.document.id}</dd>
                  </div>
                  <div>
                    <dt>Size</dt>
                    <dd>{formatBytes(documentDetail.document.size_bytes)}</dd>
                  </div>
                  <div>
                    <dt>Storage</dt>
                    <dd>{documentDetail.document.storage_key}</dd>
                  </div>
                  <div>
                    <dt>Created</dt>
                    <dd>{formatDateTime(documentDetail.document.created_at)}</dd>
                  </div>
                  <div>
                    <dt>Updated</dt>
                    <dd>{formatDateTime(documentDetail.document.updated_at)}</dd>
                  </div>
                </dl>
                <details className="inlineDetails" open>
                  <summary>Activity</summary>
                  <div className="tableList">
                    {documentDetail.jobs.map((job) => (
                      <div className="jobRow" key={job.id}>
                        <strong>{jobTypeLabel(job)}</strong>
                        <span className={stateClass(job.state)}>{titleCase(job.state)}</span>
                        <em className={job.error_message ? 'jobError' : ''} title={jobTitle(job)}>
                          {jobDetail(job)}
                        </em>
                        <small>{job.attempts} tries</small>
                        <time dateTime={job.updated_at}>{formatDateTime(job.updated_at)}</time>
                      </div>
                    ))}
                    {documentDetail.jobs.length === 0 && <p className="muted">No activity</p>}
                  </div>
                </details>
              </div>
            )}
            </section>
          </div>
        )}

        {activeView === 'activity' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Activity</h2>
              <div className="headerActions">
                <span className={trackingIngestion ? 'syncStatus syncActive' : 'syncStatus'}>
                  {ingestionLabel}
                </span>
                <button onClick={() => void refreshJobs()} type="button">
                  Refresh
                </button>
              </div>
            </div>

            <div className="tableList">
              {jobs?.jobs.map((job) => (
                <div className="jobRow" key={job.id}>
                  <strong>{job.type}</strong>
                  <span className={stateClass(job.state)}>{job.state}</span>
                  <em className={job.error_message ? 'jobError' : ''} title={jobTitle(job)}>
                    {jobDetail(job)}
                  </em>
                  <small>{job.attempts} tries</small>
                  <time dateTime={job.updated_at}>{formatDateTime(job.updated_at)}</time>
                </div>
              ))}
              {jobs && jobs.jobs.length === 0 && <p className="muted">No activity</p>}
            </div>
          </div>
        )}

        {activeView === 'history' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>History</h2>
              <button onClick={() => void refreshConversations()} type="button">
                Refresh
              </button>
            </div>

            <div className="tableList">
              {conversations?.conversations.map((conversation) => (
                <div
                  className={
                    selectedConversationID === conversation.id
                      ? 'conversationRow selectedRow'
                      : 'conversationRow'
                  }
                  key={conversation.id}
                >
                  <button onClick={() => void openConversation(conversation)} type="button">
                    <strong>{conversation.title || conversation.id}</strong>
                  </button>
                  <span>{conversation.model_target}</span>
                  <time dateTime={conversation.updated_at}>
                    {formatDateTime(conversation.updated_at)}
                  </time>
                  <details className="rowMenu">
                    <summary>More</summary>
                    <div className="rowMenuActions">
                      <button onClick={() => resumeConversation(conversation)} type="button">
                        Resume
                      </button>
                      <button
                        className="dangerButton"
                        disabled={deletingConversationID === conversation.id}
                        onClick={() => void removeConversation(conversation)}
                        type="button"
                      >
                        {deletingConversationID === conversation.id ? 'Deleting' : 'Delete'}
                      </button>
                    </div>
                  </details>
                </div>
              ))}
              {conversations && conversations.conversations.length === 0 && (
                <p className="muted">No conversations</p>
              )}
            </div>

            {conversationMessages && (
              <div className="messageStack">
                {conversationMessages.messages.map((message) => (
                  <div className={`messageBubble ${message.role}`} key={message.id}>
                    <span>{message.role}</span>
                    <p>{message.content}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {activeView === 'search' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Search</h2>
              <span>{workspaceLabel}</span>
            </div>
            <form className="searchBar" onSubmit={submitSearch}>
              <input
                aria-label="Search"
                onChange={(event) =>
                  setSearchForm((current) => ({ ...current, query: event.target.value }))
                }
                placeholder="Search documents"
                value={searchForm.query}
              />
              <details className="menuPanel compactMenu">
                <summary>Options</summary>
                <div className="menuFields">
                  <label>
                    Source
                    <DocumentSelect
                      documents={documents?.documents ?? []}
                      value={searchForm.document_id}
                      onChange={(value) =>
                        setSearchForm((current) => ({ ...current, document_id: value }))
                      }
                    />
                  </label>
                  <label>
                    Limit
                    <input
                      max="20"
                      min="1"
                      type="number"
                      value={searchForm.limit}
                      onChange={(event) =>
                        setSearchForm((current) => ({
                          ...current,
                          limit: Number(event.target.value),
                        }))
                      }
                    />
                  </label>
                  <label>
                    Retrieval
                    <select
                      value={searchForm.strategy}
                      onChange={(event) =>
                        setSearchForm((current) => ({
                          ...current,
                          strategy: event.target.value as RetrievalStrategy,
                        }))
                      }
                    >
                      {retrievalStrategies.map((strategy) => (
                        <option key={strategy.value} value={strategy.value}>
                          {strategy.label}
                        </option>
                      ))}
                    </select>
                  </label>
                </div>
              </details>
              <button disabled={searching || !workspaceReady} type="submit">
                {searching ? 'Searching' : 'Search'}
              </button>
            </form>
            {searchResult && (
              <SourcePreviewLayout
                emptyLabel="No matches"
                hits={searchResult.hits}
                label={searchForm.query}
                onSelect={(hit) => setSelectedSearchSourceKey(resultHitKey(hit))}
                selectedHit={selectedSearchSource}
                selectedKey={selectedSearchSourceKey}
              />
            )}
          </div>
        )}

        {activeView === 'status' && (
          <div className="workSurface">
            <div className="surfaceHeader">
              <h2>Status</h2>
              <button onClick={() => void refreshRuntime()} type="button">
                Refresh
              </button>
            </div>

            <div className="statusGrid">
              <StatusTile
                detail={health?.version ?? apiBase()}
                label="API"
                ready={health?.status === 'ok'}
                value={health?.status ?? 'offline'}
              />
              <StatusTile
                detail={readiness?.run_migrations ? 'migrations on' : 'migrations off'}
                label="Database"
                ready={readiness?.status === 'ready'}
                value={readiness?.persistence_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.object_store_configured ? 'configured' : 'local profile'}
                label="Objects"
                ready={readiness?.status === 'ready'}
                value={readiness?.object_storage_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.vector_collection ?? 'documents'}
                label="Vectors"
                ready={readiness?.status === 'ready'}
                value={readiness?.vector_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.embedding_model ?? 'unknown'}
                label="Embeddings"
                ready={readiness?.status === 'ready'}
                value={readiness?.embedding_backend ?? 'unknown'}
              />
              <StatusTile
                detail={readiness?.model_gateway_auth ? 'auth configured' : 'no gateway auth'}
                label="Model Gateway"
                ready={readiness?.status === 'ready'}
                value={readiness?.model_gateway ?? 'unknown'}
              />
              <StatusTile
                detail={`${jobs?.jobs.length ?? 0} recent jobs`}
                label="Worker"
                ready={Boolean(jobs)}
                value={activeJobCount > 0 ? `${activeJobCount} active` : 'idle'}
              />
              <StatusTile
                detail={`${targets.length} configured`}
                label="Targets"
                ready={targets.length > 0}
                value={targets[0]?.name ?? 'none'}
              />
            </div>

            <ProviderPanel readiness={readiness} targets={targets} />

            <details className="inlineDetails">
              <summary>Endpoints</summary>
              <dl className="runtimeList">
                <div>
                  <dt>API</dt>
                  <dd>{apiBase()}</dd>
                </div>
                <div>
                  <dt>Queue</dt>
                  <dd>{readiness?.queue_backend ?? 'unknown'}</dd>
                </div>
                <div>
                  <dt>Models</dt>
                  <dd>{readiness?.model_gateway ?? 'unknown'}</dd>
                </div>
                <div>
                  <dt>Embeddings</dt>
                  <dd>{readiness?.embedding_gateway ?? 'unknown'}</dd>
                </div>
              </dl>
            </details>
          </div>
        )}

        {activeView === 'settings' && (
          <div className={workspaceReady ? 'settingsGrid' : 'settingsGrid settingsGridFocused'}>
            <section className="workSurface settingsPrimary">
              <div className="surfaceHeader">
                <h2>Workspaces</h2>
                <span>{currentUser?.user.email ?? 'Unknown user'}</span>
              </div>
              {needsWorkspace && (
                <div className="setupNotice">
                  <strong>Create workspace</strong>
                  <span>Start with one workspace for documents, search, and chat.</span>
                </div>
              )}
              <details className="inlineDetails" open={needsWorkspace}>
                <summary>New workspace</summary>
                <form className="inlineForm workspaceCreateForm" onSubmit={submitTenant}>
                  <input
                    aria-label="Workspace name"
                    onChange={(event) => setTenantName(event.target.value)}
                    value={tenantName}
                  />
                  <button disabled={creatingTenant} type="submit">
                    {creatingTenant ? 'Creating' : 'Create'}
                  </button>
                </form>
              </details>
              <div className="dataTable workspaceTable">
                <div className="dataHeader">
                  <span>Name</span>
                  <span>Role</span>
                  <span></span>
                  <span></span>
                </div>
                {currentUser?.memberships.map((membership) => (
                  <div
                    className={
                      membership.tenant.id === tenantID
                        ? 'dataRow tenantRow selectedRow'
                        : 'dataRow tenantRow'
                    }
                    key={membership.tenant.id}
                    title={membership.tenant.id}
                  >
                    <strong>{membership.tenant.name}</strong>
                    <span>{membership.role}</span>
                    <button
                      disabled={membership.tenant.id === tenantID}
                      onClick={() => void switchTenant(membership.tenant.id)}
                      type="button"
                    >
                      {membership.tenant.id === tenantID ? 'Open' : 'Use'}
                    </button>
                    <button
                      className="dangerButton"
                      disabled={
                        deletingTenantID === membership.tenant.id ||
                        !['owner', 'admin'].includes(membership.role)
                      }
                      onClick={() => void removeTenant(membership)}
                      type="button"
                    >
                      {deletingTenantID === membership.tenant.id ? 'Deleting' : 'Delete'}
                    </button>
                  </div>
                ))}
                {currentUser && currentUser.memberships.length === 0 && (
                  <p className="muted">No workspaces</p>
                )}
              </div>
            </section>

            {workspaceReady && (
              <section className="workSurface">
                <div className="surfaceHeader">
                  <h2>Members</h2>
                  <span>{workspaceLabel}</span>
                </div>
                {!canManageMembers && <p className="muted">Owner or admin access required</p>}
                {canManageMembers && (
                  <>
                    <details className="inlineDetails">
                      <summary>Add member</summary>
                      <form className="memberForm" onSubmit={submitMember}>
                        <input
                          aria-label="Member user ID"
                          onChange={(event) =>
                            setMemberForm((current) => ({
                              ...current,
                              user_id: event.target.value,
                            }))
                          }
                          placeholder="User ID"
                          value={memberForm.user_id}
                        />
                        <input
                          aria-label="Member email"
                          onChange={(event) =>
                            setMemberForm((current) => ({ ...current, email: event.target.value }))
                          }
                          placeholder="Email"
                          type="email"
                          value={memberForm.email}
                        />
                        <input
                          aria-label="Member name"
                          onChange={(event) =>
                            setMemberForm((current) => ({ ...current, name: event.target.value }))
                          }
                          placeholder="Name"
                          value={memberForm.name}
                        />
                        <select
                          aria-label="Member role"
                          onChange={(event) =>
                            setMemberForm((current) => ({ ...current, role: event.target.value }))
                          }
                          value={memberForm.role}
                        >
                          <option value="member">Member</option>
                          <option value="viewer">Viewer</option>
                          <option value="admin">Admin</option>
                          <option value="owner">Owner</option>
                        </select>
                        <button disabled={savingMember} type="submit">
                          {savingMember ? 'Saving' : 'Add'}
                        </button>
                      </form>
                    </details>

                    <div className="dataTable memberTable">
                      <div className="dataHeader">
                        <span>User</span>
                        <span>Role</span>
                        <span></span>
                      </div>
                      {tenantMembers?.members.map((member) => {
                        const isCurrentUser = member.user.id === currentUser?.user.id;
                        const isLastOwner = member.role === 'owner' && ownerCount <= 1;
                        return (
                          <div className="dataRow memberRow" key={member.user.id}>
                            <strong>{member.user.email}</strong>
                            <span>{member.role}</span>
                            <button
                              className="dangerButton"
                              disabled={
                                removingMemberID === member.user.id ||
                                isCurrentUser ||
                                isLastOwner
                              }
                              onClick={() =>
                                void removeTenantMember(member.user.id, member.user.email)
                              }
                              type="button"
                            >
                              {removingMemberID === member.user.id ? 'Removing' : 'Remove'}
                            </button>
                          </div>
                        );
                      })}
                      {tenantMembers && tenantMembers.members.length === 0 && (
                        <p className="muted">No members</p>
                      )}
                      {!tenantMembers && <p className="muted">Loading members</p>}
                    </div>
                  </>
                )}
              </section>
            )}

            <section className="settingsAdvanced settingsWide">
              <details className="settingsDetails">
                <summary>
                  <span>Runtime</span>
                  <em>{readiness?.provider_preset ?? 'unknown'}</em>
                </summary>
                <div className="settingsDetailsBody">
                  <ModelPerformancePanel
                    canTest={workspaceReady}
                    checkingTarget={checkingTarget}
                    onRefresh={() => void refreshRuntime()}
                    onTest={(targetName) => void testModelTarget(targetName)}
                    rows={modelPerformanceRows}
                    summary={modelPerformanceSummary}
                  />
                  <ProviderPanel readiness={readiness} targets={targets} />
                  <details className="inlineDetails">
                    <summary>Services</summary>
                    <dl className="runtimeList">
                      <div>
                        <dt>API</dt>
                        <dd>{apiBase()}</dd>
                      </div>
                      <div>
                        <dt>Auth</dt>
                        <dd>{readiness?.auth_mode ?? 'unknown'}</dd>
                      </div>
                      <div>
                        <dt>Storage</dt>
                        <dd>{readiness?.persistence_backend ?? 'unknown'}</dd>
                      </div>
                      <div>
                        <dt>Objects</dt>
                        <dd>{readiness?.object_storage_backend ?? 'unknown'}</dd>
                      </div>
                      <div>
                        <dt>Queue</dt>
                        <dd>{readiness?.queue_backend ?? 'unknown'}</dd>
                      </div>
                    </dl>
                  </details>
                  <details className="inlineDetails">
                    <summary>Model Targets</summary>
                    <div className="dataTable targetTable">
                      <div className="dataHeader">
                        <span>Target</span>
                        <span>Model</span>
                        <span>Check</span>
                        <span></span>
                      </div>
                      {targets.map((target) => {
                        const check = targetChecks[target.name];
                        return (
                          <div className="dataRow targetRow" key={target.name}>
                            <span>{target.name}</span>
                            <strong>{target.model}</strong>
                            <em
                              className={
                                check?.state === 'ok'
                                  ? 'checkReady'
                                  : check?.state === 'failed'
                                    ? 'checkFailed'
                                    : ''
                              }
                            >
                              {check?.detail ?? target.provider}
                            </em>
                            <button
                              disabled={checkingTarget === target.name}
                              onClick={() => void testModelTarget(target.name)}
                              type="button"
                            >
                              {checkingTarget === target.name ? 'Testing' : 'Test'}
                            </button>
                          </div>
                        );
                      })}
                      {targets.length === 0 && <p className="muted">No targets</p>}
                    </div>
                  </details>
                </div>
              </details>

              {workspaceReady && canManageTenant && (
                <details className="settingsDetails">
                  <summary>
                    <span>Audit</span>
                    <em>
                      {auditEvents
                        ? auditFiltersActive
                          ? `${filteredAuditEvents.length}/${auditEvents.events.length} matches`
                          : `${auditEvents.events.length} recent`
                        : 'not loaded'}
                    </em>
                  </summary>
                  <div className="settingsDetailsBody">
                    <div className="auditToolbar">
                      <select
                        aria-label="Audit action filter"
                        onChange={(event) =>
                          setAuditFilters((current) => ({
                            ...current,
                            action: event.target.value,
                          }))
                        }
                        value={auditFilters.action}
                      >
                        <option value="">All actions</option>
                        {auditActionOptions.map((action) => (
                          <option key={action} value={action}>
                            {action}
                          </option>
                        ))}
                      </select>
                      <select
                        aria-label="Audit outcome filter"
                        onChange={(event) =>
                          setAuditFilters((current) => ({
                            ...current,
                            outcome: event.target.value,
                          }))
                        }
                        value={auditFilters.outcome}
                      >
                        <option value="">All outcomes</option>
                        {auditOutcomeOptions.map((outcome) => (
                          <option key={outcome} value={outcome}>
                            {outcome}
                          </option>
                        ))}
                      </select>
                      <input
                        aria-label="Audit actor filter"
                        list="audit-actors"
                        onChange={(event) =>
                          setAuditFilters((current) => ({
                            ...current,
                            actor_user_id: event.target.value,
                          }))
                        }
                        placeholder="Actor"
                        value={auditFilters.actor_user_id}
                      />
                      <datalist id="audit-actors">
                        {auditActorOptions.map((actor) => (
                          <option key={actor} value={actor} />
                        ))}
                      </datalist>
                      <input
                        aria-label="Audit from date"
                        onChange={(event) =>
                          setAuditFilters((current) => ({
                            ...current,
                            from: event.target.value,
                          }))
                        }
                        type="date"
                        value={auditFilters.from}
                      />
                      <input
                        aria-label="Audit to date"
                        onChange={(event) =>
                          setAuditFilters((current) => ({
                            ...current,
                            to: event.target.value,
                          }))
                        }
                        type="date"
                        value={auditFilters.to}
                      />
                      <input
                        aria-label="Audit text filter"
                        onChange={(event) =>
                          setAuditFilters((current) => ({
                            ...current,
                            query: event.target.value,
                          }))
                        }
                        placeholder="Actor, resource, metadata"
                        value={auditFilters.query}
                      />
                      <button
                        disabled={!auditFiltersActive}
                        onClick={() => {
                          setAuditFilters(initialAuditFilters);
                          void refreshAuditEvents(tenantID, initialAuditFilters);
                        }}
                        type="button"
                      >
                        Clear
                      </button>
                      <button
                        disabled={loadingAudit}
                        onClick={() => void refreshAuditEvents()}
                        type="button"
                      >
                        {loadingAudit ? 'Applying' : 'Apply'}
                      </button>
                    </div>
                    <div className="tableList">
                      {filteredAuditEvents.map((event) => (
                        <div className="auditRow" key={event.id} title={event.id}>
                          <strong>{event.action}</strong>
                          <span className={stateClass(event.outcome)}>{event.outcome}</span>
                          <em>{auditResourceLabel(event)}</em>
                          <small>{event.actor_user_id}</small>
                          <time dateTime={event.created_at}>
                            {formatDateTime(event.created_at)}
                          </time>
                          <p>{auditMetadataLabel(event.metadata)}</p>
                        </div>
                      ))}
                      {auditEvents && auditEvents.events.length === 0 && (
                        <p className="muted">No audit events</p>
                      )}
                      {auditEvents &&
                        auditEvents.events.length > 0 &&
                        filteredAuditEvents.length === 0 && (
                          <p className="muted">No matching audit events</p>
                        )}
                      {!auditEvents && !loadingAudit && <p className="muted">Audit not loaded</p>}
                    </div>
                  </div>
                </details>
              )}

              <details className="settingsDetails">
                <summary>
                  <span>Operator</span>
                  <em>
                    {readiness?.shutdown_available
                      ? 'shutdown ready'
                      : readiness?.shutdown_enabled
                        ? 'setup needed'
                        : 'disabled'}
                  </em>
                </summary>
                <div className="settingsDetailsBody">
                  <div className="operatorPanel">
                    <div>
                      <strong>Shutdown stack</strong>
                      <span>
                        Stop Nexus Local containers for {readiness?.shutdown_project ?? 'this project'}.
                        Volumes stay intact.
                      </span>
                    </div>
                    <button
                      className="dangerButton"
                      disabled={
                        shuttingDown ||
                        !workspaceReady ||
                        !canManageTenant ||
                        !readiness?.shutdown_available
                      }
                      onClick={() => void requestRuntimeShutdown()}
                      type="button"
                    >
                      {shuttingDown ? 'Stopping' : 'Shut down'}
                    </button>
                  </div>
                  {shutdownNotice && <div className="setupNotice">{shutdownNotice}</div>}
                  {!readiness?.shutdown_available && (
                    <p className="muted">
                      Enable local operator controls from guided setup to use this button.
                    </p>
                  )}
                </div>
              </details>

              <details className="settingsDetails">
                <summary>
                  <span>Diagnostics</span>
                  <em>{health?.status === 'ok' ? 'healthy' : 'offline'}</em>
                </summary>
                <div className="settingsDetailsBody">
                  <div className="settingsDetailsActions">
                    <button onClick={() => void refreshRuntime()} type="button">
                      Refresh
                    </button>
                  </div>
                  <div className="statusGrid">
                    <StatusTile
                      detail={health?.version ?? apiBase()}
                      label="API"
                      ready={health?.status === 'ok'}
                      value={health?.status ?? 'offline'}
                    />
                    <StatusTile
                      detail={readiness?.run_migrations ? 'migrations on' : 'migrations off'}
                      label="Database"
                      ready={readiness?.status === 'ready'}
                      value={readiness?.persistence_backend ?? 'unknown'}
                    />
                    <StatusTile
                      detail={readiness?.object_store_configured ? 'configured' : 'local profile'}
                      label="Objects"
                      ready={readiness?.status === 'ready'}
                      value={readiness?.object_storage_backend ?? 'unknown'}
                    />
                    <StatusTile
                      detail={readiness?.vector_collection ?? 'documents'}
                      label="Vectors"
                      ready={readiness?.status === 'ready'}
                      value={readiness?.vector_backend ?? 'unknown'}
                    />
                    <StatusTile
                      detail={readiness?.embedding_model ?? 'unknown'}
                      label="Embeddings"
                      ready={readiness?.status === 'ready'}
                      value={readiness?.embedding_backend ?? 'unknown'}
                    />
                    <StatusTile
                      detail={readiness?.model_gateway_auth ? 'auth configured' : 'no gateway auth'}
                      label="Model Gateway"
                      ready={readiness?.status === 'ready'}
                      value={readiness?.model_gateway ?? 'unknown'}
                    />
                    <StatusTile
                      detail={`${jobs?.jobs.length ?? 0} recent jobs`}
                      label="Worker"
                      ready={Boolean(jobs)}
                      value={activeJobCount > 0 ? `${activeJobCount} active` : 'idle'}
                    />
                    <StatusTile
                      detail={`${targets.length} configured`}
                      label="Targets"
                      ready={targets.length > 0}
                      value={targets[0]?.name ?? 'none'}
                    />
                  </div>

                  <details className="inlineDetails">
                    <summary>Endpoints</summary>
                    <dl className="runtimeList">
                      <div>
                        <dt>API</dt>
                        <dd>{apiBase()}</dd>
                      </div>
                      <div>
                        <dt>Queue</dt>
                        <dd>{readiness?.queue_backend ?? 'unknown'}</dd>
                      </div>
                      <div>
                        <dt>Models</dt>
                        <dd>{readiness?.model_gateway ?? 'unknown'}</dd>
                      </div>
                      <div>
                        <dt>Embeddings</dt>
                        <dd>{readiness?.embedding_gateway ?? 'unknown'}</dd>
                      </div>
                    </dl>
                  </details>
                </div>
              </details>
            </section>
          </div>
        )}
        </section>
      </section>
    </main>
  );
}

function SourcePreviewLayout({
  emptyLabel,
  hits,
  label,
  onSelect,
  selectedHit,
  selectedKey,
}: {
  emptyLabel: string;
  hits: SearchDocumentsResponse['hits'];
  label: string;
  onSelect: (hit: SearchDocumentsResponse['hits'][number]) => void;
  selectedHit: SearchDocumentsResponse['hits'][number] | null;
  selectedKey: string;
}) {
  const activeKey = selectedHit ? resultHitKey(selectedHit) : selectedKey;

  return (
    <div className="sourcePreviewLayout">
      <div className="sourcePreviewList">
        <div className="resultToolbar">
          <strong>{hits.length} passages</strong>
          <span>{label}</span>
        </div>
        <div className="resultStack">
          {hits.map((hit, index) => (
            <ResultHit
              active={activeKey === resultHitKey(hit)}
              hit={hit}
              index={index}
              key={resultHitKey(hit)}
              onSelect={() => onSelect(hit)}
            />
          ))}
          {hits.length === 0 && <p className="muted">{emptyLabel}</p>}
        </div>
      </div>
      <SourcePreviewPanel hit={selectedHit} />
    </div>
  );
}

function ResultHit({
  active,
  hit,
  index,
  onSelect,
}: {
  active: boolean;
  hit: SearchDocumentsResponse['hits'][number];
  index: number;
  onSelect: () => void;
}) {
  const documentName = hit.source?.document_name || hit.metadata.document_name || hit.document_id;
  const chunkLabel = formatChunkLabel(hit);
  const sourceTitle = `${hit.document_id} / ${hit.chunk_id}`;

  return (
    <button
      className={active ? 'searchHit searchHitActive' : 'searchHit'}
      onClick={onSelect}
      title={sourceTitle}
      type="button"
    >
      <div className="sourceHeader">
        <span>#{index + 1}</span>
        <strong>{documentName}</strong>
        <em>{chunkLabel}</em>
      </div>
      <p className="searchSnippet">{hit.text}</p>
      <div className="sourceMeta">
        <span>{hit.score.toFixed(3)}</span>
        <span>{chunkLabel}</span>
      </div>
    </button>
  );
}

function SourcePreviewPanel({ hit }: { hit: SearchDocumentsResponse['hits'][number] | null }) {
  if (!hit) {
    return (
      <aside className="sourcePreview emptyPreview">
        <strong>Preview</strong>
        <span>No passage selected</span>
      </aside>
    );
  }

  const documentName = hit.source?.document_name || hit.metadata.document_name || hit.document_id;
  const chunkLabel = formatChunkLabel(hit);
  const storageKey = hit.source?.storage_key || hit.metadata.storage_key || '';
  const metadata = sourceMetadataEntries(hit);

  return (
    <aside className="sourcePreview">
      <div className="sourcePreviewHeader">
        <span>{chunkLabel}</span>
        <strong>{documentName}</strong>
        <em>{hit.score.toFixed(3)}</em>
      </div>
      <p>{hit.text}</p>
      <dl>
        <div>
          <dt>Document</dt>
          <dd>{hit.document_id}</dd>
        </div>
        <div>
          <dt>Chunk</dt>
          <dd>{hit.chunk_id}</dd>
        </div>
        {storageKey && (
          <div>
            <dt>Storage</dt>
            <dd>{storageKey}</dd>
          </div>
        )}
        {metadata.map(([key, value]) => (
          <div key={key}>
            <dt>{titleCase(key.replace(/_/g, ' '))}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </aside>
  );
}

function ModelPerformancePanel({
  canTest,
  checkingTarget,
  onRefresh,
  onTest,
  rows,
  summary,
}: {
  canTest: boolean;
  checkingTarget: string;
  onRefresh: () => void;
  onTest: (targetName: string) => void;
  rows: ModelPerformanceRow[];
  summary: ReturnType<typeof summarizeModelPerformance>;
}) {
  return (
    <section className="modelPerfPanel">
      <div className="modelPerfHeader">
        <div>
          <strong>Model performance</strong>
          <span>
            {summary.askCount} asks / {summary.failureCount} failures /{' '}
            {summary.p95LatencyMS === null ? 'no p95' : `${formatLatencyMS(summary.p95LatencyMS)} p95`}
          </span>
        </div>
        <button onClick={onRefresh} type="button">
          Refresh
        </button>
      </div>

      <div className="modelPerfMetrics">
        <div>
          <span>Targets</span>
          <strong>{summary.targetCount}</strong>
          <em>{summary.checkedCount} checked</em>
        </div>
        <div>
          <span>Success</span>
          <strong>{summary.successRate === null ? '-' : `${summary.successRate}%`}</strong>
          <em>{summary.askCount} recent</em>
        </div>
        <div>
          <span>P95</span>
          <strong>{summary.p95LatencyMS === null ? '-' : formatLatencyMS(summary.p95LatencyMS)}</strong>
          <em>Ask latency</em>
        </div>
        <div>
          <span>Review</span>
          <strong>{summary.unhealthyCount}</strong>
          <em>Failed checks/errors</em>
        </div>
      </div>

      <div className="dataTable modelPerfTable">
        <div className="dataHeader">
          <span>Target</span>
          <span>Probe</span>
          <span>Requests</span>
          <span>P95</span>
          <span>Last error</span>
          <span></span>
        </div>
        {rows.map((row) => {
          const checking = checkingTarget === row.target.name;
          const probeClass =
            row.check?.state === 'ok'
              ? 'stateBadge stateReady'
              : row.check?.state === 'failed'
                ? 'stateBadge stateFailed'
                : 'stateBadge';
          return (
            <div className="dataRow modelPerfRow" key={row.target.name}>
              <div className="modelPerfTarget">
                <strong>{row.target.name}</strong>
                <span>{row.target.model}</span>
                <em>{compactEndpoint(row.target.base_url)}</em>
              </div>
              <div className="modelPerfCell">
                <span className={probeClass}>
                  {row.check?.state === 'ok'
                    ? row.check.latency_ms === undefined
                      ? 'OK'
                      : formatLatencyMS(row.check.latency_ms)
                    : row.check?.state === 'failed'
                      ? 'Failed'
                      : 'Not checked'}
                </span>
                <em>{row.check?.checked_at ? formatTimeOnly(row.check.checked_at) : row.target.provider}</em>
              </div>
              <div className="modelPerfCell">
                <strong>{row.askCount}</strong>
                <em>
                  {row.successRate === null
                    ? 'No asks'
                    : `${row.successRate}% success / ${row.failureCount} failed`}
                </em>
              </div>
              <div className="modelPerfCell">
                <strong>{row.p95LatencyMS === null ? '-' : formatLatencyMS(row.p95LatencyMS)}</strong>
                <em>{row.latestLatencyMS === null ? 'No latency' : `${formatLatencyMS(row.latestLatencyMS)} latest`}</em>
              </div>
              <div className="modelPerfError">
                <strong>{row.lastErrorClass ? titleCase(row.lastErrorClass.replace(/_/g, ' ')) : 'None'}</strong>
                <span title={row.lastError}>{row.lastError || (row.lastEventAt ? formatTimeOnly(row.lastEventAt) : 'No recent errors')}</span>
              </div>
              <button disabled={!canTest || checking} onClick={() => onTest(row.target.name)} type="button">
                {checking ? 'Testing' : 'Test'}
              </button>
            </div>
          );
        })}
        {rows.length === 0 && <p className="muted">No model targets</p>}
      </div>
    </section>
  );
}

function DashboardAttentionPanel({
  activeDocuments,
  activeJobs,
  failedDocuments,
  failedJobs,
  onConnectSource,
  onOpenDocument,
  onRefresh,
  onViewActivity,
  onViewLibrary,
  processingDocumentCount,
  readyDocumentCount,
}: {
  activeDocuments: ListDocumentsResponse['documents'];
  activeJobs: ListJobsResponse['jobs'];
  failedDocuments: ListDocumentsResponse['documents'];
  failedJobs: ListJobsResponse['jobs'];
  onConnectSource: () => void;
  onOpenDocument: (document: ListDocumentsResponse['documents'][number]) => void;
  onRefresh: () => void;
  onViewActivity: () => void;
  onViewLibrary: () => void;
  processingDocumentCount: number;
  readyDocumentCount: number;
}) {
  const failedDocumentIDs = new Set(failedDocuments.map((document) => document.id));
  const activeDocumentIDs = new Set(activeDocuments.map((document) => document.id));
  const otherFailedJobs = failedJobs.filter(
    (job) => !(job.resource_type === 'document' && failedDocumentIDs.has(job.resource_id)),
  );
  const otherActiveJobs = activeJobs.filter(
    (job) => !(job.resource_type === 'document' && activeDocumentIDs.has(job.resource_id)),
  );
  const hasFailures = failedDocuments.length > 0 || otherFailedJobs.length > 0;
  const activeAttentionCount = Math.max(activeJobs.length, processingDocumentCount);
  const hasActive = activeAttentionCount > 0;
  const visibleFailedDocuments = failedDocuments.slice(0, 2);
  const visibleFailedJobs = otherFailedJobs.slice(0, Math.max(0, 2 - visibleFailedDocuments.length));
  const visibleActiveDocuments = activeDocuments.slice(0, 2);
  const visibleActiveJobs = otherActiveJobs.slice(0, Math.max(0, 3 - visibleActiveDocuments.length));
  const totalAttention =
    failedDocuments.length + otherFailedJobs.length + activeAttentionCount;
  const visibleAttentionCount = hasFailures
    ? visibleFailedDocuments.length + visibleFailedJobs.length
    : hasActive
      ? visibleActiveDocuments.length + visibleActiveJobs.length
      : 0;
  const hiddenAttentionCount = Math.max(0, totalAttention - visibleAttentionCount);
  const firstFailedDocument = failedDocuments[0];
  const firstFailedJob = otherFailedJobs[0];
  const nextAction = hasFailures
    ? {
        label: 'Blocked',
        title: 'Review failures',
        detail: firstFailedDocument?.name ?? jobDetail(firstFailedJob),
        button: firstFailedDocument ? 'Open' : 'Activity',
        onClick: () => (firstFailedDocument ? onOpenDocument(firstFailedDocument) : onViewActivity()),
      }
    : hasActive
      ? {
          label: 'Active',
          title: 'Track progress',
          detail: `${activeAttentionCount} ${activeAttentionCount === 1 ? 'item' : 'items'} running`,
          button: 'Activity',
          onClick: onViewActivity,
        }
      : readyDocumentCount === 0
        ? {
            label: 'Next',
            title: 'Add knowledge',
            detail: 'No ready documents',
            button: 'Connect',
            onClick: onConnectSource,
          }
        : {
            label: 'Ready',
            title: 'Ask or search',
            detail: `${readyDocumentCount} ${readyDocumentCount === 1 ? 'document' : 'documents'} ready`,
            button: 'Library',
            onClick: onViewLibrary,
          };

  return (
    <section
      className={
        hasFailures
          ? 'workSurface attentionPanel attentionPanelFailed'
          : hasActive
            ? 'workSurface attentionPanel attentionPanelActive'
            : 'workSurface attentionPanel'
      }
    >
      <div className="attentionSummary">
        <button className="attentionMetric" onClick={onViewLibrary} type="button">
          <strong>{readyDocumentCount}</strong>
          <span>Ready</span>
        </button>
        <button className="attentionMetric" onClick={onViewActivity} type="button">
          <strong>{activeAttentionCount}</strong>
          <span>Active</span>
        </button>
        <button className="attentionMetric" onClick={onViewActivity} type="button">
          <strong>{failedDocuments.length + otherFailedJobs.length}</strong>
          <span>Failed</span>
        </button>
      </div>

      <div className="attentionBody">
        <div className="attentionNext">
          <span>{nextAction.label}</span>
          <strong>{nextAction.title}</strong>
          <em title={nextAction.detail}>{nextAction.detail}</em>
          <button onClick={nextAction.onClick} type="button">
            {nextAction.button}
          </button>
        </div>

        <div className="attentionFeed">
          {visibleFailedDocuments.map((document) => (
            <button
              className="attentionItem attentionItemFailed"
              key={document.id}
              onClick={() => onOpenDocument(document)}
              type="button"
            >
              <strong>{document.name}</strong>
              <span className={stateClass(document.status)}>{document.status}</span>
              <em>Open</em>
            </button>
          ))}
          {visibleFailedJobs.map((job) => (
            <button
              className="attentionItem attentionItemFailed"
              key={job.id}
              onClick={onViewActivity}
              title={jobTitle(job)}
              type="button"
            >
              <strong>{jobTypeLabel(job)}</strong>
              <span className={stateClass(job.state)}>{job.state}</span>
              <em>{jobDetail(job)}</em>
            </button>
          ))}
          {!hasFailures &&
            visibleActiveDocuments.map((document) => (
              <button
                className="attentionItem"
                key={document.id}
                onClick={() => onOpenDocument(document)}
                type="button"
              >
                <strong>{document.name}</strong>
                <span className={stateClass(document.status)}>{document.status}</span>
                <em>Open</em>
              </button>
            ))}
          {!hasFailures &&
            visibleActiveJobs.map((job) => (
              <button
                className="attentionItem"
                key={job.id}
                onClick={onViewActivity}
                title={jobTitle(job)}
                type="button"
              >
                <strong>{jobTypeLabel(job)}</strong>
                <span className={stateClass(job.state)}>{job.state}</span>
                <em>{formatDateTime(job.updated_at)}</em>
              </button>
            ))}
          {hiddenAttentionCount > 0 && (
            <button className="attentionMore" onClick={onViewActivity} type="button">
              {hiddenAttentionCount} more
            </button>
          )}
          {!hasFailures && !hasActive && (
            <div className="attentionReady">
              <strong>Ready</strong>
              <span>{readyDocumentCount > 0 ? 'Knowledge base idle' : 'Add documents'}</span>
            </div>
          )}
        </div>
      </div>

      <div className="attentionActions">
        <button onClick={onRefresh} type="button">
          Refresh
        </button>
        <button
          onClick={
            totalAttention > 0
              ? onViewActivity
              : readyDocumentCount === 0
                ? onConnectSource
                : onViewLibrary
          }
          type="button"
        >
          {totalAttention > 0 ? 'Activity' : readyDocumentCount === 0 ? 'Connect' : 'Library'}
        </button>
      </div>
    </section>
  );
}

function jobTypeLabel(job: ListJobsResponse['jobs'][number]) {
  switch (job.type) {
    case 'document_ingestion':
      return 'Document ingestion';
    case 'source_scan':
      return 'Source scan';
    case 'source_preflight':
      return 'Source check';
    case 'source_plan':
      return 'Source plan';
    default:
      return titleCase(job.type.replace(/_/g, ' '));
  }
}

function DocumentSelect({
  disabled = false,
  documents,
  onChange,
  value,
}: {
  disabled?: boolean;
  documents: ListDocumentsResponse['documents'];
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <select disabled={disabled} onChange={(event) => onChange(event.target.value)} value={value}>
      <option value="">All documents</option>
      {documents.map((document) => (
        <option key={document.id} value={document.id}>
          {document.name}
        </option>
      ))}
    </select>
  );
}

function SourceConnectorFields({
  disabled = false,
  form,
  onChange,
}: {
  disabled?: boolean;
  form: SourceFormValues;
  onChange: (updates: Partial<SourceFormValues>) => void;
}) {
  if (form.type !== 'connector') {
    return null;
  }
  return (
    <div className="sourceConnectorFields">
      <label>
        <span>Provider</span>
        <input
          aria-label="Connector provider"
          disabled={disabled}
          onChange={(event) => onChange({ connector_provider: event.target.value })}
          placeholder="sharepoint"
          value={form.connector_provider}
        />
      </label>
      <label>
        <span>Resource</span>
        <input
          aria-label="Connector resource"
          disabled={disabled}
          onChange={(event) => onChange({ connector_resource_id: event.target.value })}
          placeholder="tenant/site/library"
          value={form.connector_resource_id}
        />
      </label>
      <label>
        <span>Credential ref</span>
        <input
          aria-label="Connector credential reference"
          disabled={disabled}
          onChange={(event) => onChange({ connector_credential_ref: event.target.value })}
          placeholder="secret/provider/read-only"
          value={form.connector_credential_ref}
        />
      </label>
      <label>
        <span>Notes</span>
        <textarea
          aria-label="Connector notes"
          disabled={disabled}
          onChange={(event) => onChange({ connector_notes: event.target.value })}
          placeholder="Reference deployment secrets or setup notes; do not paste credentials."
          value={form.connector_notes}
        />
      </label>
    </div>
  );
}

function SetupWizard({
  apiURL,
  canEnter,
  checks,
  gatewayCheck,
  lastChecked,
  onEnter,
  onRecheck,
  primaryTarget,
  readiness,
  setupAutoRecheckSeconds,
  setupChecking,
}: {
  apiURL: string;
  canEnter: boolean;
  checks: SetupCheck[];
  gatewayCheck: TargetCheckState | null;
  lastChecked: string;
  onEnter: () => void;
  onRecheck: () => void;
  primaryTarget?: ModelTarget;
  readiness: Readiness | null;
  setupAutoRecheckSeconds: number | null;
  setupChecking: boolean;
}) {
  const blockingCount = checks.filter(
    (check) => check.blocking && check.status === 'blocked',
  ).length;

  return (
    <main className="setupShell">
      <section className="setupHero">
        <p className="eyebrow">Nexus Local</p>
        <h1>Setup</h1>
        <span className={canEnter ? 'syncStatus' : 'syncStatus syncActive'}>
          {canEnter ? 'Ready' : blockingCount > 0 ? `${blockingCount} blocked` : 'Checking'}
        </span>
      </section>

      <section className="setupGrid">
        <div className="setupPanel">
          <div className="surfaceHeader">
            <h2>Machine checks</h2>
            <span>{lastChecked ? `Checked ${formatTimeOnly(lastChecked)}` : apiURL}</span>
          </div>

          <div className="setupChecks">
            {checks.map((check) => (
              <div className={`setupCheck setupCheck-${check.status}`} key={check.id}>
                <span>{setupStatusLabel(check.status)}</span>
                <strong>{check.label}</strong>
                <p>{check.detail}</p>
              </div>
            ))}
          </div>

          <div className="setupActions">
            <button disabled={setupChecking} onClick={onRecheck} type="button">
              {setupChecking ? 'Checking' : 'Recheck'}
            </button>
            <button disabled={!canEnter} onClick={onEnter} type="button">
              Enter app
            </button>
          </div>
          {setupAutoRecheckSeconds !== null && !canEnter && (
            <div className="setupAutoCheck">
              <span>Watching gateway</span>
              <strong>Next check in {setupAutoRecheckSeconds}s</strong>
            </div>
          )}
        </div>

        <aside className="setupPanel setupGuide">
          <div className="surfaceHeader">
            <h2>Gateway</h2>
            <span>{primaryTarget?.name ?? 'No target'}</span>
          </div>

          <dl className="runtimeList">
            <div>
              <dt>URL</dt>
              <dd>{readiness?.model_gateway || 'Not configured'}</dd>
            </div>
            <div>
              <dt>Model</dt>
              <dd>{primaryTarget?.model ?? 'Not configured'}</dd>
            </div>
            {gatewayCheck && (
              <div>
                <dt>Status</dt>
                <dd>{gatewayStateLabel(gatewayCheck.state)}</dd>
              </div>
            )}
            <div>
              <dt>Result</dt>
              <dd>{gatewayCheck?.detail ?? 'Waiting for check'}</dd>
            </div>
          </dl>

          <details className="inlineDetails" open={!canEnter && gatewayCheck?.state !== 'loading'}>
            <summary>Configure</summary>
            <div className="setupConfigGrid">
              <label>
                <span>Gateway URL</span>
                <input readOnly value={readiness?.model_gateway || 'Not configured'} />
              </label>
              <label>
                <span>Model</span>
                <input readOnly value={primaryTarget?.model ?? 'Not configured'} />
              </label>
              <p>
                Change these from the Nexus launcher setup, then return here and recheck.
              </p>
            </div>
          </details>

          <details className="inlineDetails">
            <summary>Common URLs</summary>
            <dl className="runtimeList">
              <div>
                <dt>Ollama</dt>
                <dd>http://host.docker.internal:11434/v1</dd>
              </div>
              <div>
                <dt>LM Studio</dt>
                <dd>http://host.docker.internal:1234/v1</dd>
              </div>
              <div>
                <dt>vLLM</dt>
                <dd>http://host.docker.internal:8000/v1</dd>
              </div>
            </dl>
          </details>
        </aside>
      </section>
    </main>
  );
}

function buildSetupChecks({
  checking,
  gatewayCheck,
  health,
  readiness,
  targets,
}: {
  checking: boolean;
  gatewayCheck: TargetCheckState | null;
  health: Health | null;
  readiness: Readiness | null;
  targets: ModelTarget[];
}): SetupCheck[] {
  const primaryTarget = targets.find((target) => target.name === 'general') ?? targets[0];
  const apiStatus: SetupCheckStatus = health ? 'ok' : checking ? 'checking' : 'blocked';
  const readinessStatus: SetupCheckStatus = readiness
    ? readiness.status === 'ready'
      ? 'ok'
      : 'blocked'
    : checking
      ? 'checking'
      : 'blocked';
  const targetStatus: SetupCheckStatus =
    targets.length > 0 ? 'ok' : checking ? 'checking' : 'blocked';
  const gatewayStatus: SetupCheckStatus =
    gatewayCheck?.state === 'ok'
      ? 'ok'
      : gatewayCheck?.state === 'loading'
        ? 'checking'
      : checking && gatewayCheck === null
        ? 'checking'
        : 'blocked';
  const embeddingStatus: SetupCheckStatus = readiness
    ? readiness.embedding_backend.toLowerCase() === 'hash'
      ? 'warning'
      : 'ok'
    : checking
      ? 'checking'
      : 'blocked';

  return [
    {
      id: 'api',
      label: 'API',
      detail: health ? `${health.env} ${health.version}` : 'Cannot reach the Nexus API.',
      status: apiStatus,
      blocking: true,
    },
    {
      id: 'runtime',
      label: 'Runtime',
      detail: readiness
        ? `${readiness.persistence_backend}, ${readiness.object_storage_backend}, ${readiness.vector_backend}`
        : 'Waiting for runtime readiness.',
      status: readinessStatus,
      blocking: true,
    },
    {
      id: 'target',
      label: 'Model target',
      detail: primaryTarget
        ? `${primaryTarget.name}: ${primaryTarget.model}`
        : 'Configure at least one model target.',
      status: targetStatus,
      blocking: true,
    },
    {
      id: 'gateway',
      label: 'Gateway responds',
      detail:
        gatewayCheck?.detail ??
        (readiness?.model_gateway
          ? `Testing ${compactEndpoint(readiness.model_gateway)}`
          : 'Configure an OpenAI-compatible gateway.'),
      status: gatewayStatus,
      blocking: true,
    },
    {
      id: 'embeddings',
      label: 'Embeddings',
      detail: readiness
        ? readiness.embedding_backend.toLowerCase() === 'hash'
          ? 'Hash embeddings are fine for setup; use semantic embeddings for production search.'
          : `${readiness.embedding_model} (${readiness.embedding_dimensions} dims)`
        : 'Waiting for embedding configuration.',
      status: embeddingStatus,
      blocking: false,
    },
  ];
}

function setupStatusLabel(status: SetupCheckStatus) {
  switch (status) {
    case 'ok':
      return 'OK';
    case 'warning':
      return 'Review';
    case 'checking':
      return 'Checking';
    case 'blocked':
      return 'Blocked';
  }
}

function gatewayStateLabel(state: TargetCheckState['state']) {
  switch (state) {
    case 'ok':
      return 'Ready';
    case 'loading':
      return 'Loading';
    case 'failed':
      return 'Needs configuration';
  }
}

function formatChunkLabel(hit: SearchDocumentsResponse['hits'][number]) {
  const chunkIndex = hit.source?.chunk_index;
  if (chunkIndex) {
    const numericIndex = Number(chunkIndex);
    if (Number.isFinite(numericIndex)) {
      return `Chunk ${numericIndex + 1}`;
    }
    return `Chunk ${chunkIndex}`;
  }
  return hit.source?.chunk_id || hit.chunk_id;
}

function resultHitKey(hit: SearchDocumentsResponse['hits'][number]) {
  return `${hit.document_id}:${hit.chunk_id}`;
}

function selectedResultHit(
  hits: SearchDocumentsResponse['hits'],
  selectedKey: string,
): SearchDocumentsResponse['hits'][number] | null {
  if (hits.length === 0) {
    return null;
  }
  return hits.find((hit) => resultHitKey(hit) === selectedKey) ?? hits[0];
}

function sourceMetadataEntries(hit: SearchDocumentsResponse['hits'][number]) {
  const hidden = new Set([
    'chunk_id',
    'chunk_index',
    'document_name',
    'lexical_score',
    'storage_key',
    'vector_score',
  ]);
  return Object.entries(hit.metadata ?? {})
    .filter(([key, value]) => value && !hidden.has(key))
    .slice(0, 6);
}

function StatusTile({
  detail,
  label,
  ready,
  value,
}: {
  detail: string;
  label: string;
  ready: boolean;
  value: string;
}) {
  return (
    <div className={ready ? 'statusTile statusTileReady' : 'statusTile'}>
      <span>{label}</span>
      <strong>{value}</strong>
      <em>{detail}</em>
    </div>
  );
}

function ProviderPanel({
  readiness,
  targets,
}: {
  readiness: Readiness | null;
  targets: ModelTarget[];
}) {
  const primaryTarget = targets.find((target) => target.name === 'general') ?? targets[0];
  const embeddingBackend = readiness?.embedding_backend ?? 'unknown';
  const hashEmbeddings = embeddingBackend.toLowerCase() === 'hash';
  const embeddingDetail =
    readiness && readiness.embedding_dimensions > 0
      ? `${embeddingBackend} / ${readiness.embedding_dimensions} dims`
      : embeddingBackend;

  return (
    <div className="providerPanel">
      <div className="providerCard">
        <span>Preset</span>
        <strong>{formatProviderPreset(readiness?.provider_preset)}</strong>
        <em>{readiness?.auth_mode ?? 'unknown'} auth</em>
      </div>
      <div className="providerCard">
        <span>Chat</span>
        <strong>{primaryTarget?.model ?? 'No target'}</strong>
        <em>
          {compactEndpoint(readiness?.model_gateway)} /{' '}
          {readiness?.model_gateway_auth ? 'key set' : 'no key'}
        </em>
      </div>
      <div
        className={hashEmbeddings ? 'providerCard providerCardWarn' : 'providerCard'}
        title={hashEmbeddings ? 'Hash embeddings are for setup and tests, not semantic retrieval.' : ''}
      >
        <span>Embeddings</span>
        <strong>{readiness?.embedding_model ?? 'unknown'}</strong>
        <em>
          {embeddingDetail} / {readiness?.embedding_gateway_auth ? 'key set' : 'no key'}
        </em>
      </div>
      <div className="providerCard">
        <span>Vectors</span>
        <strong>{readiness?.vector_collection ?? 'documents'}</strong>
        <em>{readiness?.vector_backend ?? 'unknown'}</em>
      </div>
    </div>
  );
}

function formatProviderPreset(value?: string) {
  switch ((value ?? '').toLowerCase()) {
    case 'starter':
      return 'Starter';
    case 'semantic':
      return 'Semantic';
    case '':
      return 'Unknown';
    default:
      return value ?? 'Unknown';
  }
}

function sourceTypeLabel(value: string) {
  switch (value) {
    case 'synced_folder':
      return 'Synced folder';
    case 'network_share':
      return 'Network share';
    case 'folder':
      return 'Local folder';
    case 'export':
      return 'Export';
    case 'connector':
      return 'Connector';
    default:
      return titleCase(value.replace(/_/g, ' '));
  }
}

function sourceRootPath(readiness: Readiness | null) {
  const configuredPath = readiness?.source_container_path?.trim();
  return configuredPath || '/sources/primary';
}

function sourcePathJoin(root: string, child: string) {
  const normalizedRoot = root.replace(/\/+$/, '') || '/sources/primary';
  const normalizedChild = child.replace(/^\/+/, '').replace(/\/+$/, '');
  return normalizedChild ? `${normalizedRoot}/${normalizedChild}` : normalizedRoot;
}

function dashboardSourceTemplates(root: string): SourceTemplate[] {
  const generalExcludes = [
    commonDocumentExcludes,
    '**/.git/**',
    '**/.obsidian/**',
    '**/node_modules/**',
    '**/tmp/**',
    '**/temp/**',
  ].join('\n');

  return [
    {
      id: 'dashboard-folder',
      label: 'Folder / vault',
      detail: 'Notes, docs, PDFs',
      type: 'synced_folder',
      name: 'Knowledge Folder',
      root_path: root,
      include_patterns: broadDocumentIncludes,
      exclude_patterns: generalExcludes,
      scan_interval_minutes: '0',
    },
    {
      id: 'dashboard-team-drive',
      label: 'Team drive',
      detail: 'OneDrive, Teams, share',
      type: 'synced_folder',
      name: 'Team Drive',
      root_path: sourcePathJoin(root, 'Team Docs'),
      include_patterns: broadDocumentIncludes,
      exclude_patterns: generalExcludes,
      scan_interval_minutes: '1440',
    },
    {
      id: 'dashboard-export',
      label: 'Export folder',
      detail: 'Cases, HAR, CSV, JSON',
      type: 'export',
      name: 'Case Export',
      root_path: sourcePathJoin(root, 'exports'),
      include_patterns: ['**/*.json', '**/*.csv', '**/*.html', '**/*.htm', '**/*.txt', '**/*.md'].join(
        '\n',
      ),
      exclude_patterns: ['**/attachments/**', '**/raw/**', '**/tmp/**', '**/temp/**'].join('\n'),
      scan_interval_minutes: '0',
    },
    {
      id: 'dashboard-runbooks',
      label: 'Runbooks',
      detail: 'Procedures, SOPs',
      type: 'synced_folder',
      name: 'Runbooks',
      root_path: sourcePathJoin(root, 'runbooks'),
      include_patterns: ['**/*.md', '**/*.txt', '**/*.pdf', '**/*.docx', '**/*.csv', '**/*.json'].join(
        '\n',
      ),
      exclude_patterns: generalExcludes,
      scan_interval_minutes: '10080',
    },
  ];
}

function sourcePathPlaceholder(type: string) {
  switch (type) {
    case 'connector':
      return 'connector://provider/resource';
    case 'export':
      return '/sources/primary/exports';
    case 'network_share':
      return '/sources/primary/share';
    default:
      return '/sources/primary';
  }
}

function sourceMountTitle(type: string) {
  switch (type) {
    case 'synced_folder':
      return 'Synced folder';
    case 'network_share':
      return 'Network share';
    case 'export':
      return 'Export folder';
    case 'connector':
      return 'Connector source';
    default:
      return 'Local folder';
  }
}

function sourceMountDescription(type: string) {
  switch (type) {
    case 'synced_folder':
      return 'Mount the synced root into the worker, then use its /sources path.';
    case 'network_share':
      return 'Mount SMB or NFS on the host first, then expose that folder to the worker.';
    case 'export':
      return 'Place exports under a mounted source root and scan the export folder.';
    case 'connector':
      return 'Capture connector handoff details now; direct connector workers will resolve these references later.';
    default:
      return 'Use the path the worker container can read, not the browser path.';
  }
}

function sourcePathExamples(type: string) {
  switch (type) {
    case 'synced_folder':
      return ['/sources/primary', '/sources/primary/OneDrive', '/sources/primary/SharePoint'];
    case 'network_share':
      return ['/sources/primary', '/sources/primary/customers', '/sources/primary/runbooks'];
    case 'export':
      return ['/sources/primary/exports', '/sources/primary/tickets', '/sources/primary/cases'];
    case 'connector':
      return ['connector://sharepoint/site', 'connector://onedrive/drive', 'connector://tickets/project'];
    default:
      return ['/sources/primary', '/sources/primary/docs'];
  }
}

function sourceFormFromSource(source: ListDataSourcesResponse['sources'][number]) {
  const connector = source.connector_config ?? {};
  return {
    type: source.type,
    name: source.name,
    root_path: source.root_path,
    connector_provider: connector.provider ?? '',
    connector_resource_id: connector.resource_id ?? '',
    connector_credential_ref: connector.credential_ref ?? '',
    connector_notes: connector.notes ?? '',
    include_patterns: (source.include_patterns ?? []).join('\n'),
    exclude_patterns: (source.exclude_patterns ?? []).join('\n'),
    scan_interval_minutes: String(source.scan_interval_minutes ?? 0),
  };
}

function connectorConfigFromSourceForm(form: SourceFormValues): ConnectorConfig {
  return {
    provider: form.connector_provider.trim(),
    resource_id: form.connector_resource_id.trim(),
    credential_ref: form.connector_credential_ref.trim(),
    notes: form.connector_notes.trim(),
  };
}

function sourceConnectorConfigured(config: ConnectorConfig = {}) {
  return Boolean(
    config.provider?.trim() ||
      config.resource_id?.trim() ||
      config.credential_ref?.trim() ||
      config.notes?.trim(),
  );
}

function roleCanWriteKnowledge(role?: string) {
  return role === 'owner' || role === 'admin' || role === 'member';
}

function sourceIsConnector(source: { type: string }) {
  return source.type === 'connector';
}

function patternLinesToList(value: string) {
  return value
    .split(/\r?\n|,/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'));
}

function folderRelativeName(file: File) {
  return (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name;
}

function isSupportedUploadFile(file: File) {
  const name = folderRelativeName(file).toLowerCase();
  return supportedDocumentExtensions.some((extension) => name.endsWith(extension));
}

function sourceScanSummary(source: ListDataSourcesResponse['sources'][number]) {
  if (sourceIsConnector(source)) {
    const connector = source.connector_config ?? {};
    return connector.resource_id || connector.provider || source.root_path;
  }
  if (!source.last_scan_at) {
    return source.root_path;
  }
  const imported = source.last_scan_imported ?? 0;
  const skipped = source.last_scan_skipped ?? 0;
  const failed = source.last_scan_failed ?? 0;
  const parts = [`${imported} imported`];
  if (skipped > 0) {
    parts.push(`${skipped} skipped`);
  }
  if (failed > 0) {
    parts.push(`${failed} failed`);
  }
  return `${source.root_path} / ${parts.join(', ')}`;
}

function compactEndpoint(value?: string) {
  if (!value) {
    return 'unknown';
  }
  try {
    return new URL(value).host || value;
  } catch {
    return value;
  }
}

function askPhaseFromStatus(message: string): AskPhase {
  const lower = message.toLowerCase();
  if (lower.includes('connect')) {
    return 'connecting';
  }
  if (lower.includes('retriev')) {
    return 'retrieving';
  }
  if (lower.includes('generat')) {
    return 'generating';
  }
  if (lower.includes('stream')) {
    return 'streaming';
  }
  return 'generating';
}

function askPhaseLabel(phase: AskPhase, answer: string) {
  if (answer && phase !== 'failed') {
    return 'Receiving answer';
  }
  switch (phase) {
    case 'connecting':
      return 'Connecting';
    case 'retrieving':
      return 'Retrieving context';
    case 'generating':
      return 'Waiting for model';
    case 'streaming':
      return 'Receiving answer';
    case 'complete':
      return 'Complete';
    case 'failed':
      return 'Request failed';
    case 'idle':
      return 'Starting';
  }
}

function askPhaseStepClass(current: AskPhase, step: AskPhase) {
  if (current === 'failed') {
    return 'answerPhase answerPhaseFailed';
  }
  if (current === 'complete') {
    return 'answerPhase answerPhaseDone';
  }
  const currentIndex = askPhaseSteps.findIndex((item) => item.phase === current);
  const stepIndex = askPhaseSteps.findIndex((item) => item.phase === step);
  if (currentIndex < 0 || stepIndex < 0) {
    return 'answerPhase';
  }
  if (stepIndex < currentIndex) {
    return 'answerPhase answerPhaseDone';
  }
  if (stepIndex === currentIndex) {
    return 'answerPhase answerPhaseActive';
  }
  return 'answerPhase';
}

function askProgressDetailLabel(input: {
  answer: string;
  elapsedSeconds: number;
  phase: AskPhase;
  status: string;
}) {
  if (input.phase === 'failed') {
    return 'Ready to retry';
  }
  if (input.answer) {
    const rate = input.elapsedSeconds > 0 ? input.answer.length / input.elapsedSeconds : 0;
    const rateLabel = rate >= 1 ? ` / ${Math.round(rate).toLocaleString()} chars/sec` : '';
    return `${input.answer.length.toLocaleString()} chars received${rateLabel}`;
  }
  if (input.elapsedSeconds >= 105) {
    return 'No tokens yet';
  }
  if (input.elapsedSeconds >= 90) {
    return 'Still waiting on the model';
  }
  if (input.elapsedSeconds >= 45) {
    return 'Model warm-up';
  }
  return input.status || askWaitingLabel(input.phase);
}

function askProgressHintLabel(input: {
  answer: string;
  asking: boolean;
  elapsedSeconds: number;
  phase: AskPhase;
}): AskProgressHint | null {
  if (!input.asking || input.phase === 'failed' || input.phase === 'complete') {
    return null;
  }
  if (!input.answer && input.elapsedSeconds >= 105) {
    return {
      detail: 'Gateway connection is still open.',
      label: 'No tokens yet',
      tone: 'risk',
    };
  }
  if (!input.answer && input.elapsedSeconds >= 45) {
    return {
      detail: 'Local model warm-up is taking longer than usual.',
      label: 'Model warm-up',
      tone: 'slow',
    };
  }
  if (input.answer && input.elapsedSeconds >= 150) {
    return {
      detail: `${input.answer.length.toLocaleString()} chars received.`,
      label: 'Streaming slowly',
      tone: 'slow',
    };
  }
  return null;
}

function askWaitingLabel(phase: AskPhase) {
  switch (phase) {
    case 'connecting':
      return 'Opening model stream';
    case 'retrieving':
      return 'Finding relevant context';
    case 'generating':
      return 'Waiting for the first token';
    case 'streaming':
      return 'Receiving answer';
    default:
      return 'Working';
  }
}

function sampleFlowStatusLabel(
  state: SampleFlowState,
  document: ListDocumentsResponse['documents'][number] | null,
) {
  if (state === 'creating') {
    return 'creating workspace';
  }
  if (state === 'uploading') {
    return 'adding sample';
  }
  if (state === 'indexing') {
    return 'indexing';
  }
  if (state === 'ready' || document?.status === 'ready') {
    return 'ready';
  }
  if (state === 'failed' || document?.status === 'failed') {
    return 'needs review';
  }
  return 'empty workspace';
}

function sampleFlowPrimaryLabel(
  state: SampleFlowState,
  document: ListDocumentsResponse['documents'][number] | null,
) {
  if (state === 'failed' || document?.status === 'failed') {
    return 'Retry sample';
  }
  if (state === 'creating') {
    return 'Creating';
  }
  if (state === 'uploading') {
    return 'Adding';
  }
  if (state === 'indexing' || (document && document.status !== 'ready')) {
    return 'Indexing';
  }
  if (document?.status === 'ready' || state === 'ready') {
    return 'Sample ready';
  }
  return 'Add sample';
}

function sampleFlowButtonLabel(state: SampleFlowState) {
  switch (state) {
    case 'creating':
      return 'Creating';
    case 'uploading':
      return 'Adding';
    case 'indexing':
      return 'Indexing';
    default:
      return 'Working';
  }
}

function sampleStepClass(state: SampleFlowState, ready: boolean, failed = false) {
  if (ready || state === 'ready') {
    return 'sampleStepButton sampleStepButtonReady';
  }
  if (failed || state === 'failed') {
    return 'sampleStepButton sampleStepButtonFailed';
  }
  if (state !== 'idle') {
    return 'sampleStepButton sampleStepButtonActive';
  }
  return 'sampleStepButton';
}

function sourceScheduleLabel(minutes: number) {
  if (!minutes) {
    return 'Manual';
  }
  if (minutes === 15) {
    return '15 min';
  }
  if (minutes === 60) {
    return 'Hourly';
  }
  if (minutes === 1440) {
    return 'Daily';
  }
  if (minutes === 10080) {
    return 'Weekly';
  }
  if (minutes % 1440 === 0) {
    return `${minutes / 1440} days`;
  }
  if (minutes % 60 === 0) {
    return `${minutes / 60} hours`;
  }
  return `${minutes} min`;
}

function titleCase(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function messageFromError(err: unknown) {
  const message = err instanceof Error ? err.message : 'Unknown API error';
  return friendlyErrorMessage(message, err instanceof ApiError ? err : null);
}

function friendlyErrorMessage(message: string, apiError: ApiError | null = null) {
  switch (apiError?.errorClass) {
    case 'starting':
      return 'Model gateway is starting. Nexus will keep checking automatically.';
    case 'loading_timeout':
      return 'Model gateway is still loading or compiling. Nexus will keep checking automatically.';
    case 'gateway_loading':
      return 'Model gateway answered but is not ready yet. Nexus will keep checking automatically.';
    case 'rate_limited':
      return 'Model gateway is rate limited right now. Nexus will retry shortly.';
    case 'not_found':
      return 'Model gateway answered, but the route or model was not found. Check the provider URL and model in Settings.';
    case 'host_not_found':
      return 'Model gateway host was not found. Check the gateway URL for this machine.';
    case 'auth':
      return 'Model gateway rejected the request. Check the gateway API key in Settings.';
    case 'unknown_target':
      return 'Model target was not found. Check the configured default target.';
    case 'invalid_target':
      return 'Model target is invalid. Check the configured target name and model.';
  }

  const lower = message.toLowerCase();
  if (lower.includes('model gateway') && lower.includes('404')) {
    return 'Model gateway answered, but the route or model was not found. Check the provider URL and model in Settings.';
  }
  if (lower.includes('model gateway') && lower.includes('status')) {
    return 'Model gateway answered with an error. It may still be loading; check Settings if this does not clear.';
  }
  if (
    lower.includes('context deadline') ||
    lower.includes('timed out') ||
    lower.includes('timeout')
  ) {
    return 'Model gateway timed out. It may still be loading; wait a minute, then recheck.';
  }
  if (lower.includes('connection refused') || lower.includes('actively refused')) {
    return 'Model gateway is starting or not listening yet. Wait a minute, then recheck.';
  }
  if (lower.includes('no such host')) {
    return 'Model gateway host was not found. Check the gateway URL for this machine.';
  }
  if (lower.includes('failed to fetch')) {
    return 'Cannot reach the API. Check that Nexus Local is running.';
  }
  if (lower.includes('workspace still has active documents or sources')) {
    return 'Delete documents and archive sources before deleting this workspace.';
  }
  return message;
}

function hasActiveIngestionJob(detail: DocumentDetailResponse) {
  return detail.jobs.some(
    (job) =>
      job.type === 'document_ingestion' &&
      job.resource_type === 'document' &&
      job.resource_id === detail.document.id &&
      isActiveJobState(job.state),
  );
}

function emptyDataSourceScanSummary(): DataSourceDetailResponse['scan_summary'] {
  return {
    total: 0,
    imported: 0,
    skipped: 0,
    failed: 0,
    deleted: 0,
    reasons: {},
  };
}

function emptyDataSourceScanEntryPage(): DataSourceDetailResponse['scan_entries_page'] {
  return {
    total: 0,
    limit: scanEntryPageSize,
    offset: 0,
    has_more: false,
  };
}

function sourceScanEntryOptions(filter: ScanEntryFilter, offset: number) {
  return {
    scan_entry_limit: scanEntryPageSize,
    scan_entry_offset: Math.max(0, offset),
    scan_entry_outcome: filter === 'all' ? undefined : filter,
  };
}

function scanEntryPageStart(page: DataSourceDetailResponse['scan_entries_page']) {
  if (page.total === 0) {
    return 0;
  }
  return page.offset + 1;
}

function scanEntryPageEnd(
  page: DataSourceDetailResponse['scan_entries_page'],
  visibleCount: number,
) {
  if (page.total === 0) {
    return 0;
  }
  return Math.min(page.offset + visibleCount, page.total);
}

function sourceScanMetrics(summary: DataSourceDetailResponse['scan_summary']): ScanMetric[] {
  const metrics: ScanMetric[] = [
    {
      key: 'all',
      label: 'Files',
      count: summary.total,
      filter: 'all',
    },
    {
      key: 'imported',
      label: 'Imported',
      count: summary.imported,
      filter: 'imported',
    },
    {
      key: 'skipped',
      label: 'Skipped',
      count: summary.skipped,
      filter: 'skipped',
    },
    {
      key: 'failed',
      label: 'Failed',
      count: summary.failed,
      filter: 'failed',
    },
  ];
  if (summary.deleted > 0) {
    metrics.push({
      key: 'deleted',
      label: 'Deleted',
      count: summary.deleted,
      filter: 'deleted',
    });
  }
  return metrics;
}

function sourceScanMixSegments(summary: DataSourceDetailResponse['scan_summary']) {
  return [
    {
      key: 'imported',
      label: 'Imported',
      count: summary.imported,
    },
    {
      key: 'skipped',
      label: 'Skipped',
      count: summary.skipped,
    },
    {
      key: 'failed',
      label: 'Failed',
      count: summary.failed,
    },
    {
      key: 'deleted',
      label: 'Deleted',
      count: summary.deleted,
    },
  ].filter((segment) => segment.count > 0);
}

function sourceScanMixLabel(summary: DataSourceDetailResponse['scan_summary']) {
  return sourceScanMixSegments(summary)
    .map((segment) => `${segment.label} ${segment.count}`)
    .join(', ');
}

function sourceScanHeadline(detail: DataSourceDetailResponse) {
  const summary = detail.scan_summary;
  if (summary.failed > 0 || detail.source.status === 'failed') {
    return `${summary.failed} failed in latest scan`;
  }
  if (summary.imported > 0 && summary.skipped === 0 && summary.deleted === 0) {
    return `${summary.imported} imported`;
  }
  if (summary.imported === 0 && summary.skipped > 0 && summary.deleted === 0) {
    return `${summary.skipped} skipped`;
  }
  const parts = [`${summary.imported} imported`];
  if (summary.skipped > 0) {
    parts.push(`${summary.skipped} skipped`);
  }
  if (summary.deleted > 0) {
    parts.push(`${summary.deleted} deleted`);
  }
  return parts.join(', ');
}

function sourceScanSubline(detail: DataSourceDetailResponse) {
  const summary = detail.scan_summary;
  const parts = [];
  if (summary.latest_at) {
    parts.push(formatDateTime(summary.latest_at));
  } else if (detail.source.last_scan_at) {
    parts.push(formatDateTime(detail.source.last_scan_at));
  }
  if (summary.latest_job_id) {
    parts.push(summary.latest_job_id);
  }
  if (detail.failed_documents > 0) {
    parts.push(`${detail.failed_documents} failed docs`);
  }
  return parts.join(' / ') || 'Latest scan';
}

function sourceScanReasonRows(summary: DataSourceDetailResponse['scan_summary']) {
  return Object.entries(summary.reasons)
    .sort(([, a], [, b]) => b - a)
    .map(([reason, count]) => ({
      reason,
      label: scanReasonLabel(reason),
      count,
      filter: scanReasonFilter(reason),
    }));
}

function scanSummaryReasons(summary: DataSourceDetailResponse['scan_summary']) {
  return Object.entries(summary.reasons)
    .sort(([, a], [, b]) => b - a)
    .slice(0, 2)
    .map(([reason, count]) => `${scanReasonLabel(reason)} ${count}`)
    .join(', ');
}

function scanReasonLabel(reason: string) {
  if (reason.trim() === '') {
    return 'Other';
  }
  return titleCase(reason.replace(/_/g, ' '));
}

function scanReasonFilter(reason: string): ScanEntryFilter {
  if (reason === 'changed') {
    return 'imported';
  }
  if (reason === 'missing') {
    return 'deleted';
  }
  if (
    [
      'cannot_access',
      'not_directory',
      'walk_error',
      'stat_failed',
      'hash_failed',
      'open_failed',
      'upload_failed',
      'close_failed',
      'replace_failed',
      'delete_missing_failed',
    ].includes(reason)
  ) {
    return 'failed';
  }
  return 'skipped';
}

function sourceFailureTotal(detail: DataSourceDetailResponse) {
  return Math.max(detail.source.last_scan_failed ?? 0, detail.scan_summary.failed ?? 0);
}

function sourceNeedsRecovery(detail: DataSourceDetailResponse) {
  return detail.source.status === 'failed' || sourceFailureTotal(detail) > 0;
}

function sourceRecoveryMessage(detail: DataSourceDetailResponse) {
  const reason = scanSummaryReasons(detail.scan_summary);
  if (reason) {
    return `${reason}. Fix the source, then retry; unchanged files will be skipped.`;
  }
  if (detail.source.status === 'failed') {
    return 'Fix the source path, mount, or permissions, then retry the scan.';
  }
  return 'Review failed files, fix the source, then retry the scan.';
}

function sourceCurrentScanProgress(
  detail: DataSourceDetailResponse,
  job?: ListJobsResponse['jobs'][number],
) {
  if (!job) {
    return 'Waiting for scan activity';
  }
  if (detail.scan_summary.latest_job_id !== job.id) {
    return 'Waiting for first file result';
  }
  const total = detail.scan_summary.total;
  if (total === 0) {
    return 'Waiting for first file result';
  }
  const parts = [`${total} seen`];
  if (detail.scan_summary.imported > 0) {
    parts.push(`${detail.scan_summary.imported} imported`);
  }
  if (detail.scan_summary.skipped > 0) {
    parts.push(`${detail.scan_summary.skipped} skipped`);
  }
  if (detail.scan_summary.failed > 0) {
    parts.push(`${detail.scan_summary.failed} failed`);
  }
  return parts.join(' / ');
}

function SourceHealthRollup({
  activePlanJob,
  activePreflightJob,
  activeScanJob,
  detail,
}: {
  activePlanJob?: ListJobsResponse['jobs'][number];
  activePreflightJob?: ListJobsResponse['jobs'][number];
  activeScanJob?: ListJobsResponse['jobs'][number];
  detail: DataSourceDetailResponse;
}) {
  const items = sourceHealthItems(detail, activePreflightJob, activePlanJob, activeScanJob);
  return (
    <section className="sourceHealthRollup" aria-label="Source health">
      {items.map((item) => (
        <div className="sourceHealthItem" key={item.key}>
          <div className="sourceHealthItemHeader">
            <small>{item.label}</small>
            <span className={sourceHealthBadgeClass(item.state)}>
              {sourceHealthBadgeLabel(item.state)}
            </span>
          </div>
          <strong>{item.value}</strong>
          <em title={item.detail}>{item.detail}</em>
        </div>
      ))}
    </section>
  );
}

function sourceHealthItems(
  detail: DataSourceDetailResponse,
  activePreflightJob?: ListJobsResponse['jobs'][number],
  activePlanJob?: ListJobsResponse['jobs'][number],
  activeScanJob?: ListJobsResponse['jobs'][number],
): SourceHealthItem[] {
  const latestPreflightJob = activePreflightJob ?? sourceLatestRelevantPreflightJob(detail);
  const latestPlanJob = activePlanJob ?? sourceLatestRelevantPlanJob(detail);
  const latestScanJob = activeScanJob ?? sourceLatestScanJob(detail);
  return [
    sourceReadinessHealth(detail, latestPreflightJob, latestPlanJob, latestScanJob),
    sourceScheduleHealth(detail, activeScanJob),
    sourceFailureHealth(detail),
    sourceNextActionHealth(detail, latestPreflightJob, latestPlanJob, latestScanJob),
  ];
}

function sourceReadinessHealth(
  detail: DataSourceDetailResponse,
  preflightJob?: ListJobsResponse['jobs'][number],
  planJob?: ListJobsResponse['jobs'][number],
  scanJob?: ListJobsResponse['jobs'][number],
): SourceHealthItem {
  if (preflightJob && isActiveJobState(preflightJob.state)) {
    return {
      key: 'state',
      label: 'State',
      value: 'Checking path',
      detail: 'Worker is validating the mounted source path.',
      state: 'active',
    };
  }
  if (planJob && isActiveJobState(planJob.state)) {
    return {
      key: 'state',
      label: 'State',
      value: 'Planning import',
      detail: 'Worker is previewing files before the next scan.',
      state: 'active',
    };
  }
  if (scanJob && isActiveJobState(scanJob.state)) {
    return {
      key: 'state',
      label: 'State',
      value: 'Scanning',
      detail: sourceCurrentScanProgress(detail, scanJob),
      state: 'active',
    };
  }
  if (preflightJob && sourcePreflightBlocksScan(detail.source, preflightJob)) {
    return {
      key: 'state',
      label: 'State',
      value: 'Path blocked',
      detail: sourcePreflightMessage(preflightJob),
      state: 'blocked',
    };
  }
  if (planJob && sourcePlanBlocksScan(detail.source, planJob)) {
    return {
      key: 'state',
      label: 'State',
      value: 'Plan blocked',
      detail: sourcePlanMessage(planJob, sourcePlanSummaryFromJob(planJob)),
      state: 'blocked',
    };
  }
  if (scanJob?.state === 'failed') {
    return {
      key: 'state',
      label: 'State',
      value: 'Scan failed',
      detail: sourceScanRunMessage(detail, scanJob, sourceScanRunSummaryFromJob(scanJob)),
      state: 'blocked',
    };
  }
  if (detail.source.status === 'archived') {
    return {
      key: 'state',
      label: 'State',
      value: 'Archived',
      detail: 'Source is no longer maintained automatically.',
      state: 'neutral',
    };
  }
  if (!sourceImportHasStarted(detail.source)) {
    return {
      key: 'state',
      label: 'State',
      value: 'Not scanned',
      detail: 'Run Check path, Plan, then Scan when ready.',
      state: 'review',
    };
  }
  if (detail.failed_documents > 0) {
    return {
      key: 'state',
      label: 'State',
      value: 'Needs review',
      detail: `${detail.failed_documents} imported documents have failed ingestion jobs.`,
      state: 'review',
    };
  }
  return {
    key: 'state',
    label: 'State',
    value: 'Healthy',
    detail: 'Latest source state is usable.',
    state: 'ready',
  };
}

function sourceScheduleHealth(
  detail: DataSourceDetailResponse,
  activeScanJob?: ListJobsResponse['jobs'][number],
): SourceHealthItem {
  const interval = detail.source.scan_interval_minutes ?? 0;
  if (!interval) {
    return {
      key: 'schedule',
      label: 'Schedule',
      value: 'Manual',
      detail: 'Scans run only when requested.',
      state: 'neutral',
    };
  }
  if (activeScanJob && isActiveJobState(activeScanJob.state)) {
    return {
      key: 'schedule',
      label: 'Schedule',
      value: 'Running now',
      detail: `${sourceScheduleLabel(interval)} cadence resumes after this run.`,
      state: 'active',
    };
  }
  if (!detail.source.next_scan_at) {
    return {
      key: 'schedule',
      label: 'Schedule',
      value: sourceScheduleLabel(interval),
      detail: 'No next run is currently scheduled.',
      state: 'review',
    };
  }
  const nextScanAt = Date.parse(detail.source.next_scan_at);
  if (Number.isNaN(nextScanAt)) {
    return {
      key: 'schedule',
      label: 'Schedule',
      value: sourceScheduleLabel(interval),
      detail: 'Next run timestamp is not readable.',
      state: 'review',
    };
  }
  const now = Date.now();
  if (nextScanAt <= now) {
    return {
      key: 'schedule',
      label: 'Schedule',
      value: nextScanAt <= now - 5 * 60 * 1000 ? 'Overdue' : 'Due now',
      detail: `Due ${formatRelativeDateTime(detail.source.next_scan_at)} / ${sourceScheduleLabel(
        interval,
      )} cadence`,
      state: 'review',
    };
  }
  return {
    key: 'schedule',
    label: 'Schedule',
    value: `Next ${formatRelativeDateTime(detail.source.next_scan_at)}`,
    detail: `${sourceScheduleLabel(interval)} cadence.`,
    state: 'ready',
  };
}

function sourceFailureHealth(detail: DataSourceDetailResponse): SourceHealthItem {
  const consecutiveFailures = sourceConsecutiveFailedOperationCount(detail);
  if (consecutiveFailures >= 2) {
    const latestFailure = sourceRecentOperationalJobs(detail).find((job) => job.state === 'failed');
    return {
      key: 'failures',
      label: 'Failures',
      value: 'Repeated failures',
      detail: `${consecutiveFailures} recent operations failed. ${
        latestFailure?.error_message || 'Open the activity rows for details.'
      }`,
      state: 'blocked',
    };
  }
  if (detail.failed_documents > 0) {
    return {
      key: 'failures',
      label: 'Failures',
      value: `${detail.failed_documents} failed docs`,
      detail: 'Retry failed documents after fixing parser or runtime issues.',
      state: 'review',
    };
  }
  const lastScanFailed = detail.source.last_scan_failed ?? 0;
  if (lastScanFailed > 0) {
    return {
      key: 'failures',
      label: 'Failures',
      value: `${lastScanFailed} scan failures`,
      detail: 'Review failed files in the latest scan report.',
      state: 'review',
    };
  }
  return {
    key: 'failures',
    label: 'Failures',
    value: 'None',
    detail: 'No source or document failures are currently visible.',
    state: 'ready',
  };
}

function sourceNextActionHealth(
  detail: DataSourceDetailResponse,
  preflightJob?: ListJobsResponse['jobs'][number],
  planJob?: ListJobsResponse['jobs'][number],
  scanJob?: ListJobsResponse['jobs'][number],
): SourceHealthItem {
  if (
    [preflightJob, planJob, scanJob].some((job) => job && isActiveJobState(job.state))
  ) {
    return {
      key: 'action',
      label: 'Next action',
      value: 'Monitor',
      detail: 'Refresh the source detail while the worker finishes.',
      state: 'active',
    };
  }
  if (detail.source.status === 'archived') {
    return {
      key: 'action',
      label: 'Next action',
      value: 'No action',
      detail: 'Archived sources are retained for document history.',
      state: 'neutral',
    };
  }
  if (preflightJob && sourcePreflightBlocksScan(detail.source, preflightJob)) {
    return {
      key: 'action',
      label: 'Next action',
      value: 'Check path',
      detail: 'Fix the mount or permissions, then run Check path again.',
      state: 'blocked',
    };
  }
  if (planJob && sourcePlanBlocksScan(detail.source, planJob)) {
    return {
      key: 'action',
      label: 'Next action',
      value: 'Plan again',
      detail: 'Fix plan errors, then preview the source again.',
      state: 'blocked',
    };
  }
  if (!sourceImportHasStarted(detail.source)) {
    if (!preflightJob || preflightJob.state !== 'succeeded') {
      return {
        key: 'action',
        label: 'Next action',
        value: 'Check path',
        detail: 'Validate the worker can read the mounted source.',
        state: 'review',
      };
    }
    if (!planJob || planJob.state !== 'succeeded') {
      return {
        key: 'action',
        label: 'Next action',
        value: 'Plan',
        detail: 'Preview matched files before the first import.',
        state: 'review',
      };
    }
    return {
      key: 'action',
      label: 'Next action',
      value: 'Scan',
      detail: 'The source is ready for the first import.',
      state: 'ready',
    };
  }
  if (detail.failed_documents > 0) {
    return {
      key: 'action',
      label: 'Next action',
      value: 'Retry failed docs',
      detail: 'Requeue failed ingestion jobs from the source actions.',
      state: 'review',
    };
  }
  if (sourceScheduleIsOverdue(detail.source)) {
    return {
      key: 'action',
      label: 'Next action',
      value: 'Run scan',
      detail: 'A scheduled source appears overdue; run a scan or check the worker.',
      state: 'review',
    };
  }
  return {
    key: 'action',
    label: 'Next action',
    value: 'Monitor',
    detail: 'No operator action is required right now.',
    state: 'ready',
  };
}

function sourceRecentOperationalJobs(detail: DataSourceDetailResponse) {
  return detail.jobs
    .filter(
      (job) =>
        ['source_preflight', 'source_plan', 'source_scan'].includes(job.type) &&
        job.resource_type === 'data_source' &&
        job.resource_id === detail.source.id &&
        (job.type === 'source_scan' || sourceJobAppliesToSource(detail.source, job)),
    )
    .sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at));
}

function sourceConsecutiveFailedOperationCount(detail: DataSourceDetailResponse) {
  let count = 0;
  for (const job of sourceRecentOperationalJobs(detail).slice(0, 6)) {
    if (isActiveJobState(job.state)) {
      break;
    }
    if (job.state !== 'failed') {
      break;
    }
    count += 1;
  }
  return count;
}

function sourceScheduleIsOverdue(source: ListDataSourcesResponse['sources'][number]) {
  if (!source.scan_interval_minutes || !source.next_scan_at) {
    return false;
  }
  const nextScanAt = Date.parse(source.next_scan_at);
  return !Number.isNaN(nextScanAt) && nextScanAt <= Date.now() - 5 * 60 * 1000;
}

function sourceHealthBadgeClass(state: SourceHealthState) {
  if (state === 'ready') {
    return stateClass('ready');
  }
  if (state === 'active') {
    return stateClass('running');
  }
  if (state === 'review') {
    return 'stateBadge stateReview';
  }
  if (state === 'blocked') {
    return stateClass('failed');
  }
  return 'stateBadge';
}

function sourceHealthBadgeLabel(state: SourceHealthState) {
  switch (state) {
    case 'ready':
      return 'ok';
    case 'active':
      return 'active';
    case 'review':
      return 'review';
    case 'blocked':
      return 'blocked';
    case 'neutral':
      return 'info';
  }
}

function sourceListHealthLabel(
  source: ListDataSourcesResponse['sources'][number],
  preflightJob?: ListJobsResponse['jobs'][number],
  planJob?: ListJobsResponse['jobs'][number],
  scanJob?: ListJobsResponse['jobs'][number],
) {
  if (sourceIsConnector(source)) {
    return 'Connector handoff';
  }
  if (preflightJob && isActiveJobState(preflightJob.state)) {
    return 'Checking path';
  }
  if (planJob && isActiveJobState(planJob.state)) {
    return 'Planning import';
  }
  if (scanJob && isActiveJobState(scanJob.state)) {
    return 'Scanning source';
  }
  if (preflightJob && sourcePreflightBlocksScan(source, preflightJob)) {
    return 'Path blocked';
  }
  if (planJob && sourcePlanBlocksScan(source, planJob)) {
    return 'Plan blocked';
  }
  if ((source.last_scan_failed ?? 0) > 0) {
    return `${source.last_scan_failed} failed last scan`;
  }
  if (sourceScheduleIsOverdue(source)) {
    return 'Schedule overdue';
  }
  if (!source.last_scan_at) {
    return 'Not scanned';
  }
  return `Last scan ${formatDateTime(source.last_scan_at)}`;
}

function sourceListCountLabel(total: number, filtered: number) {
  if (total === 0 || total === filtered) {
    return `${total} active`;
  }
  return `${filtered}/${total} shown`;
}

function sourceFilterSetIsActive(filters: typeof initialSourceFilters) {
  return Boolean(
    filters.health || filters.query.trim() || filters.schedule || filters.type,
  );
}

function sourceFiltersEqual(left: SourceFilterValues, right: SourceFilterValues) {
  return (
    left.health === right.health &&
    left.query.trim() === right.query.trim() &&
    left.schedule === right.schedule &&
    left.type === right.type
  );
}

function normalizeSourceFilters(filters: Partial<SourceFilterValues>): SourceFilterValues {
  return {
    health: typeof filters.health === 'string' ? filters.health : '',
    query: typeof filters.query === 'string' ? filters.query : '',
    schedule: typeof filters.schedule === 'string' ? filters.schedule : '',
    type: typeof filters.type === 'string' ? filters.type : '',
  };
}

function sourceSavedViewLabel(view: SourceSavedView) {
  return view.name.trim() || 'Untitled view';
}

function sortSourceViews(views: SourceSavedView[]) {
  return [...views].sort((left, right) => {
    const nameCompare = sourceSavedViewLabel(left).localeCompare(
      sourceSavedViewLabel(right),
      undefined,
      { sensitivity: 'base' },
    );
    if (nameCompare !== 0) {
      return nameCompare;
    }
    return left.id.localeCompare(right.id);
  });
}

function sourcePolicyProfileToOption(profile: PersistedSourcePolicyProfile): SourcePolicyProfile {
  return {
    id: profile.id,
    label: profile.name,
    detail: profile.detail,
    include_patterns: profile.include_patterns.join('\n'),
    exclude_patterns: profile.exclude_patterns.join('\n'),
    scan_interval_minutes: String(profile.scan_interval_minutes),
    persisted: true,
  };
}

function sortSourcePolicyProfiles(profiles: SourcePolicyProfile[]) {
  return [...profiles].sort((left, right) => {
    const nameCompare = left.label.localeCompare(right.label, undefined, { sensitivity: 'base' });
    if (nameCompare !== 0) {
      return nameCompare;
    }
    return left.id.localeCompare(right.id);
  });
}

function sourceBulkEligibilityCounts(
  sources: ListDataSourcesResponse['sources'],
  activePreflightJobs: Map<string, ListJobsResponse['jobs'][number]>,
  activePlanJobs: Map<string, ListJobsResponse['jobs'][number]>,
  activeScanJobs: Map<string, ListJobsResponse['jobs'][number]>,
  latestPreflightJobs: Map<string, ListJobsResponse['jobs'][number]>,
  latestPlanJobs: Map<string, ListJobsResponse['jobs'][number]>,
): Record<SourceBulkAction, number> {
  return {
    preflight: sources.filter((source) =>
      sourceBulkActionIsEligible(
        'preflight',
        source,
        activePreflightJobs,
        activePlanJobs,
        activeScanJobs,
        latestPreflightJobs,
        latestPlanJobs,
      ),
    ).length,
    plan: sources.filter((source) =>
      sourceBulkActionIsEligible(
        'plan',
        source,
        activePreflightJobs,
        activePlanJobs,
        activeScanJobs,
        latestPreflightJobs,
        latestPlanJobs,
      ),
    ).length,
    scan: sources.filter((source) =>
      sourceBulkActionIsEligible(
        'scan',
        source,
        activePreflightJobs,
        activePlanJobs,
        activeScanJobs,
        latestPreflightJobs,
        latestPlanJobs,
      ),
    ).length,
    retry_failures: sources.filter((source) =>
      sourceBulkActionIsEligible(
        'retry_failures',
        source,
        activePreflightJobs,
        activePlanJobs,
        activeScanJobs,
        latestPreflightJobs,
        latestPlanJobs,
      ),
    ).length,
    reindex: sources.filter((source) =>
      sourceBulkActionIsEligible(
        'reindex',
        source,
        activePreflightJobs,
        activePlanJobs,
        activeScanJobs,
        latestPreflightJobs,
        latestPlanJobs,
      ),
    ).length,
  };
}

function sourceBulkActionIsEligible(
  action: SourceBulkAction,
  source: ListDataSourcesResponse['sources'][number],
  activePreflightJobs: Map<string, ListJobsResponse['jobs'][number]>,
  activePlanJobs: Map<string, ListJobsResponse['jobs'][number]>,
  activeScanJobs: Map<string, ListJobsResponse['jobs'][number]>,
  latestPreflightJobs: Map<string, ListJobsResponse['jobs'][number]>,
  latestPlanJobs: Map<string, ListJobsResponse['jobs'][number]>,
) {
  if (source.status === 'archived' || sourceIsConnector(source)) {
    return false;
  }
  const activePreflightJob = activePreflightJobs.get(source.id);
  const activePlanJob = activePlanJobs.get(source.id);
  const activeScanJob = activeScanJobs.get(source.id);
  if (activePreflightJob || activePlanJob || activeScanJob) {
    return false;
  }
  const latestPreflightJob = latestPreflightJobs.get(source.id);
  const latestPlanJob = latestPlanJobs.get(source.id);
  const preflightBlocksScan = sourcePreflightBlocksScan(source, latestPreflightJob);
  const planBlocksScan = sourcePlanBlocksScan(source, latestPlanJob);

  if (action === 'preflight') {
    return true;
  }
  if (action === 'retry_failures') {
    return (source.last_scan_failed ?? 0) > 0;
  }
  if (action === 'reindex') {
    return sourceImportHasStarted(source);
  }
  if (preflightBlocksScan || planBlocksScan) {
    return false;
  }
  if (action === 'plan') {
    return true;
  }
  return sourceFirstScanReviewPrompt(source, latestPlanJob) === '';
}

function sourceBulkActionVerb(action: SourceBulkAction) {
  if (action === 'preflight') {
    return 'Check paths';
  }
  if (action === 'plan') {
    return 'Plan imports';
  }
  if (action === 'scan') {
    return 'Rescan ready';
  }
  if (action === 'retry_failures') {
    return 'Retry failures';
  }
  return 'Reindex docs';
}

function sourceBulkActionButtonLabel(action: SourceBulkAction, count: number) {
  return `${sourceBulkActionVerb(action)} (${count})`;
}

function sourceBulkActionResultLabel(action: SourceBulkAction, count: number) {
  if (action === 'preflight') {
    return count === 1 ? 'path check queued' : 'path checks queued';
  }
  if (action === 'plan') {
    return count === 1 ? 'import plan queued' : 'import plans queued';
  }
  if (action === 'scan') {
    return count === 1 ? 'scan queued' : 'scans queued';
  }
  if (action === 'retry_failures') {
    return count === 1 ? 'failure retry queued' : 'failure retries queued';
  }
  return count === 1 ? 'reindex queued' : 'reindexes queued';
}

function filterDataSources(
  sources: ListDataSourcesResponse['sources'],
  filters: typeof initialSourceFilters,
  activePreflightJobs: Map<string, ListJobsResponse['jobs'][number]>,
  activePlanJobs: Map<string, ListJobsResponse['jobs'][number]>,
  activeScanJobs: Map<string, ListJobsResponse['jobs'][number]>,
  latestPreflightJobs: Map<string, ListJobsResponse['jobs'][number]>,
  latestPlanJobs: Map<string, ListJobsResponse['jobs'][number]>,
) {
  const query = filters.query.trim().toLowerCase();
  return sources.filter((source) => {
    const preflightJob = activePreflightJobs.get(source.id) ?? latestPreflightJobs.get(source.id);
    const planJob = activePlanJobs.get(source.id) ?? latestPlanJobs.get(source.id);
    const scanJob = activeScanJobs.get(source.id);
    if (filters.type && source.type !== filters.type) {
      return false;
    }
    if (filters.schedule && sourceScheduleFilterValue(source) !== filters.schedule) {
      return false;
    }
    if (
      filters.health &&
      sourceHealthFilterValue(source, preflightJob, planJob, scanJob) !== filters.health
    ) {
      return false;
    }
    if (!query) {
      return true;
    }
    const haystack = [
      source.name,
      source.root_path,
      source.type,
      sourceTypeLabel(source.type),
      sourceScheduleLabel(source.scan_interval_minutes ?? 0),
      sourceScanSummary(source),
      sourceListHealthLabel(source, preflightJob, planJob, scanJob),
    ]
      .join(' ')
      .toLowerCase();
    return haystack.includes(query);
  });
}

function sourceHealthFilterValue(
  source: ListDataSourcesResponse['sources'][number],
  preflightJob?: ListJobsResponse['jobs'][number],
  planJob?: ListJobsResponse['jobs'][number],
  scanJob?: ListJobsResponse['jobs'][number],
) {
  if (source.status === 'archived') {
    return 'archived';
  }
  if (
    [preflightJob, planJob, scanJob].some((job) => job && isActiveJobState(job.state))
  ) {
    return 'active';
  }
  if (
    source.status === 'failed' ||
    scanJob?.state === 'failed' ||
    (preflightJob && sourcePreflightBlocksScan(source, preflightJob)) ||
    (planJob && sourcePlanBlocksScan(source, planJob))
  ) {
    return 'blocked';
  }
  if (
    !sourceImportHasStarted(source) ||
    (source.last_scan_failed ?? 0) > 0 ||
    sourceScheduleIsOverdue(source)
  ) {
    return 'review';
  }
  return 'healthy';
}

function sourceScheduleFilterValue(source: ListDataSourcesResponse['sources'][number]) {
  if (sourceScheduleIsOverdue(source)) {
    return 'overdue';
  }
  if (!source.scan_interval_minutes) {
    return 'manual';
  }
  return 'scheduled';
}

function SourceScanRunStatus({
  activeJob,
  cancelDisabled,
  canceling,
  detail,
  onCancel,
  onRefresh,
  refreshing,
}: {
  activeJob?: ListJobsResponse['jobs'][number];
  cancelDisabled: boolean;
  canceling: boolean;
  detail: DataSourceDetailResponse;
  onCancel: () => void;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const job = activeJob ?? sourceLatestScanJob(detail);
  if (!job) {
    return null;
  }
  const active = isActiveJobState(job.state);
  const failed = job.state === 'failed';
  const run = sourceScanRunSummaryFromJob(job);
  return (
    <section
      aria-label="Source scan run"
      className={
        failed
          ? 'sourceProgressPanel sourceScanRunPanel sourcePreflightFailed'
          : 'sourceProgressPanel sourceScanRunPanel'
      }
    >
      {active ? (
        <span className="spinner" aria-hidden="true" />
      ) : (
        <span className={stateClass(job.state)}>{titleCase(job.state)}</span>
      )}
      <div className="sourceProgressText">
        <strong>{sourceScanRunTitle(job)}</strong>
        <span>{sourceScanRunMessage(detail, job, run)}</span>
        <div className="sourceScanRunMetrics">
          {sourceScanRunMetrics(detail, job, run).map((metric) => (
            <span key={metric.label}>
              <strong>{metric.value}</strong>
              <em>{metric.label}</em>
            </span>
          ))}
        </div>
      </div>
      <div className="sourceRunActions">
        <button disabled={refreshing} onClick={onRefresh} type="button">
          {refreshing ? 'Refreshing' : 'Refresh'}
        </button>
        {active && (
          <button
            className="dangerButton"
            disabled={cancelDisabled || canceling}
            onClick={onCancel}
            type="button"
          >
            {canceling ? 'Canceling' : 'Cancel'}
          </button>
        )}
      </div>
    </section>
  );
}

function SourceScanRunHistory({ detail }: { detail: DataSourceDetailResponse }) {
  const jobs = sourceScanJobs(detail).slice(0, 5);
  if (jobs.length <= 1) {
    return null;
  }
  return (
    <details className="inlineDetails sourceRunHistory" open>
      <summary>Runs</summary>
      <div className="sourceRunHistoryTable" aria-label="Source scan run history">
        <div className="sourceRunHistoryHeader">
          <span>Run</span>
          <span>State</span>
          <span>Imported</span>
          <span>Skipped</span>
          <span>Deleted</span>
          <span>Failed</span>
          <span>Duration</span>
        </div>
        {jobs.map((job) => {
          const run = sourceScanRunSummaryFromJob(job);
          const metrics = sourceScanRunMetricMap(detail, job, run);
          return (
            <div className="sourceRunHistoryRow" key={job.id}>
              <strong title={job.id}>{sourceScanRunHistoryLabel(job, run)}</strong>
              <span className={stateClass(job.state)}>{job.state}</span>
              <em>{metrics.imported}</em>
              <em>{metrics.skipped}</em>
              <em>{metrics.deleted}</em>
              <em>{metrics.failed}</em>
              <small>{sourceScanRunDurationLabel(job, run)}</small>
            </div>
          );
        })}
      </div>
    </details>
  );
}

function SourcePlanStatus({
  activeJob,
  detail,
  onRefresh,
  refreshing,
}: {
  activeJob?: ListJobsResponse['jobs'][number];
  detail: DataSourceDetailResponse;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const job = activeJob ?? sourceLatestRelevantPlanJob(detail);
  if (!job) {
    return null;
  }
  const active = isActiveJobState(job.state);
  const failed = job.state === 'failed';
  const summary = sourcePlanSummaryFromJob(job);
  const reviewReasons =
    summary && !sourceImportHasStarted(detail.source) ? sourcePlanReviewReasons(summary) : [];
  return (
    <section
      aria-label="Source import plan"
      className={
        failed
          ? 'sourceProgressPanel sourcePlanPanel sourcePreflightFailed'
          : 'sourceProgressPanel sourcePlanPanel'
      }
    >
      {active ? (
        <span className="spinner" aria-hidden="true" />
      ) : (
        <span className={stateClass(job.state)}>{titleCase(job.state)}</span>
      )}
      <div className="sourceProgressText">
        <strong>{sourcePlanTitle(job)}</strong>
        <span>{sourcePlanMessage(job, summary)}</span>
        {summary && (
          <>
            <div className="sourcePlanMetrics" aria-label="Import plan totals">
              <span>
                <strong>{summary.would_import}</strong>
                <em>Would import</em>
              </span>
              <span>
                <strong>{summary.skipped}</strong>
                <em>Skipped</em>
              </span>
              <span>
                <strong>{summary.failed}</strong>
                <em>Failed</em>
              </span>
              <span>
                <strong>{formatBytes(summary.estimated_bytes)}</strong>
                <em>Estimate</em>
              </span>
            </div>
            {reviewReasons.length > 0 && (
              <div className="sourcePlanReview">
                <strong>Review before scan</strong>
                <em>{reviewReasons.slice(0, 2).join(' / ')}</em>
              </div>
            )}
            <SourcePlanReviewTable summary={summary} />
          </>
        )}
      </div>
      <button disabled={refreshing} onClick={onRefresh} type="button">
        {refreshing ? 'Refreshing' : 'Refresh'}
      </button>
    </section>
  );
}

function sourceLatestScanJob(detail: DataSourceDetailResponse) {
  return sourceScanJobs(detail)[0];
}

function sourceScanJobs(detail: DataSourceDetailResponse) {
  return detail.jobs
    .filter(
      (job) =>
        job.type === 'source_scan' &&
        job.resource_type === 'data_source' &&
        job.resource_id === detail.source.id,
    )
    .sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at));
}

function sourceScanRunSummaryFromJob(job: ListJobsResponse['jobs'][number]) {
  if (!job.result_json) {
    return null;
  }
  try {
    const summary = JSON.parse(job.result_json) as SourceScanRunSummary;
    if (!summary.job_id || !summary.started_at) {
      return null;
    }
    return summary;
  } catch {
    return null;
  }
}

function sourceScanRunTitle(job: ListJobsResponse['jobs'][number]) {
  if (isActiveJobState(job.state)) {
    return 'Scan running';
  }
  if (job.state === 'succeeded') {
    return 'Scan complete';
  }
  if (job.state === 'failed') {
    return 'Scan failed';
  }
  if (job.state === 'canceled') {
    return 'Scan canceled';
  }
  return titleCase(job.state);
}

function sourceScanRunMessage(
  detail: DataSourceDetailResponse,
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  if (isActiveJobState(job.state)) {
    return `${sourceCurrentScanProgress(detail, job)} / started ${formatDateTime(
      sourceScanRunStartedAt(job, run),
    )}`;
  }
  if (job.state === 'failed' && job.error_message.trim() !== '') {
    return job.error_message;
  }
  if (job.state === 'canceled' && job.error_message.trim() !== '') {
    return `${job.error_message} / ${formatDateTime(job.updated_at)}`;
  }
  const startedAt = sourceScanRunStartedAt(job, run);
  const finishedAt = sourceScanRunFinishedAt(job, run);
  const parts = [`started ${formatDateTime(startedAt)}`];
  if (finishedAt) {
    parts.push(`finished ${formatDateTime(finishedAt)}`);
  }
  const duration = sourceScanRunDurationMS(job, run);
  if (duration !== null) {
    parts.push(formatDurationMS(duration));
  }
  return parts.join(' / ');
}

function sourceScanRunMetrics(
  detail: DataSourceDetailResponse,
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  const metrics = sourceScanRunMetricMap(detail, job, run);
  return [
    { label: 'Imported', value: metrics.imported },
    { label: 'Skipped', value: metrics.skipped },
    { label: 'Deleted', value: metrics.deleted },
    { label: 'Failed', value: metrics.failed },
  ];
}

function sourceScanRunMetricMap(
  detail: DataSourceDetailResponse,
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  const current = detail.scan_summary.latest_job_id === job.id ? detail.scan_summary : null;
  return {
    imported: run?.imported ?? current?.imported ?? 0,
    skipped: run?.skipped ?? current?.skipped ?? 0,
    deleted: run?.deleted ?? current?.deleted ?? 0,
    failed: run?.failed ?? current?.failed ?? 0,
  };
}

function sourceScanRunStartedAt(
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  if (run?.started_at) {
    return run.started_at;
  }
  return job.state === 'queued' ? job.created_at : job.updated_at;
}

function sourceScanRunFinishedAt(
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  if (run?.finished_at) {
    return run.finished_at;
  }
  if (isActiveJobState(job.state)) {
    return '';
  }
  return job.updated_at;
}

function sourceScanRunDurationMS(
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  if (typeof run?.duration_ms === 'number') {
    return Math.max(0, run.duration_ms);
  }
  const finishedAt = sourceScanRunFinishedAt(job, run);
  if (!finishedAt) {
    return null;
  }
  return Math.max(0, Date.parse(finishedAt) - Date.parse(sourceScanRunStartedAt(job, run)));
}

function sourceScanRunDurationLabel(
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  const duration = sourceScanRunDurationMS(job, run);
  if (duration === null) {
    return isActiveJobState(job.state) ? 'running' : '-';
  }
  return formatDurationMS(duration);
}

function sourceScanRunHistoryLabel(
  job: ListJobsResponse['jobs'][number],
  run: SourceScanRunSummary | null,
) {
  const startedAt = sourceScanRunStartedAt(job, run);
  const finishedAt = sourceScanRunFinishedAt(job, run);
  const timestamp = finishedAt || startedAt || job.updated_at;
  return formatDateTime(timestamp);
}

function SourcePlanReviewTable({ summary }: { summary: SourcePlanSummary }) {
  const samples = summary.samples ?? [];
  const [filter, setFilter] = useState<SourcePlanSampleFilter>('all');
  const [page, setPage] = useState(0);
  const filteredSamples = useMemo(
    () => samples.filter((sample) => filter === 'all' || sample.outcome === filter),
    [filter, samples],
  );
  if (samples.length === 0) {
    return null;
  }
  const filters = sourcePlanSampleFilters(samples);
  const pageCount = Math.max(1, Math.ceil(filteredSamples.length / sourcePlanSamplePageSize));
  const currentPage = Math.min(page, pageCount - 1);
  const pageStart = currentPage * sourcePlanSamplePageSize;
  const pageSamples = filteredSamples.slice(pageStart, pageStart + sourcePlanSamplePageSize);
  const visibleStart = filteredSamples.length === 0 ? 0 : pageStart + 1;
  const visibleEnd = Math.min(pageStart + pageSamples.length, filteredSamples.length);
  const capturedText =
    samples.length >= summary.total_entries
      ? `${samples.length} captured`
      : `${samples.length} captured of ${summary.total_entries}`;
  return (
    <details className="sourcePlanReviewDetails">
      <summary>
        <strong>Review samples</strong>
        <span>
          {visibleStart}-{visibleEnd} of {filteredSamples.length} / {capturedText}
        </span>
      </summary>
      <div className="sourcePlanReviewContent">
        <div className="sourcePlanReviewToolbar">
          <div className="scanEntryFilters" aria-label="Import plan sample filters">
            {filters.map((item) => (
              <button
                className={
                  item.key === filter ? 'scanEntryFilter scanEntryFilterActive' : 'scanEntryFilter'
                }
                key={item.key}
                onClick={() => {
                  setFilter(item.key);
                  setPage(0);
                }}
                type="button"
              >
                {item.label}
                <span>{item.count}</span>
              </button>
            ))}
          </div>
          <div className="scanEntryPager">
            <button
              disabled={currentPage === 0}
              onClick={() => setPage((value) => Math.max(0, value - 1))}
              type="button"
            >
              Prev
            </button>
            <button
              disabled={currentPage + 1 >= pageCount}
              onClick={() => setPage((value) => Math.min(pageCount - 1, value + 1))}
              type="button"
            >
              Next
            </button>
          </div>
        </div>
        <div className="sourcePlanSampleTable">
          <div className="sourcePlanSampleHeader">
            <span>Outcome</span>
            <span>Path</span>
            <span>Size</span>
            <span>Reason</span>
          </div>
          {pageSamples.map((sample) => (
            <div className="sourcePlanSampleRow" key={`${sample.outcome}:${sample.path}`}>
              <span className={sourcePlanOutcomeClass(sample)}>{sourcePlanSampleLabel(sample)}</span>
              <strong title={sample.path}>{sample.path}</strong>
              <em>{sample.size_bytes ? formatBytes(sample.size_bytes) : '-'}</em>
              <small title={sample.message || sample.reason || ''}>
                {sample.message || sourcePlanSampleReason(sample) || '-'}
              </small>
            </div>
          ))}
          {pageSamples.length === 0 && <p className="muted">No samples match this filter</p>}
        </div>
      </div>
    </details>
  );
}

function sourceLatestRelevantPlanJob(detail: DataSourceDetailResponse) {
  return detail.jobs
    .filter(
      (job) =>
        job.type === 'source_plan' &&
        job.resource_type === 'data_source' &&
        job.resource_id === detail.source.id &&
        sourceJobAppliesToSource(detail.source, job),
    )
    .sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at))[0];
}

function sourceHasActivePlanJob(detail: DataSourceDetailResponse) {
  const job = sourceLatestRelevantPlanJob(detail);
  return Boolean(job && isActiveJobState(job.state));
}

function sourceScanBlockedByPlan(detail: DataSourceDetailResponse) {
  return sourcePlanBlocksScan(detail.source, sourceLatestRelevantPlanJob(detail));
}

function sourcePlanBlocksScan(
  source: { updated_at: string },
  job?: ListJobsResponse['jobs'][number],
) {
  return Boolean(job && job.state === 'failed' && sourceJobAppliesToSource(source, job));
}

function sourcePlanSummaryFromJob(job: ListJobsResponse['jobs'][number]) {
  if (!job.result_json) {
    return null;
  }
  try {
    return JSON.parse(job.result_json) as SourcePlanSummary;
  } catch {
    return null;
  }
}

function sourcePlanTitle(job: ListJobsResponse['jobs'][number]) {
  if (isActiveJobState(job.state)) {
    return 'Planning import';
  }
  if (job.state === 'succeeded') {
    return 'Import plan ready';
  }
  if (job.state === 'failed') {
    return 'Plan failed';
  }
  return titleCase(job.state);
}

function sourcePlanMessage(
  job: ListJobsResponse['jobs'][number],
  summary: SourcePlanSummary | null,
) {
  if (job.state === 'failed' && job.error_message.trim() !== '') {
    return job.error_message;
  }
  if (isActiveJobState(job.state)) {
    return 'Worker is previewing matched files.';
  }
  if (summary) {
    return `${summary.files_seen} files seen / ${summary.total_entries} entries checked`;
  }
  return jobDetail(job);
}

function sourcePlanSampleLabel(sample: SourcePlanSample) {
  if (sample.outcome === 'would_import') {
    return 'Import';
  }
  if (sample.reason) {
    return titleCase(sample.reason.replace(/_/g, ' '));
  }
  return titleCase(sample.outcome);
}

function sourcePlanSampleReason(sample: SourcePlanSample) {
  if (!sample.reason) {
    return '';
  }
  return titleCase(sample.reason.replace(/_/g, ' '));
}

function sourcePlanOutcomeClass(sample: SourcePlanSample) {
  if (sample.outcome === 'failed') {
    return 'stateBadge stateFailed';
  }
  if (sample.outcome === 'would_import') {
    return 'stateBadge stateReady';
  }
  return 'stateBadge';
}

function sourcePlanSampleFilters(samples: SourcePlanSample[]) {
  const sampledSkipped = samples.filter((sample) => sample.outcome === 'skipped').length;
  const sampledFailed = samples.filter((sample) => sample.outcome === 'failed').length;
  const sampledImport = samples.filter((sample) => sample.outcome === 'would_import').length;
  return [
    { key: 'all' as const, label: 'All', count: samples.length },
    { key: 'would_import' as const, label: 'Import', count: sampledImport },
    { key: 'skipped' as const, label: 'Skipped', count: sampledSkipped },
    { key: 'failed' as const, label: 'Failed', count: sampledFailed },
  ];
}

function sourceFirstScanReviewPrompt(
  source: ListDataSourcesResponse['sources'][number],
  job?: ListJobsResponse['jobs'][number],
) {
  if (sourceImportHasStarted(source)) {
    return '';
  }
  if (!job || !sourceJobAppliesToSource(source, job) || job.state !== 'succeeded') {
    return 'This source has not been previewed with Plan yet. Continue the first scan anyway?';
  }
  const summary = sourcePlanSummaryFromJob(job);
  if (!summary) {
    return 'The latest import plan has no readable summary. Continue the first scan anyway?';
  }
  const reasons = sourcePlanReviewReasons(summary);
  if (reasons.length === 0) {
    return '';
  }
  return `Review this import plan before the first scan?\n\n${reasons.join('\n')}\n\nContinue scan?`;
}

function sourcePlanReviewReasons(summary: SourcePlanSummary) {
  const reasons: string[] = [];
  if (summary.failed > 0) {
    reasons.push(`${summary.failed} files or folders had planning errors`);
  }
  if (summary.would_import >= sourcePlanLargeImportFileThreshold) {
    reasons.push(`${summary.would_import} files would import`);
  }
  if (summary.estimated_bytes >= sourcePlanLargeImportBytesThreshold) {
    reasons.push(`${formatBytes(summary.estimated_bytes)} estimated ingest size`);
  }
  const considered = summary.would_import + summary.skipped + summary.failed;
  const skippedRatio = considered > 0 ? summary.skipped / considered : 0;
  if (
    summary.skipped >= sourcePlanSkippedFileThreshold ||
    (summary.skipped > 0 && skippedRatio >= sourcePlanSkippedRatioThreshold)
  ) {
    reasons.push(`${summary.skipped} files or folders would be skipped`);
  }
  return reasons;
}

function sourceImportHasStarted(source: {
  last_scan_at?: string;
  last_scan_imported: number;
  last_scan_skipped: number;
  last_scan_failed: number;
}) {
  return Boolean(
    source.last_scan_at ||
      (source.last_scan_imported ?? 0) > 0 ||
      (source.last_scan_skipped ?? 0) > 0 ||
      (source.last_scan_failed ?? 0) > 0,
  );
}

function SourcePreflightStatus({
  activeJob,
  checking,
  checkDisabled,
  detail,
  onCheck,
  onRefresh,
  refreshing,
}: {
  activeJob?: ListJobsResponse['jobs'][number];
  checking: boolean;
  checkDisabled: boolean;
  detail: DataSourceDetailResponse;
  onCheck: () => void;
  onRefresh: () => void;
  refreshing: boolean;
}) {
  const job = activeJob ?? sourceLatestRelevantPreflightJob(detail);
  if (!job) {
    return null;
  }
  const active = isActiveJobState(job.state);
  const failed = job.state === 'failed';
  return (
    <section
      aria-label="Source path check"
      className={
        failed
          ? 'sourceProgressPanel sourcePreflightPanel sourcePreflightFailed'
          : 'sourceProgressPanel sourcePreflightPanel'
      }
    >
      {active ? (
        <span className="spinner" aria-hidden="true" />
      ) : (
        <span className={stateClass(job.state)}>{titleCase(job.state)}</span>
      )}
      <div className="sourceProgressText">
        <strong>{sourcePreflightTitle(job)}</strong>
        <span>{sourcePreflightMessage(job)}</span>
      </div>
      <div className="sourcePreflightActions">
        <button disabled={refreshing} onClick={onRefresh} type="button">
          {refreshing ? 'Refreshing' : 'Refresh'}
        </button>
        <button disabled={active || checking || checkDisabled} onClick={onCheck} type="button">
          {checking ? 'Queuing' : 'Check again'}
        </button>
      </div>
    </section>
  );
}

function sourceLatestRelevantPreflightJob(detail: DataSourceDetailResponse) {
	return detail.jobs
		.filter(
			(job) =>
				job.type === 'source_preflight' &&
				job.resource_type === 'data_source' &&
				job.resource_id === detail.source.id &&
				sourceJobAppliesToSource(detail.source, job),
		)
		.sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at))[0];
}

function sourceHasActivePreflightJob(detail: DataSourceDetailResponse) {
  const job = sourceLatestRelevantPreflightJob(detail);
  return Boolean(job && isActiveJobState(job.state));
}

function sourceScanBlockedByPreflight(detail: DataSourceDetailResponse) {
  return sourcePreflightBlocksScan(detail.source, sourceLatestRelevantPreflightJob(detail));
}

function sourcePreflightBlocksScan(
  source: { updated_at: string },
  job?: ListJobsResponse['jobs'][number],
) {
  return Boolean(job && job.state === 'failed' && sourceJobAppliesToSource(source, job));
}

function sourceJobAppliesToSource(
  source: { updated_at: string },
  job: ListJobsResponse['jobs'][number],
) {
  const sourceUpdatedAt = Date.parse(source.updated_at);
  const jobUpdatedAt = Date.parse(job.updated_at);
  if (Number.isNaN(sourceUpdatedAt) || Number.isNaN(jobUpdatedAt)) {
    return true;
  }
  return jobUpdatedAt >= sourceUpdatedAt;
}

function sourcePreflightTitle(job: ListJobsResponse['jobs'][number]) {
  if (isActiveJobState(job.state)) {
    return 'Checking path';
  }
  if (job.state === 'succeeded') {
    return 'Path reachable';
  }
  if (job.state === 'failed') {
    return 'Path blocked';
  }
  return titleCase(job.state);
}

function sourcePreflightMessage(job: ListJobsResponse['jobs'][number]) {
  if (job.state === 'failed' && job.error_message.trim() !== '') {
    return job.error_message;
  }
  if (isActiveJobState(job.state)) {
    return 'Worker is checking the mounted folder.';
  }
  if (job.state === 'succeeded') {
    return 'Worker can open and read the configured folder.';
  }
  if (job.state === 'canceled') {
    return 'The source was archived before the check ran.';
  }
  return jobDetail(job);
}

function sourceHasActiveScanJob(detail: DataSourceDetailResponse) {
  return detail.jobs.some(
    (job) =>
      job.type === 'source_scan' &&
      job.resource_type === 'data_source' &&
      job.resource_id === detail.source.id &&
      isActiveJobState(job.state),
  );
}

function isActiveJobState(state: string) {
  return ['queued', 'running', 'retrying'].includes(state);
}

function isActiveDocumentStatus(status: string) {
  return ['uploaded', 'processing'].includes(status);
}

function documentStatusHint(
  document: ListDocumentsResponse['documents'][number],
  active: boolean,
) {
  if (active) {
    return 'Ingestion running';
  }
  switch (document.status) {
    case 'ready':
      return `Ready ${formatDateTime(document.updated_at)}`;
    case 'failed':
      return `Failed ${formatDateTime(document.updated_at)}`;
    case 'uploaded':
      return 'Queued for ingestion';
    case 'processing':
      return 'Parsing and indexing';
    default:
      return titleCase(document.status);
  }
}

function documentStatusTitle(detail: DocumentDetailResponse) {
  if (hasActiveIngestionJob(detail)) {
    return 'Ingestion running';
  }
  switch (detail.document.status) {
    case 'ready':
      return 'Ready for ask and search';
    case 'failed':
      return 'Needs attention';
    case 'uploaded':
      return 'Queued for ingestion';
    case 'processing':
      return 'Processing';
    default:
      return titleCase(detail.document.status);
  }
}

function documentStatusDetail(detail: DocumentDetailResponse) {
  const latestJob = detail.jobs[0];
  if (detail.document.status === 'failed') {
    return documentFailureMessage(detail) || 'Retry after fixing the source or parser issue.';
  }
  if (hasActiveIngestionJob(detail)) {
    return latestJob
      ? `${jobTypeLabel(latestJob)} ${titleCase(latestJob.state)}`
      : 'Worker is active';
  }
  if (detail.document.status === 'ready') {
    return `Indexed ${formatDateTime(detail.document.updated_at)}`;
  }
  if (latestJob) {
    return jobDetail(latestJob);
  }
  return `Updated ${formatDateTime(detail.document.updated_at)}`;
}

function documentFailureMessage(detail: DocumentDetailResponse) {
  return (
    detail.jobs.find((job) => job.state === 'failed' && job.error_message.trim() !== '')
      ?.error_message ?? ''
  );
}

function sourceFailureMessage(detail: DataSourceDetailResponse) {
  return (
    detail.jobs.find((job) => job.state === 'failed' && job.error_message.trim() !== '')
      ?.error_message ?? ''
  );
}

function scanOutcomeClass(entry: DataSourceDetailResponse['scan_entries'][number]) {
  switch (entry.outcome) {
    case 'imported':
      return 'stateBadge stateReady';
    case 'failed':
      return 'stateBadge stateFailed';
    case 'deleted':
      return 'stateBadge stateReady';
    case 'skipped':
      if (entry.reason === 'unchanged') {
        return 'stateBadge stateReady';
      }
      return 'stateBadge stateActive';
    default:
      return 'stateBadge';
  }
}

function scanEntryReason(entry: DataSourceDetailResponse['scan_entries'][number]) {
  if (entry.reason.trim() === '') {
    return entry.document_id ? entry.document_id : 'ok';
  }
  return titleCase(entry.reason.replace(/_/g, ' '));
}

function scanEntryMessage(entry: DataSourceDetailResponse['scan_entries'][number]) {
  return entry.message || entry.reason || entry.document_id || entry.path;
}

function jobDetail(job: ListJobsResponse['jobs'][number]) {
  if (job.type === 'source_scan') {
    const run = sourceScanRunSummaryFromJob(job);
    if (run) {
      return `${run.imported} imported / ${run.skipped} skipped / ${run.failed} failed`;
    }
  }
  if (job.type === 'source_plan') {
    const summary = sourcePlanSummaryFromJob(job);
    if (summary) {
      return `${summary.would_import} would import / ${summary.skipped} skipped / ${formatBytes(
        summary.estimated_bytes,
      )}`;
    }
  }
  return job.error_message || job.resource_id || job.id;
}

function jobTitle(job: ListJobsResponse['jobs'][number]) {
  if (job.error_message) {
    return `${job.id}: ${job.error_message}`;
  }
  return job.id;
}

function auditResourceLabel(event: ListAuditEventsResponse['events'][number]) {
  if (event.resource_type && event.resource_id) {
    return `${event.resource_type}/${event.resource_id}`;
  }
  return event.resource_type || event.resource_id || event.id;
}

function auditApiFilters(filters: typeof initialAuditFilters): AuditEventFilters {
  return {
    action: filters.action || undefined,
    actor_user_id: filters.actor_user_id.trim() || undefined,
    from: filters.from || undefined,
    limit: 100,
    outcome: filters.outcome || undefined,
    query: filters.query.trim() || undefined,
    to: filters.to || undefined,
  };
}

function filterAuditEvents(
  events: ListAuditEventsResponse['events'],
  filters: typeof initialAuditFilters,
) {
  const query = filters.query.trim().toLowerCase();
  const actorUserID = filters.actor_user_id.trim();
  return events.filter((event) => {
    if (filters.action && event.action !== filters.action) {
      return false;
    }
    if (filters.outcome && event.outcome !== filters.outcome) {
      return false;
    }
    if (actorUserID && event.actor_user_id !== actorUserID) {
      return false;
    }
    const eventDate = dateInputValue(event.created_at);
    if (filters.from && eventDate && eventDate < filters.from) {
      return false;
    }
    if (filters.to && eventDate && eventDate > filters.to) {
      return false;
    }
    if (!query) {
      return true;
    }
    const metadata = Object.entries(event.metadata)
      .map(([key, value]) => `${key} ${value}`)
      .join(' ');
    const haystack = [
      event.id,
      event.actor_user_id,
      event.action,
      event.outcome,
      auditResourceLabel(event),
      metadata,
    ]
      .join(' ')
      .toLowerCase();
    return haystack.includes(query);
  });
}

function dateInputValue(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return date.toISOString().slice(0, 10);
}

function auditMetadataLabel(metadata: Record<string, string>) {
  const entries = Object.entries(metadata).filter(([, value]) => value.trim() !== '');
  if (entries.length === 0) {
    return 'no metadata';
  }
  return entries
    .slice(0, 4)
    .map(([key, value]) => `${key}=${value}`)
    .join(' / ');
}

function buildModelPerformanceRows(
  targets: ModelTarget[],
  targetChecks: Record<string, TargetCheckState>,
  events: ListAuditEventsResponse['events'],
): ModelPerformanceRow[] {
  const targetMap = new Map(targets.map((target) => [target.name, target]));
  const grouped = new Map<string, ListAuditEventsResponse['events']>();
  for (const event of events.filter((item) => item.action === 'conversation.ask')) {
    const targetName = event.metadata.model_target || 'general';
    if (!targetMap.has(targetName)) {
      targetMap.set(targetName, {
        name: targetName,
        provider: 'unknown',
        base_url: '',
        model: event.metadata.model || targetName,
      });
    }
    grouped.set(targetName, [...(grouped.get(targetName) ?? []), event]);
  }

  return Array.from(targetMap.values())
    .sort((left, right) => left.name.localeCompare(right.name))
    .map((target) => {
      const targetEvents = (grouped.get(target.name) ?? []).sort(
        (left, right) => Date.parse(right.created_at) - Date.parse(left.created_at),
      );
      const latencies = targetEvents
        .map((event) => Number(event.metadata.latency_ms))
        .filter((value) => Number.isFinite(value) && value >= 0)
        .sort((left, right) => left - right);
      const askCount = targetEvents.length;
      const failureEvents = targetEvents.filter((event) => event.outcome === 'failed');
      const lastErrorEvent = failureEvents[0];
      const successCount = askCount - failureEvents.length;
      return {
        target,
        check: targetChecks[target.name],
        askCount,
        failureCount: failureEvents.length,
        latestLatencyMS: firstFiniteLatency(targetEvents),
        p95LatencyMS: percentileValue(latencies, 95),
        successRate: askCount === 0 ? null : Math.round((successCount / askCount) * 100),
        lastError: lastErrorEvent?.metadata.error ?? '',
        lastErrorClass: lastErrorEvent?.metadata.error_class ?? '',
        lastEventAt: targetEvents[0]?.created_at ?? '',
      };
    });
}

function summarizeModelPerformance(rows: ModelPerformanceRow[]) {
  const latencies = rows
    .flatMap((row) => (row.p95LatencyMS === null ? [] : [row.p95LatencyMS]))
    .sort((left, right) => left - right);
  const askCount = rows.reduce((total, row) => total + row.askCount, 0);
  const failureCount = rows.reduce((total, row) => total + row.failureCount, 0);
  const successCount = askCount - failureCount;
  const checkedCount = rows.filter((row) => row.check).length;
  const unhealthyCount = rows.filter(
    (row) => row.failureCount > 0 || row.check?.state === 'failed',
  ).length;
  return {
    targetCount: rows.length,
    checkedCount,
    askCount,
    failureCount,
    unhealthyCount,
    p95LatencyMS: percentileValue(latencies, 95),
    successRate: askCount === 0 ? null : Math.round((successCount / askCount) * 100),
  };
}

function firstFiniteLatency(events: ListAuditEventsResponse['events']) {
  for (const event of events) {
    const latency = Number(event.metadata.latency_ms);
    if (Number.isFinite(latency) && latency >= 0) {
      return latency;
    }
  }
  return null;
}

function percentileValue(values: number[], percentile: number) {
  if (values.length === 0) {
    return null;
  }
  const safePercentile = Math.min(100, Math.max(1, percentile));
  const index = Math.ceil((safePercentile / 100) * values.length) - 1;
  return values[Math.max(0, Math.min(values.length - 1, index))];
}

function uniqueSorted(values: string[]) {
  return Array.from(new Set(values.filter((value) => value.trim() !== ''))).sort((a, b) =>
    a.localeCompare(b),
  );
}

function stateClass(state: string) {
  if (['failed', 'denied'].includes(state)) {
    return 'stateBadge stateFailed';
  }
  if (['queued', 'running', 'retrying', 'uploaded', 'processing', 'scanning'].includes(state)) {
    return 'stateBadge stateActive';
  }
  if (['active', 'ready', 'succeeded'].includes(state)) {
    return 'stateBadge stateReady';
  }
  return 'stateBadge';
}

function mergeRegistrationProgress(
  current: RegisterDocumentResponse,
  documents: ListDocumentsResponse,
  jobs: ListJobsResponse,
) {
  return {
    document:
      documents.documents.find((document) => document.id === current.document.id) ??
      current.document,
    job: jobs.jobs.find((job) => job.id === current.job.id) ?? current.job,
  };
}

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = globalThis.document.createElement('a');
  link.href = url;
  link.download = filename;
  globalThis.document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function formatBytes(bytes: number) {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(1)} KB`;
  }
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(new Date(value));
}

function formatElapsed(seconds: number) {
  if (seconds < 60) {
    return `${seconds}s`;
  }
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  return `${minutes}m ${remainder.toString().padStart(2, '0')}s`;
}

function formatDurationMS(ms: number) {
  return formatElapsed(Math.max(0, Math.round(ms / 1000)));
}

function formatLatencyMS(ms: number) {
  if (ms < 1000) {
    return `${Math.max(0, Math.round(ms))}ms`;
  }
  if (ms < 60000) {
    return `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)}s`;
  }
  return formatDurationMS(ms);
}

function waitForMs(milliseconds: number) {
  return new Promise<void>((resolve) => {
    window.setTimeout(resolve, milliseconds);
  });
}

function formatRelativeDateTime(value: string) {
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return 'unknown';
  }
  const deltaMS = timestamp - Date.now();
  const absSeconds = Math.max(0, Math.round(Math.abs(deltaMS) / 1000));
  if (absSeconds < 45) {
    return 'now';
  }
  let amount = Math.round(absSeconds / 60);
  let unit = 'min';
  if (amount >= 60) {
    amount = Math.round(amount / 60);
    unit = amount === 1 ? 'hour' : 'hours';
  }
  if (unit !== 'min' && amount >= 24) {
    amount = Math.round(amount / 24);
    unit = amount === 1 ? 'day' : 'days';
  }
  const label = unit === 'min' ? `${amount} min` : `${amount} ${unit}`;
  return deltaMS < 0 ? `${label} ago` : `in ${label}`;
}

function formatTimeOnly(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value));
}
