import { lazy, Suspense, useEffect, useState, type FormEvent } from 'react';
import { createRoot } from 'react-dom/client';
import {
  Activity,
  ArrowDownToLine,
  ArrowRight,
  Check,
  ChevronRight,
  CircleAlert,
  FlaskConical,
  GitCompareArrows,
  History,
  Layers3,
  Play,
  ShieldCheck,
  Timer,
  X,
} from 'lucide-react';
const ReleaseScene = lazy(() => import('./ReleaseScene'));
import './style.css';

type CheckResult = { name: string; passed: boolean; detail: string };
type ResponseData = { response: unknown; latencyMs: number };
type Result = {
  case: {
    id: string;
    title: string;
    locale: string;
    input: string;
    critical: boolean;
  };
  baseline: ResponseData;
  candidate: ResponseData;
  baselineChecks: CheckResult[];
  checks: CheckResult[];
  regression: boolean;
  needsReview: boolean;
  review?: string;
};
type Run = {
  id: string;
  suite: string;
  baseline: string;
  candidate: string;
  createdAt: string;
  results: Result[];
  status: string;
};

async function api<T>(path: string, method = 'GET', data?: object): Promise<T> {
  const response = await fetch('/api/' + path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: data ? JSON.stringify(data) : undefined,
  });
  const payload = await response.json();
  if (!response.ok) throw new Error(payload.error ?? 'Request failed.');
  return payload as T;
}

function statusClass(value: string) {
  if (['Blocked', 'Regression', 'Rejected', 'Failed'].includes(value))
    return 'danger';
  if (['Review required', 'Warning'].includes(value)) return 'caution';
  return 'good';
}

function Badge({ value }: { value: string }) {
  return (
    <span className={'badge ' + statusClass(value)}>
      <i />
      {value}
    </span>
  );
}

function resultStatus(result: Result) {
  if (result.review === 'rejected') return 'Rejected';
  if (result.regression) return 'Regression';
  if (result.checks.some((check) => !check.passed)) return 'Failed';
  if (result.needsReview && !result.review) return 'Review required';
  return 'Passed';
}

function ResponseCard({
  title,
  version,
  output,
  locale,
}: {
  title: string;
  version: string;
  output: ResponseData;
  locale: string;
}) {
  const value = output.response as {
    answer?: unknown;
    sources?: unknown;
  } | null;
  return (
    <article className="response">
      <div className="response-head">
        <strong>{title}</strong>
        <code>{version}</code>
        <span>{output.latencyMs} ms</span>
      </div>
      {typeof value?.answer === 'string' ? (
        <p lang={locale} dir="auto">
          {value.answer}
        </p>
      ) : (
        <pre>{JSON.stringify(output.response, null, 2)}</pre>
      )}
      {Array.isArray(value?.sources) && (
        <div className="sources">
          {value.sources
            .filter((source) => typeof source === 'string')
            .map((source, index) => (
              <code key={index}>{source}</code>
            ))}
        </div>
      )}
    </article>
  );
}

function Dashboard() {
  const [versions, setVersions] = useState<string[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);
  const [runID, setRunID] = useState('');
  const [caseID, setCaseID] = useState('');
  const [baseline, setBaseline] = useState('support-v1');
  const [candidate, setCandidate] = useState('support-v2');
  const [language, setLanguage] = useState('all');
  const [filter, setFilter] = useState('all');
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [publicDemo, setPublicDemo] = useState(false);
  const [importsEnabled, setImportsEnabled] = useState(true);
  const [tab, setTab] = useState<'workbench' | 'history'>('workbench');

  useEffect(() => {
    api<{
      versions: string[];
      publicDemo: boolean;
      importsEnabled: boolean;
    }>('config')
      .then((config) => {
        setVersions(config.versions);
        setPublicDemo(config.publicDemo);
        setImportsEnabled(config.importsEnabled);
        return api<Run[]>('runs');
      })
      .then((saved) => {
        setRuns(saved);
        setRunID(saved[0]?.id ?? '');
      })
      .catch((reason: Error) => setError(reason.message))
      .finally(() => setLoading(false));
  }, []);

  const run = runs.find((item) => item.id === runID);
  const cases = run?.results ?? [];
  const visible = cases.filter(
    (item) =>
      (language === 'all' || item.case.locale === language) &&
      (filter === 'all' || resultStatus(item) === filter),
  );
  const selected =
    visible.find((item) => item.case.id === caseID) ?? visible[0];
  const regressions = cases.filter((item) => item.regression).length;
  const pending = cases.filter(
    (item) => item.needsReview && !item.review,
  ).length;
  const passed = cases.filter((item) =>
    item.checks.every((check) => check.passed),
  ).length;

  function showComparison(created: Run) {
    setRuns((items) => [created, ...items]);
    selectRun(created.id);
    setCaseID(created.results.find((item) => item.regression)?.case.id ?? '');
  }

  function selectRun(id: string) {
    setRunID(id);
    setCaseID('');
    setLanguage('all');
    setFilter('all');
    setTab('workbench');
  }

  async function start(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const created = await api<Run>('runs', 'POST', { baseline, candidate });
      showComparison(created);
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : 'Could not start the comparison.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function importComparison(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const files = new FormData(event.currentTarget);
    setBusy(true);
    setError('');
    try {
      const payload: Record<string, unknown> = {};
      for (const field of ['suite', 'baseline', 'candidate']) {
        const file = files.get(field);
        if (
          !(file instanceof File) ||
          file.size === 0 ||
          file.size > 512 * 1024
        ) {
          throw new Error(
            'Choose three nonempty JSON files under 512 KiB total.',
          );
        }
        payload[field] = JSON.parse(await file.text());
      }
      const created = await api<Run>('import', 'POST', payload);
      showComparison(created);
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : 'Could not import files.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function saveReview(decision: string) {
    if (!run || !selected) return;
    setBusy(true);
    setError('');
    try {
      const updated = await api<Run>(
        `runs/${run.id}/cases/${selected.case.id}`,
        'PATCH',
        { decision },
      );
      setRuns((items) =>
        items.map((item) => (item.id === updated.id ? updated : item)),
      );
    } catch (reason) {
      setError(
        reason instanceof Error ? reason.message : 'Could not save review.',
      );
    } finally {
      setBusy(false);
    }
  }

  function exportRun() {
    if (!run) return;
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(run, null, 2)], { type: 'application/json' }),
    );
    const link = document.createElement('a');
    link.href = url;
    link.download = `evalgate-${run.id}.json`;
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }

  return (
    <div className="shell">
      <aside className="rail">
        <div className="brand">
          <span>
            <Layers3 size={21} />
          </span>
          <div>
            EvalGate<small>AI RELEASE QUALITY</small>
          </div>
        </div>
        <nav aria-label="Workspace">
          <button
            aria-pressed={tab === 'workbench'}
            className={tab === 'workbench' ? 'active' : ''}
            onClick={() => setTab('workbench')}
          >
            <GitCompareArrows size={17} /> Workbench
          </button>
          <button
            aria-pressed={tab === 'history'}
            className={tab === 'history' ? 'active' : ''}
            onClick={() => setTab('history')}
          >
            <History size={17} /> Run history
          </button>
        </nav>
        <div className="rail-foot">
          <span className="live-dot" />{' '}
          {publicDemo ? 'Private demo session' : 'Offline fixtures'}
        </div>
      </aside>
      <main>
        <header className="top">
          <span>
            Release engineering <ChevronRight size={14} />{' '}
            <strong>{tab === 'workbench' ? 'Workbench' : 'Run history'}</strong>
          </span>
          <span className="top-mode">
            <FlaskConical size={15} />{' '}
            {publicDemo ? 'Public fixture demo' : 'Demo workspace'}
          </span>
        </header>
        <div className="content">
          {error && (
            <p className="error" role="alert">
              {error}
            </p>
          )}
          {tab === 'history' ? (
            <section className="history panel">
              <h1>Run history</h1>
              <p>
                Open a saved comparison to inspect its checks and review
                decisions.
              </p>
              {runs.length === 0 && (
                <p>{loading ? 'Loading runs…' : 'No comparisons saved yet.'}</p>
              )}
              {runs.map((item) => (
                <button
                  className="history-row"
                  key={item.id}
                  onClick={() => selectRun(item.id)}
                >
                  <span>
                    <strong>
                      {item.baseline} → {item.candidate}
                    </strong>
                    <small>{new Date(item.createdAt).toLocaleString()}</small>
                  </span>
                  <Badge value={item.status} />
                  <ChevronRight size={16} />
                </button>
              ))}
            </section>
          ) : (
            <>
              {publicDemo && (
                <aside className="demo-notice panel">
                  <ShieldCheck size={19} />
                  <div>
                    <strong>Safe, session-only demo</strong>
                    <span>
                      Runs and review decisions are private to this browser
                      session. History is temporary and can reset when the
                      free service restarts.
                    </span>
                  </div>
                </aside>
              )}
              <section className="hero">
                <div className="hero-copy">
                  <span className="eyebrow">
                    <Activity size={16} /> RELEASE WORKBENCH
                  </span>
                  <h1>
                    Test the change.
                    <br /> <em>Then ship.</em>
                  </h1>
                  <p>
                    Compare two versions of a bilingual support assistant and
                    see exactly which checks changed.
                  </p>
                </div>
                <Suspense
                  fallback={<div className="scene" aria-hidden="true" />}
                >
                  <ReleaseScene blocked={run?.status === 'Blocked'} />
                </Suspense>
              </section>
              <form className="run-form panel" onSubmit={start}>
                <label>
                  Baseline
                  <select
                    value={baseline}
                    onChange={(event) => setBaseline(event.target.value)}
                  >
                    {versions.map((version) => (
                      <option key={version}>{version}</option>
                    ))}
                  </select>
                </label>
                <ArrowRight className="form-arrow" size={19} />
                <label>
                  Candidate
                  <select
                    value={candidate}
                    onChange={(event) => setCandidate(event.target.value)}
                  >
                    {versions.map((version) => (
                      <option key={version}>{version}</option>
                    ))}
                  </select>
                </label>
                <button
                  className="primary"
                  disabled={
                    loading ||
                    busy ||
                    baseline === candidate ||
                    !versions.length
                  }
                >
                  <Play size={15} fill="currentColor" />{' '}
                  {busy ? 'Working…' : 'Run comparison'}
                </button>
              </form>
              {importsEnabled && (
                <details className="import panel">
                  <summary>Compare your own JSON files</summary>
                  <form onSubmit={importComparison}>
                    <label>
                      Test suite
                      <input
                        name="suite"
                        type="file"
                        accept=".json,application/json"
                        required
                      />
                    </label>
                    <label>
                      Baseline responses
                      <input
                        name="baseline"
                        type="file"
                        accept=".json,application/json"
                        required
                      />
                    </label>
                    <label>
                      Candidate responses
                      <input
                        name="candidate"
                        type="file"
                        accept=".json,application/json"
                        required
                      />
                    </label>
                    <button className="primary" disabled={busy}>
                      Import and compare
                    </button>
                  </form>
                </details>
              )}
              {run ? (
                <>
                  <div className="section-title">
                    <div>
                      <h2>{run.suite}</h2>
                      <p>
                        {run.baseline} → {run.candidate}
                      </p>
                    </div>
                    <Badge value={run.status} />
                    <button className="secondary" onClick={exportRun}>
                      <ArrowDownToLine size={15} /> JSON
                    </button>
                  </div>
                  <div className="metrics">
                    <article className="panel">
                      <ShieldCheck size={18} />
                      <div>
                        <strong>
                          {passed}
                          <small> / {cases.length}</small>
                        </strong>
                        <span>Passed all automatic checks</span>
                      </div>
                    </article>
                    <article className="panel">
                      <CircleAlert size={18} />
                      <div>
                        <strong>{regressions}</strong>
                        <span>Cases with regressions</span>
                      </div>
                    </article>
                    <article className="panel">
                      <Timer size={18} />
                      <div>
                        <strong>{pending}</strong>
                        <span>Awaiting human review</span>
                      </div>
                    </article>
                  </div>
                  <div className="workspace">
                    <section className="panel cases">
                      <h2>Test cases</h2>
                      <div className="filters">
                        <select
                          aria-label="Filter by language"
                          value={language}
                          onChange={(event) => setLanguage(event.target.value)}
                        >
                          <option value="all">All languages</option>
                          <option value="ar">Arabic</option>
                          <option value="en">English</option>
                        </select>
                        <select
                          aria-label="Filter by result"
                          value={filter}
                          onChange={(event) => setFilter(event.target.value)}
                        >
                          <option value="all">All results</option>
                          {[
                            'Regression',
                            'Failed',
                            'Review required',
                            'Rejected',
                            'Passed',
                          ].map((value) => (
                            <option key={value}>{value}</option>
                          ))}
                        </select>
                      </div>
                      {visible.length === 0 && (
                        <p>No cases match these filters.</p>
                      )}
                      {visible.map((item) => (
                        <button
                          className={
                            'case-row ' +
                            (selected?.case.id === item.case.id
                              ? 'selected'
                              : '')
                          }
                          key={item.case.id}
                          onClick={() => setCaseID(item.case.id)}
                        >
                          <span
                            className={
                              'case-dot ' + statusClass(resultStatus(item))
                            }
                          />
                          <span>
                            <strong>{item.case.title}</strong>
                            <small>
                              {item.case.locale.toUpperCase()} ·{' '}
                              {resultStatus(item)}
                            </small>
                          </span>
                          <ChevronRight size={15} />
                        </button>
                      ))}
                    </section>
                    {selected && (
                      <section className="panel detail" key={selected.case.id}>
                        <div className="detail-title">
                          <h2>{selected.case.title}</h2>
                          {selected.case.critical && (
                            <span className="critical">Critical case</span>
                          )}
                        </div>
                        <p
                          className="input"
                          lang={selected.case.locale}
                          dir="auto"
                        >
                          {selected.case.input}
                        </p>
                        <div className="responses">
                          <ResponseCard
                            title="Baseline"
                            version={run.baseline}
                            output={selected.baseline}
                            locale={selected.case.locale}
                          />
                          <ResponseCard
                            title="Candidate"
                            version={run.candidate}
                            output={selected.candidate}
                            locale={selected.case.locale}
                          />
                        </div>
                        <h3>Automatic checks</h3>
                        <ul className="checks">
                          {selected.checks.map((check, index) => (
                            <li key={check.name}>
                              {check.passed ? (
                                <Check className="pass" size={16} />
                              ) : (
                                <X className="fail" size={16} />
                              )}
                              <strong>{check.name}</strong>
                              <span dir="auto">{check.detail}</span>
                              {!check.passed &&
                                selected.baselineChecks[index]?.passed && (
                                  <small>Regressed</small>
                                )}
                            </li>
                          ))}
                        </ul>
                        {selected.needsReview && (
                          <div className="review">
                            <p>
                              <strong>Human review</strong>
                              <br />
                              {selected.review
                                ? `Decision saved: ${selected.review}.`
                                : 'Decide whether the changed wording is acceptable.'}
                            </p>
                            <button
                              className="secondary"
                              disabled={busy || selected.review === 'rejected'}
                              onClick={() => void saveReview('rejected')}
                            >
                              Reject
                            </button>
                            <button
                              className="primary"
                              disabled={busy || selected.review === 'approved'}
                              onClick={() => void saveReview('approved')}
                            >
                              Approve
                            </button>
                          </div>
                        )}
                      </section>
                    )}
                  </div>
                </>
              ) : (
                <section className="empty panel">
                  <GitCompareArrows size={23} />
                  <h2>
                    {loading ? 'Loading workspace…' : 'Start a comparison'}
                  </h2>
                  <p>
                    Compare support-v1 with support-v2 to inspect the Arabic
                    refund regression.
                  </p>
                </section>
              )}
            </>
          )}
          <footer>
            EvalGate{' '}
            <span>Fixture responses and latency samples · Tasnim Makableh</span>
          </footer>
        </div>
      </main>
    </div>
  );
}

createRoot(document.getElementById('root')!).render(<Dashboard />);
