import { FormEvent, useEffect, useMemo, useState } from 'react';
import {
  AskConversationResponse,
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
  SearchDocumentsResponse,
  addTenantMember,
  archiveDataSource,
  askConversationStream,
  apiBase,
  checkModelTarget,
  createDataSource,
  createTenant,
  dataSourceScanEntriesExportUrl,
  deleteConversation,
  deleteDocument,
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
  listTenantMembers,
  reindexDataSource,
  retryDocument,
  scanDataSource,
  searchDocuments,
  updateDataSource,
  uploadDocument,
} from './api';

type View = 'ask' | 'documents' | 'search' | 'activity' | 'history' | 'status' | 'settings';

type AskPhase = 'idle' | 'connecting' | 'retrieving' | 'generating' | 'streaming' | 'complete' | 'failed';

type ScanEntryFilter = 'all' | 'imported' | 'skipped' | 'failed' | 'deleted';

type TargetCheckState = {
  state: 'ok' | 'failed';
  detail: string;
};

type SetupCheckStatus = 'checking' | 'ok' | 'warning' | 'blocked';

type SetupCheck = {
  id: string;
  label: string;
  detail: string;
  status: SetupCheckStatus;
  blocking: boolean;
};

const setupWizardStorageKey = 'nexus-local.setupWizardAcknowledged';
const scanEntryPageSize = 100;

const initialAsk = {
  conversation_id: '',
  document_id: '',
  model_target: 'general',
  question: '',
  limit: 5,
};

const initialMemberForm = {
  user_id: '',
  email: '',
  name: '',
  role: 'member',
};

const initialAuditFilters = {
  action: '',
  outcome: '',
  query: '',
};

const initialSourceForm = {
  type: 'synced_folder',
  name: '',
  root_path: '',
  include_patterns: '',
  exclude_patterns: '',
  scan_interval_minutes: '0',
};

const supportedDocumentAccept = [
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
].join(',');

const sampleDocumentContent = `# Nexus Local Sample

Nexus Local can ingest documents, index their text, search retrieved chunks, and answer questions with cited sources.

Sample phrase: cedar signal atlas.

Try searching for cedar signal atlas, then ask what phrase appears in the sample document.
`;

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
  const [setupAcknowledgedKey, setSetupAcknowledgedKey] = useState(
    () => window.localStorage.getItem(setupWizardStorageKey) ?? '',
  );
  const [tenantID, setTenantID] = useState('');
  const [tenantName, setTenantName] = useState('Personal Workspace');
  const [file, setFile] = useState<File | null>(null);
  const [registration, setRegistration] = useState<RegisterDocumentResponse | null>(null);
  const [documents, setDocuments] = useState<ListDocumentsResponse | null>(null);
  const [dataSources, setDataSources] = useState<ListDataSourcesResponse | null>(null);
  const [sourceDetail, setSourceDetail] = useState<DataSourceDetailResponse | null>(null);
  const [scanEntryFilter, setScanEntryFilter] = useState<ScanEntryFilter>('all');
  const [scanEntryOffset, setScanEntryOffset] = useState(0);
  const [documentDetail, setDocumentDetail] = useState<DocumentDetailResponse | null>(null);
  const [jobs, setJobs] = useState<ListJobsResponse | null>(null);
  const [auditEvents, setAuditEvents] = useState<ListAuditEventsResponse | null>(null);
  const [auditFilters, setAuditFilters] = useState(initialAuditFilters);
  const [tenantMembers, setTenantMembers] = useState<ListTenantMembersResponse | null>(null);
  const [conversations, setConversations] = useState<ListConversationsResponse | null>(null);
  const [conversationMessages, setConversationMessages] =
    useState<ListConversationMessagesResponse | null>(null);
  const [selectedConversationID, setSelectedConversationID] = useState('');
  const [searchForm, setSearchForm] = useState({ document_id: '', query: '', limit: 5 });
  const [sourceForm, setSourceForm] = useState(initialSourceForm);
  const [sourceEditForm, setSourceEditForm] = useState(initialSourceForm);
  const [searchResult, setSearchResult] = useState<SearchDocumentsResponse | null>(null);
  const [memberForm, setMemberForm] = useState(initialMemberForm);
  const [askForm, setAskForm] = useState(initialAsk);
  const [askResult, setAskResult] = useState<AskConversationResponse | null>(null);
  const [streamAnswer, setStreamAnswer] = useState('');
  const [streamStatus, setStreamStatus] = useState('');
  const [askPhase, setAskPhase] = useState<AskPhase>('idle');
  const [askStartedAt, setAskStartedAt] = useState<number | null>(null);
  const [askElapsedSeconds, setAskElapsedSeconds] = useState(0);
  const [currentAskQuestion, setCurrentAskQuestion] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [creatingSource, setCreatingSource] = useState(false);
  const [archivingSourceID, setArchivingSourceID] = useState('');
  const [deletingSourceDocumentsID, setDeletingSourceDocumentsID] = useState('');
  const [scanningSourceID, setScanningSourceID] = useState('');
  const [reindexingSourceID, setReindexingSourceID] = useState('');
  const [uploadingSample, setUploadingSample] = useState(false);
  const [creatingTenant, setCreatingTenant] = useState(false);
  const [savingMember, setSavingMember] = useState(false);
  const [removingMemberID, setRemovingMemberID] = useState('');
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

  const selectedTenant = useMemo(
    () => currentUser?.memberships.find((membership) => membership.tenant.id === tenantID),
    [currentUser, tenantID],
  );
  const workspaceReady = tenantID !== '';
  const needsWorkspace = currentUser !== null && currentUser.memberships.length === 0;
  const workspaceLabel = selectedTenant?.tenant.name ?? (tenantID || 'No workspace');
  const canManageTenant = selectedTenant?.role === 'owner' || selectedTenant?.role === 'admin';
  const canManageMembers = canManageTenant;
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
  const readyDocumentCount =
    documents?.documents.filter((document) => document.status === 'ready').length ?? 0;
  const processingDocumentCount = activeDocuments.length;
  const activeJobCount = activeJobs.length;
  const activeSourceScanCount = activeSourceScanJobs.size;
  const trackedSourceID = sourceDetail?.source.id ?? '';
  const trackedDocumentID = documentDetail?.document.id ?? registration?.document.id ?? '';
  const trackingIngestion = useMemo(
    () =>
      activeIngestionJobs.length > 0 ||
      activeSourceScanCount > 0 ||
      Boolean(documentDetail && isActiveDocumentStatus(documentDetail.document.status)) ||
      Boolean(documentDetail && hasActiveIngestionJob(documentDetail)) ||
      Boolean(
        registration &&
          (isActiveDocumentStatus(registration.document.status) ||
            isActiveJobState(registration.job.state)),
      ),
    [activeIngestionJobs.length, activeSourceScanCount, documentDetail, registration],
  );
  const ingestionLabel = ingestionSyncing
    ? 'Updating'
    : trackingIngestion
      ? activeIngestionJobs.length > 0
        ? `${activeIngestionJobs.length} active`
        : activeSourceScanCount > 0
          ? `${activeSourceScanCount} scan`
        : 'Tracking'
      : lastIngestionSync
        ? `Synced ${formatTimeOnly(lastIngestionSync)}`
        : 'Idle';
  const visibleAskAnswer = askResult?.assistant_message.content || streamAnswer;
  const visibleAskTitle =
    askResult?.conversation.title ||
    askResult?.conversation.id ||
    currentAskQuestion ||
    streamStatus ||
    'Working';
  const visibleAskModel = askResult?.completion.model || askForm.model_target;
  const showAskProgress = asking || askPhase === 'failed';
  const showAskResult = Boolean(askResult || visibleAskAnswer || showAskProgress || streamStatus);
  const auditActionOptions = useMemo(
    () => uniqueSorted(auditEvents?.events.map((event) => event.action) ?? []),
    [auditEvents],
  );
  const auditOutcomeOptions = useMemo(
    () => uniqueSorted(auditEvents?.events.map((event) => event.outcome) ?? []),
    [auditEvents],
  );
  const filteredAuditEvents = useMemo(
    () => filterAuditEvents(auditEvents?.events ?? [], auditFilters),
    [auditEvents, auditFilters],
  );
  const auditFiltersActive = Boolean(
    auditFilters.action || auditFilters.outcome || auditFilters.query.trim(),
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
      return;
    }

    void refreshAuditEvents(tenantID);
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
      setJobs({ jobs: [] });
      setAuditEvents(null);
      setConversations({ conversations: [] });
      setActiveView('ask');
      return { targets: targetsResult.targets };
    }

    const [documentsResult, dataSourcesResult, jobsResult, conversationsResult] = await Promise.all([
      listDocuments(initialTenantID),
      listDataSources(initialTenantID),
      listJobs(initialTenantID),
      listConversations(initialTenantID),
    ]);
    setDocuments(documentsResult);
    setDataSources(dataSourcesResult);
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
      setSetupGatewayCheck({
        state: 'failed',
        detail: messageFromError(err),
      });
    } finally {
      setSetupChecking(false);
      setSetupLastChecked(new Date().toISOString());
    }
  }

  async function runSetupChecks() {
    setError(null);
    setSetupChecking(true);
    try {
      const result = await loadBootstrapData();
      await checkSetupGateway(result.targets);
    } catch (err) {
      const message = messageFromError(err);
      setError(message);
      setSetupGatewayCheck({ state: 'failed', detail: message });
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

  async function refreshAuditEvents(nextTenantID = tenantID) {
    setError(null);
    if (!nextTenantID) {
      setAuditEvents(null);
      return;
    }
    setLoadingAudit(true);
    try {
      setAuditEvents(await listAuditEvents(nextTenantID));
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setLoadingAudit(false);
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
      setJobs({ jobs: [] });
      setConversations({ conversations: [] });
      return;
    }
    try {
      const [documentsResult, dataSourcesResult, jobsResult, conversationsResult] = await Promise.all([
        listDocuments(nextTenantID),
        listDataSources(nextTenantID),
        listJobs(nextTenantID),
        listConversations(nextTenantID),
      ]);
      setDocuments(documentsResult);
      setDataSources(dataSourcesResult);
      setJobs(jobsResult);
      setConversations(conversationsResult);
    } catch (err) {
      setError(messageFromError(err));
    }
  }

  async function refreshRuntime() {
    setError(null);
    try {
      const [healthResult, readinessResult, targetsResult, jobsResult, auditResult] =
        await Promise.all([
          getHealth(),
          getReadiness(),
          getModelTargets(),
          tenantID ? listJobs(tenantID) : Promise.resolve({ jobs: [] }),
          tenantID && canManageTenant
            ? listAuditEvents(tenantID).catch(() => null)
            : Promise.resolve(null),
        ]);
      setHealth(healthResult);
      setReadiness(readinessResult);
      setTargets(targetsResult.targets);
      setJobs(jobsResult);
      setAuditEvents(auditResult);
    } catch (err) {
      setError(messageFromError(err));
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
        },
      }));
    } catch (err) {
      setTargetChecks((current) => ({
        ...current,
        [targetName]: {
          state: 'failed',
          detail: messageFromError(err),
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
    setSearchResult(null);
    setTenantMembers(null);
    setAuditEvents(null);
    setSelectedConversationID('');
    setConversationMessages(null);
    setAskForm((current) => ({ ...current, conversation_id: '', document_id: '' }));
    setSearchForm((current) => ({ ...current, document_id: '' }));
    setError(null);
    if (!nextTenantID) {
      setDocuments({ documents: [] });
      setDataSources({ sources: [] });
      setSourceDetail(null);
      setJobs({ jobs: [] });
      setTenantMembers(null);
      setAuditEvents(null);
      setConversations({ conversations: [] });
      return;
    }
    try {
      const [documentsResult, dataSourcesResult, jobsResult, conversationsResult, auditResult] = await Promise.all([
        listDocuments(nextTenantID),
        listDataSources(nextTenantID),
        listJobs(nextTenantID),
        listConversations(nextTenantID),
        listAuditEvents(nextTenantID).catch(() => null),
      ]);
      setDocuments(documentsResult);
      setDataSources(dataSourcesResult);
      setJobs(jobsResult);
      setConversations(conversationsResult);
      setAuditEvents(auditResult);
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
    if (!file) {
      setError('Choose a file');
      return;
    }
    await uploadWorkspaceFile(file);
  }

  async function uploadSampleDocument() {
    const sampleFile = new File([sampleDocumentContent], 'nexus-local-sample.md', {
      type: 'text/markdown',
    });
    setUploadingSample(true);
    try {
      await uploadWorkspaceFile(sampleFile);
    } finally {
      setUploadingSample(false);
    }
  }

  async function uploadWorkspaceFile(nextFile: File) {
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const result = await uploadDocument({ tenant_id: tenantID, file: nextFile });
      setRegistration(result);
      setDocumentDetail({ document: result.document, jobs: [result.job] });
      await Promise.all([
        refreshDocuments(tenantID),
        refreshJobs(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setSubmitting(false);
    }
  }

  async function submitDataSource(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    if (!sourceForm.name.trim() || !sourceForm.root_path.trim()) {
      setError('Enter a source name and path');
      return;
    }
    setCreatingSource(true);
    setError(null);
    try {
      await createDataSource({
        tenant_id: tenantID,
        type: sourceForm.type,
        name: sourceForm.name.trim(),
        root_path: sourceForm.root_path.trim(),
        include_patterns: patternLinesToList(sourceForm.include_patterns),
        exclude_patterns: patternLinesToList(sourceForm.exclude_patterns),
        scan_interval_minutes: Number(sourceForm.scan_interval_minutes),
      });
      setSourceForm(initialSourceForm);
      await Promise.all([
        refreshDataSources(tenantID),
        canManageTenant ? refreshAuditEvents(tenantID) : Promise.resolve(),
      ]);
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setCreatingSource(false);
    }
  }

  async function submitSourceUpdate(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
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

  async function requestDataSourceScan(source: ListDataSourcesResponse['sources'][number]) {
    if (!tenantID) {
      setError('Create a workspace first');
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
          };
        }
        return {
          source: result.source,
          jobs: [result.job, ...current.jobs.filter((job) => job.id !== result.job.id)],
          scan_entries: current.scan_entries,
          scan_summary: current.scan_summary,
          scan_entries_page: current.scan_entries_page,
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

  async function requestDataSourceReindex(source: ListDataSourcesResponse['sources'][number]) {
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
        canManageTenant ? listAuditEvents(tenantID).catch(() => null) : Promise.resolve(null),
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

  function exportSourceScanEntries() {
    if (!tenantID || !sourceDetail) {
      return;
    }
    window.location.href = dataSourceScanEntriesExportUrl(tenantID, sourceDetail.source.id, {
      outcome: scanEntryFilter === 'all' ? undefined : scanEntryFilter,
    });
  }

  async function submitTenant(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!tenantName.trim()) {
      setError('Enter a workspace name');
      return;
    }
    setCreatingTenant(true);
    setError(null);
    try {
      const hadNoWorkspace = needsWorkspace;
      const created = await createTenant({ name: tenantName });
      setCurrentUser(await getCurrentUser());
      await switchTenant(created.tenant.id);
      if (hadNoWorkspace) {
        setActiveView('ask');
      }
    } catch (err) {
      setError(messageFromError(err));
    } finally {
      setCreatingTenant(false);
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
      setSearchResult(
        await searchDocuments({
          tenant_id: tenantID,
          document_id: searchForm.document_id || undefined,
          query: searchForm.query,
          limit: Number(searchForm.limit),
        }),
      );
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
    if (!tenantID) {
      setError('Create a workspace first');
      return;
    }
    const question = askForm.question.trim();
    if (!question) {
      setError('Enter a question');
      return;
    }
    setAsking(true);
    setError(null);
    setAskResult(null);
    setStreamAnswer('');
    setCurrentAskQuestion(question);
    setAskPhase('connecting');
    setAskStartedAt(Date.now());
    setAskElapsedSeconds(0);
    setStreamStatus('Starting');
    let failed = false;
    try {
      let streamedResult: AskConversationResponse | undefined;
      await askConversationStream({
        tenant_id: tenantID,
        conversation_id: askForm.conversation_id || undefined,
        document_id: askForm.document_id || undefined,
        model_target: askForm.model_target,
        question,
        limit: Number(askForm.limit),
      }, {
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
          setStreamAnswer(response.assistant_message.content);
          setAskPhase('complete');
        },
      });
      if (!streamedResult) {
        return;
      }
      const result = streamedResult;
      setAskResult(result);
      setAskForm((current) => ({
        ...current,
        conversation_id: result.conversation.id,
        question: '',
      }));
      setSelectedConversationID(result.conversation.id);
      await refreshConversations(tenantID);
      if (canManageTenant) {
        await refreshAuditEvents(tenantID);
      }
      if (conversationMessages && selectedConversationID === result.conversation.id) {
        setConversationMessages(await listConversationMessages(tenantID, result.conversation.id));
      }
    } catch (err) {
      failed = true;
      const message = messageFromError(err);
      setAskPhase('failed');
      setStreamStatus(message);
      setError(message);
    } finally {
      setAsking(false);
      setAskStartedAt(null);
      if (!failed) {
        setStreamStatus('');
      }
    }
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
                    <button disabled={asking || !workspaceReady} type="submit">
                      {asking ? 'Asking' : 'Ask'}
                    </button>
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
                        {asking && <span className="spinner" aria-hidden="true"></span>}
                        <strong>
                          {askPhase === 'failed'
                            ? 'Request failed'
                            : askPhaseLabel(askPhase, visibleAskAnswer)}
                        </strong>
                        <em>{formatElapsed(askElapsedSeconds)}</em>
                      </div>
                    )}
                    {askPhase === 'failed' ? (
                      <div className="failureNotice compactFailure">
                        <strong>Answer failed</strong>
                        <span>{streamStatus || 'The request did not complete.'}</span>
                      </div>
                    ) : (
                      <p className={asking ? 'answerText answerTextStreaming' : 'answerText'}>
                        {visibleAskAnswer || askWaitingLabel(askPhase)}
                      </p>
                    )}
                    {askResult && (
                      <details className="inlineDetails">
                        <summary>Sources</summary>
                        <div className="resultStack">
                          {askResult.hits.map((hit, index) => (
                            <ResultHit
                              hit={hit}
                              index={index}
                              key={`${hit.document_id}:${hit.chunk_id}`}
                            />
                          ))}
                          {askResult.hits.length === 0 && <p className="muted">No sources</p>}
                        </div>
                      </details>
                    )}
                  </div>
                )}
              </div>

              <aside className="workSurface contextPanel">
                <div className="surfaceHeader">
                  <h2>Context</h2>
                  <span>{askForm.document_id ? 'Scoped' : 'All documents'}</span>
                </div>

                <form className="uploadBar compactUpload" onSubmit={submitUpload}>
                  <input
                    aria-label="Document"
                    accept={supportedDocumentAccept}
                    type="file"
                    onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                  />
                  <button
                    disabled={submitting || uploadingSample || !workspaceReady}
                    onClick={() => void uploadSampleDocument()}
                    type="button"
                  >
                    {uploadingSample ? 'Adding' : 'Sample'}
                  </button>
                  <button disabled={submitting || uploadingSample || !workspaceReady} type="submit">
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
                      <div className="resultStack">
                        {searchResult.hits.map((hit, index) => (
                          <ResultHit
                            hit={hit}
                            index={index}
                            key={`${hit.document_id}:${hit.chunk_id}`}
                          />
                        ))}
                        {searchResult.hits.length === 0 && <p className="muted">No matches</p>}
                      </div>
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
                  <button disabled={creatingTenant} type="submit">
                    {creatingTenant ? 'Creating' : 'Create'}
                  </button>
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
                  <span className="syncStatus">{dataSources?.sources.length ?? 0} active</span>
                  <button
                    onClick={() => void Promise.all([refreshDataSources(), refreshJobs()])}
                    type="button"
                  >
                    Refresh
                  </button>
                </div>
              </div>

              <details className="inlineDetails">
                <summary>Add source</summary>
                <form className="sourceForm" onSubmit={submitDataSource}>
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
                    placeholder="C:\\Docs, /mnt/docs, or \\\\server\\share"
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
                  <button disabled={creatingSource || !workspaceReady} type="submit">
                    {creatingSource ? 'Adding' : 'Add'}
                  </button>
                </form>
              </details>

              <div className="tableList sourceList">
                {dataSources?.sources.map((source) => {
                  const scanJob = activeSourceScanJobs.get(source.id);
                  return (
                    <div
                      className={
                        sourceDetail?.source.id === source.id ? 'sourceRow selectedRow' : 'sourceRow'
                      }
                      key={source.id}
                    >
                      <button onClick={() => void openSource(source)} type="button">
                        <strong>{source.name}</strong>
                      </button>
                      <span className={stateClass(scanJob?.state ?? source.status)}>
                        {scanJob ? `scan ${scanJob.state}` : source.status}
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
                              Boolean(scanJob) ||
                              scanningSourceID === source.id ||
                              archivingSourceID === source.id
                            }
                            onClick={() => void requestDataSourceScan(source)}
                            type="button"
                          >
                            {scanningSourceID === source.id
                              ? 'Queuing'
                              : scanJob
                                ? titleCase(scanJob.state)
                                : 'Rescan'}
                          </button>
                          <button
                            disabled={
                              Boolean(scanJob) ||
                              reindexingSourceID === source.id ||
                              source.status === 'archived' ||
                              archivingSourceID === source.id ||
                              deletingSourceDocumentsID === source.id
                            }
                            onClick={() => void requestDataSourceReindex(source)}
                            type="button"
                          >
                            {reindexingSourceID === source.id ? 'Reindexing' : 'Reindex'}
                          </button>
                          <button
                            className="dangerButton"
                            disabled={
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
                {!dataSources && <p className="muted">Loading sources</p>}
              </div>

              {sourceDetail && (
                <div className="detailPanel">
                  <div className="detailHeader">
                    <h3>{sourceDetail.source.name}</h3>
                    <div className="detailActions">
                      <span
                        className={stateClass(
                          activeSourceScanJobs.get(sourceDetail.source.id)?.state ??
                            sourceDetail.source.status,
                        )}
                      >
                        {activeSourceScanJobs.get(sourceDetail.source.id)
                          ? `scan ${activeSourceScanJobs.get(sourceDetail.source.id)?.state}`
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
                              reindexingSourceID === sourceDetail.source.id ||
                              sourceDetail.source.status === 'archived' ||
                              Boolean(activeSourceScanJobs.get(sourceDetail.source.id)) ||
                              sourceHasActiveScanJob(sourceDetail)
                            }
                            onClick={() => void requestDataSourceReindex(sourceDetail.source)}
                            type="button"
                          >
                            {reindexingSourceID === sourceDetail.source.id
                              ? 'Reindexing'
                              : 'Reindex'}
                          </button>
                          <button
                            className="dangerButton"
                            disabled={
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
                              deletingSourceDocumentsID === sourceDetail.source.id ||
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
                  {sourceDetail.source.status === 'failed' && sourceFailureMessage(sourceDetail) && (
                    <div className="failureNotice">
                      <strong>Scan failed</strong>
                      <span>{sourceFailureMessage(sourceDetail)}</span>
                    </div>
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
                  </dl>
                  {sourceDetail.scan_summary.total > 0 && (
                    <section className="sourceScanSummary" aria-label="Latest scan summary">
                      <div>
                        <strong>{sourceDetail.scan_summary.total}</strong>
                        <span>Files</span>
                      </div>
                      <div>
                        <strong>{sourceDetail.scan_summary.imported}</strong>
                        <span>Imported</span>
                      </div>
                      <div>
                        <strong>{sourceDetail.scan_summary.skipped}</strong>
                        <span>Skipped</span>
                      </div>
                      <div>
                        <strong>{sourceDetail.scan_summary.failed}</strong>
                        <span>Failed</span>
                      </div>
                      <div>
                        <strong>{sourceDetail.scan_summary.deleted}</strong>
                        <span>Deleted</span>
                      </div>
                      <div className="scanReasonSummary">
                        <strong>{scanSummaryReasons(sourceDetail.scan_summary) || 'Clean'}</strong>
                        <span>Reasons</span>
                      </div>
                    </section>
                  )}
                  <details className="inlineDetails">
                    <summary>Edit</summary>
                    <form className="sourceForm sourceEditForm" onSubmit={submitSourceUpdate}>
                      <input
                        aria-label="Edit source name"
                        disabled={
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
                      <div className="sourcePatternFields">
                        <label>
                          <span>Include</span>
                          <textarea
                            aria-label="Edit source include patterns"
                            disabled={
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
                      <div className="scanEntryPager">
                        <select
                          aria-label="Filter file outcomes"
                          onChange={(event) =>
                            void loadSourceScanEntries(event.target.value as ScanEntryFilter, 0)
                          }
                          value={scanEntryFilter}
                        >
                          <option value="all">All outcomes</option>
                          <option value="failed">Failed</option>
                          <option value="skipped">Skipped</option>
                          <option value="imported">Imported</option>
                          <option value="deleted">Deleted</option>
                        </select>
                        <button onClick={exportSourceScanEntries} type="button">
                          CSV
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
                type="file"
                onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              />
              <button
                disabled={submitting || uploadingSample || !workspaceReady}
                onClick={() => void uploadSampleDocument()}
                type="button"
              >
                {uploadingSample ? 'Adding' : 'Sample'}
              </button>
              <button disabled={submitting || uploadingSample || !workspaceReady} type="submit">
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

            <div className="tableList">
              {documents?.documents.map((document) => (
                <div className="documentRow" key={document.id}>
                  <button onClick={() => void openDocument(document)} type="button">
                    <strong>{document.name}</strong>
                  </button>
                  <span className={stateClass(document.status)}>{document.status}</span>
                  <em>{document.id}</em>
                  <small>{formatBytes(document.size_bytes)}</small>
                  <details className="rowMenu">
                    <summary>More</summary>
                    <div className="rowMenuActions">
                      <button
                        disabled={loadingDocumentID === document.id}
                        onClick={() => void openDocument(document)}
                        type="button"
                      >
                        {loadingDocumentID === document.id ? 'Loading' : 'Details'}
                      </button>
                      <button
                        disabled={downloadingDocumentID === document.id}
                        onClick={() => void downloadDocumentSource(document)}
                        type="button"
                      >
                        {downloadingDocumentID === document.id ? 'Downloading' : 'Download'}
                      </button>
                      {document.status === 'failed' && (
                        <button
                          disabled={
                            retryingDocumentID === document.id ||
                            activeIngestionDocumentIDs.has(document.id)
                          }
                          onClick={() => void retryDocumentIngestion(document.id)}
                          type="button"
                        >
                          {retryingDocumentID === document.id
                            ? 'Retrying'
                            : activeIngestionDocumentIDs.has(document.id)
                              ? 'Queued'
                              : 'Retry'}
                        </button>
                      )}
                      <button
                        className="dangerButton"
                        disabled={deletingDocumentID === document.id}
                        onClick={() => void removeDocument(document.id, document.name)}
                        type="button"
                      >
                        {deletingDocumentID === document.id ? 'Deleting' : 'Delete'}
                      </button>
                    </div>
                  </details>
                </div>
              ))}
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
                        <strong>{job.type}</strong>
                        <span className={stateClass(job.state)}>{job.state}</span>
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
                </div>
              </details>
              <button disabled={searching || !workspaceReady} type="submit">
                {searching ? 'Searching' : 'Search'}
              </button>
            </form>
            {searchResult && (
              <div className="resultStack">
                <div className="resultToolbar">
                  <strong>{searchResult.hits.length} passages</strong>
                  <span>{searchForm.query}</span>
                </div>
                {searchResult.hits.map((hit, index) => (
                  <ResultHit
                    hit={hit}
                    index={index}
                    key={`${hit.document_id}:${hit.chunk_id}`}
                  />
                ))}
                {searchResult.hits.length === 0 && <p className="muted">No matches</p>}
              </div>
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
                <form className="inlineForm" onSubmit={submitTenant}>
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
                          ? `${filteredAuditEvents.length}/${auditEvents.events.length} recent`
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
                        onClick={() => setAuditFilters(initialAuditFilters)}
                        type="button"
                      >
                        Clear
                      </button>
                      <button
                        disabled={loadingAudit}
                        onClick={() => void refreshAuditEvents()}
                        type="button"
                      >
                        {loadingAudit ? 'Refreshing' : 'Refresh'}
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

function ResultHit({
  hit,
  index,
}: {
  hit: SearchDocumentsResponse['hits'][number];
  index: number;
}) {
  const documentName = hit.source?.document_name || hit.metadata.document_name || hit.document_id;
  const chunkLabel = formatChunkLabel(hit);
  const sourceTitle = `${hit.document_id} / ${hit.chunk_id}`;

  return (
    <div className="searchHit" title={sourceTitle}>
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
    </div>
  );
}

function DashboardAttentionPanel({
  activeDocuments,
  activeJobs,
  failedDocuments,
  failedJobs,
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
  const totalAttention =
    failedDocuments.length + otherFailedJobs.length + activeAttentionCount;

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

      <div className="attentionFeed">
        {failedDocuments.slice(0, 2).map((document) => (
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
        {otherFailedJobs.slice(0, 2).map((job) => (
          <button
            className="attentionItem attentionItemFailed"
            key={job.id}
            onClick={onViewActivity}
            title={jobTitle(job)}
            type="button"
          >
            <strong>{job.type}</strong>
            <span className={stateClass(job.state)}>{job.state}</span>
            <em>{jobDetail(job)}</em>
          </button>
        ))}
        {!hasFailures &&
          activeDocuments.slice(0, 2).map((document) => (
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
          otherActiveJobs.slice(0, 3).map((job) => (
            <button
              className="attentionItem"
              key={job.id}
              onClick={onViewActivity}
              title={jobTitle(job)}
              type="button"
            >
              <strong>{job.type}</strong>
              <span className={stateClass(job.state)}>{job.state}</span>
              <em>{formatDateTime(job.updated_at)}</em>
            </button>
          ))}
        {!hasFailures && !hasActive && (
          <div className="attentionReady">
            <strong>Ready</strong>
            <span>{readyDocumentCount > 0 ? 'Knowledge base idle' : 'Add documents to begin'}</span>
          </div>
        )}
      </div>

      <div className="attentionActions">
        <button onClick={onRefresh} type="button">
          Refresh
        </button>
        <button onClick={totalAttention > 0 ? onViewActivity : onViewLibrary} type="button">
          {totalAttention > 0 ? 'Activity' : 'Library'}
        </button>
      </div>
    </section>
  );
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
  setupChecking: boolean;
}) {
  const blockingCount = checks.filter(
    (check) => check.blocking && check.status === 'blocked',
  ).length;
  const commandProfile = readiness?.deployment_profile || 'cpu-lite';
  const commandGateway = readiness?.model_gateway || 'http://host.docker.internal:11434/v1';
  const commandModel = primaryTarget?.model || 'your-model-name';

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
            <div>
              <dt>Result</dt>
              <dd>{gatewayCheck?.detail ?? 'Waiting for check'}</dd>
            </div>
          </dl>

          <details className="inlineDetails" open={!canEnter}>
            <summary>Configure</summary>
            <pre className="setupCode">{`.\\scripts\\setup.ps1 -Profile ${commandProfile} -ProviderPreset ${readiness?.provider_preset || 'starter'} -ModelGatewayBaseUrl "${commandGateway}" -GeneralModelId "${commandModel}" -Force`}</pre>
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

function sourceFormFromSource(source: ListDataSourcesResponse['sources'][number]) {
  return {
    type: source.type,
    name: source.name,
    root_path: source.root_path,
    include_patterns: (source.include_patterns ?? []).join('\n'),
    exclude_patterns: (source.exclude_patterns ?? []).join('\n'),
    scan_interval_minutes: String(source.scan_interval_minutes ?? 0),
  };
}

function patternLinesToList(value: string) {
  return value
    .split(/\r?\n|,/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith('#'));
}

function sourceScanSummary(source: ListDataSourcesResponse['sources'][number]) {
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
  return friendlyErrorMessage(message);
}

function friendlyErrorMessage(message: string) {
  const lower = message.toLowerCase();
  if (lower.includes('model gateway') && lower.includes('404')) {
    return 'Model gateway not found. Check the provider URL and model in Settings.';
  }
  if (lower.includes('model gateway') && lower.includes('status')) {
    return 'Model gateway is not responding correctly. Check Settings, then test the target.';
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

function scanSummaryReasons(summary: DataSourceDetailResponse['scan_summary']) {
  return Object.entries(summary.reasons)
    .sort(([, a], [, b]) => b - a)
    .slice(0, 2)
    .map(([reason, count]) => `${titleCase(reason.replace(/_/g, ' '))} ${count}`)
    .join(', ');
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

function filterAuditEvents(
  events: ListAuditEventsResponse['events'],
  filters: typeof initialAuditFilters,
) {
  const query = filters.query.trim().toLowerCase();
  return events.filter((event) => {
    if (filters.action && event.action !== filters.action) {
      return false;
    }
    if (filters.outcome && event.outcome !== filters.outcome) {
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

function formatTimeOnly(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(new Date(value));
}
