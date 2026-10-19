import { useCallback, useEffect, useMemo, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import './components.css';

type Call = {
  id: string;
  timestamp: string;
  type: string;
  call_type: string;
  incident_number: string;
  vehicles: string[] | string | null;
  address: string;
  intersection: string;
  latitude?: number;
  longitude?: number;
  client_id: string;
  station: string;
  source?: string;
  text?: string;
  raw?: unknown;
};

type Range = '24h' | '7d' | 'all';
type Result = { items: Call[]; total: number; nextCursor?: string; hasMore: boolean; serverFiltered: boolean };

const PAGE_SIZE = 25;
const POLL_MS = 15_000;

function vehicles(value: Call['vehicles']) {
  if (Array.isArray(value)) return value.map(String).map((v) => v.trim()).filter(Boolean);
  return String(value || '').split(/[,/;]+/).map((v) => v.trim()).filter(Boolean);
}

function text(value: unknown, fallback = '—') {
  return value === null || value === undefined || String(value).trim() === '' ? fallback : String(value);
}

function normalize(raw: Record<string, unknown>, index: number): Call {
  return {
    id: text(raw.id ?? raw.message_id, `call-${index}`),
    timestamp: text(raw.timestamp ?? raw.created_at ?? raw.time, new Date().toISOString()),
    type: text(raw.type, 'CALL'), call_type: text(raw.call_type ?? raw.callType, 'Unclassified'),
    incident_number: text(raw.incident_number ?? raw.incidentNumber, 'N/A'),
    vehicles: raw.vehicles as Call['vehicles'], address: text(raw.address), intersection: text(raw.intersection, ''),
    latitude: Number(raw.latitude) || undefined, longitude: Number(raw.longitude) || undefined,
    client_id: text(raw.client_id ?? raw.clientId), station: text(raw.station),
    source: text(raw.source), text: text(raw.text, ''), raw
  };
}

function payloadCalls(data: unknown): Result {
  const object = data && typeof data === 'object' ? data as Record<string, unknown> : {};
  const list = Array.isArray(data) ? data : (object.items ?? object.calls ?? object.messages ?? []);
  const items = Array.isArray(list) ? list.map((entry, index) => normalize((entry || {}) as Record<string, unknown>, index)).filter((call) => call.type.toUpperCase() === 'CALL') : [];
  const total = Number(object.total ?? object.count ?? items.length);
  return { items, total: Number.isFinite(total) ? total : items.length, nextCursor: text(object.next_cursor, ''), hasMore: Boolean(object.has_more), serverFiltered: !Array.isArray(data) && Boolean(object.items || object.calls || object.messages) };
}

function queryFor(range: Range, search: string, station: string, type: string, cursor = '') {
  const params = new URLSearchParams({ limit: String(PAGE_SIZE), type: 'CALL' });
  if (cursor) params.set('cursor', cursor);
  if (range !== 'all') {
    const from = new Date(Date.now() - (range === '24h' ? 1 : 7) * 86_400_000);
    params.set('from', from.toISOString());
  }
  if (search.trim()) params.set('search', search.trim());
  if (station !== 'all') params.set('station', station);
  if (type !== 'all') params.set('call_type', type);
  return params.toString();
}

function formatTime(timestamp: string) {
  const date = new Date(timestamp);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function isRecent(timestamp: string, range: Range) {
  if (range === 'all') return true;
  const age = Date.now() - new Date(timestamp).getTime();
  return age >= 0 && age <= (range === '24h' ? 1 : 7) * 86_400_000;
}

export function OperatorDashboard() {
  const { token, user } = useAuth();
  const [calls, setCalls] = useState<Call[]>([]);
  const [total, setTotal] = useState(0);
  const [range, setRange] = useState<Range>('24h');
  const [station, setStation] = useState('all');
  const [callType, setCallType] = useState('all');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [cursors, setCursors] = useState<Record<number, string>>({});
  const [selected, setSelected] = useState<Call | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [stale, setStale] = useState(false);
  const [serverMode, setServerMode] = useState(false);
  const [metricSummary, setMetricSummary] = useState<Record<string, unknown> | null>(null);
  const pageCursor = cursors[page] || '';

  const load = useCallback(async (background = false) => {
    if (!token) return;
    if (!background) setLoading(true);
    try {
       const query = queryFor(range, search, station, callType, pageCursor);
      let response = await fetch(`/api/calls?${query}`, { headers: { Authorization: `Bearer ${token}` } });
       if (!response.ok && (response.status === 404 || response.status === 405)) {
         response = await fetch(`/api/messages?${query}`, { headers: { Authorization: `Bearer ${token}` } });
      }
      if (!response.ok) throw new Error(`Unable to load calls (${response.status})`);
      const result = payloadCalls(await response.json());
       setCalls(result.items); setTotal(result.total); setServerMode(result.serverFiltered);
       const nextCursor = result.nextCursor;
       if (nextCursor) setCursors((current) => ({ ...current, [page + 1]: nextCursor }));
      // Newer deployments expose an aggregate endpoint. It is advisory: the
      // table and local-derived metrics remain fully compatible with legacy APIs.
      try {
         let metricsResponse = await fetch(`/api/metrics/summary?${query}`, { headers: { Authorization: `Bearer ${token}` } });
         if (!metricsResponse.ok) metricsResponse = await fetch(`/api/metrics?${query}`, { headers: { Authorization: `Bearer ${token}` } });
         setMetricSummary(metricsResponse.ok ? await metricsResponse.json() as Record<string, unknown> : null);
      } catch { setMetricSummary(null); }
      setError(''); setStale(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load calls');
      setStale(true);
    } finally { setLoading(false); }
  }, [token, range, search, station, callType, page, pageCursor]);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => {
    const interval = window.setInterval(() => void load(true), POLL_MS);
    return () => window.clearInterval(interval);
  }, [load]);

  const visible = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (serverMode) return calls;
    return calls.filter((call) => isRecent(call.timestamp, range)
      && (station === 'all' || call.station === station)
      && (callType === 'all' || call.call_type === callType)
      && (!query || [call.incident_number, call.address, call.call_type, call.station, call.client_id].join(' ').toLowerCase().includes(query))
    ).slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
  }, [calls, serverMode, range, station, callType, search, page]);

  const facets = useMemo(() => ({
    stations: [...new Set(calls.map((call) => call.station).filter((value) => value !== '—'))].slice(0, 100),
    types: [...new Set(calls.map((call) => call.call_type).filter((value) => value !== '—'))].slice(0, 100)
  }), [calls]);
  const stats = useMemo(() => {
    const values = serverMode ? calls : calls.filter((call) => isRecent(call.timestamp, range));
    const byType = new Map<string, number>();
    values.forEach((call) => byType.set(call.call_type, (byType.get(call.call_type) || 0) + 1));
    const summaryTypes = metricSummary?.by_type;
    if (Array.isArray(summaryTypes)) summaryTypes.forEach((item) => { const entry = item as Record<string, unknown>; const label = text(entry.label ?? entry.name, 'Unclassified'); const value = Number(entry.value ?? entry.count); if (Number.isFinite(value)) byType.set(label, value); });
    return { incidents: Number(metricSummary?.distinct_incidents) || new Set(values.map((call) => call.incident_number)).size, stations: Number(metricSummary?.active_stations) || new Set(values.map((call) => call.station)).size, units: Number(metricSummary?.vehicle_mentions) || values.reduce((sum, call) => sum + vehicles(call.vehicles).length, 0), byType: [...byType.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5) };
  }, [calls, serverMode, range, metricSummary]);
  const pages = serverMode ? Math.max(1, page + (calls.length === PAGE_SIZE && cursors[page + 1] ? 1 : 0)) : Math.max(1, Math.ceil(calls.length / PAGE_SIZE));

  const changeFilter = (setter: (value: string) => void, value: string) => { setter(value); setPage(1); setCursors({}); };

  return <div className="operator-shell">
    <section className="operator-hero">
      <div><p className="eyebrow">Operations / live queue</p><h2>Operator console</h2><p className="operator-subtitle">Monitor incoming incidents and keep the response picture current.</p></div>
      <div className="operator-health"><span className={`health-dot${stale ? ' stale' : ''}`} />{stale ? 'Data is stale' : 'Live'} <span className="small-text">· refreshed {new Date().toLocaleTimeString()}</span></div>
    </section>
    {error && <div className="error operator-alert"><strong>Feed unavailable.</strong> {error} <button type="button" onClick={() => void load()}>Retry</button></div>}
    <div className="operator-kpis">
      <article><span>Calls in window</span><strong>{serverMode ? total : stats.incidents}</strong><small>{range === '24h' ? 'last 24 hours' : range === '7d' ? 'last 7 days' : 'all available'}</small></article>
      <article><span>Distinct incidents</span><strong>{stats.incidents}</strong><small>deduplicated view</small></article>
      <article><span>Stations active</span><strong>{stats.stations}</strong><small>reporting in window</small></article>
      <article><span>Unit assignments</span><strong>{stats.units}</strong><small>across visible calls</small></article>
    </div>
    <section className="operator-grid">
      <div className="operator-chart card"><div className="section-heading"><div><p className="eyebrow">Volume</p><h3>Calls by type</h3></div><span className="small-text">{serverMode ? 'server window' : 'local fallback'}</span></div><div className="bar-chart">{stats.byType.length ? stats.byType.map(([label, value]) => <div className="bar-row" key={label}><span title={label}>{label}</span><div><i style={{ width: `${Math.max(8, (value / stats.byType[0][1]) * 100)}%` }} /></div><strong>{value}</strong></div>) : <p className="empty-state">No calls in this window.</p>}</div></div>
      <div className="operator-chart card"><div className="section-heading"><div><p className="eyebrow">Watchlist</p><h3>Queue status</h3></div></div><div className="watchlist"><div><span className="health-dot" /> Ingestion online <strong>Healthy</strong></div><div><span className="health-dot" /> Poll interval <strong>15 sec</strong></div><div><span className="health-dot blue" /> Render budget <strong>25 rows</strong></div></div></div>
    </section>
    <section className="calls-panel card"><div className="section-heading"><div><p className="eyebrow">Incident queue</p><h3>Calls</h3></div><span className="count-badge">{total || visible.length} total</span></div>
      <div className="call-filters"><label className="search-field"><span>⌕</span><input value={search} onChange={(event) => changeFilter(setSearch, event.target.value)} placeholder="Search incident, location, station…" aria-label="Search calls" /></label><select value={range} onChange={(event) => changeFilter((value) => setRange(value as Range), event.target.value)} aria-label="Time range"><option value="24h">Last 24 hours</option><option value="7d">Last 7 days</option><option value="all">All available</option></select><select value={station} onChange={(event) => changeFilter(setStation, event.target.value)} aria-label="Station"><option value="all">All stations</option>{facets.stations.map((value) => <option key={value}>{value}</option>)}</select><select value={callType} onChange={(event) => changeFilter(setCallType, event.target.value)} aria-label="Call type"><option value="all">All call types</option>{facets.types.map((value) => <option key={value}>{value}</option>)}</select><button type="button" className="secondary-button" onClick={() => void load()}>Refresh</button></div>
      {loading && calls.length === 0 ? <div className="skeleton-list" aria-label="Loading calls"><i /><i /><i /></div> : <div className="table-container operator-table-wrap"><table className="operator-table"><thead><tr><th>Time</th><th>Incident</th><th>Call type</th><th>Location</th><th>Station</th><th>Units</th><th>Node</th></tr></thead><tbody>{visible.length ? visible.map((call) => <tr key={call.id} onClick={() => setSelected(call)} tabIndex={0} onKeyDown={(event) => { if (event.key === 'Enter') setSelected(call); }}><td className="time-text">{formatTime(call.timestamp)}</td><td><strong className="incident-text">#{call.incident_number}</strong></td><td><span className="call-status">{call.call_type}</span></td><td><strong>{call.address}</strong>{call.intersection && <small>{call.intersection}</small>}</td><td><span className="station-badge">{call.station}</span></td><td>{vehicles(call.vehicles).slice(0, 3).join(', ') || '—'}</td><td className="client-text">{call.client_id}</td></tr>) : <tr><td colSpan={7} className="empty-table">{error ? 'Unable to display calls.' : 'No calls match these filters.'}</td></tr>}</tbody></table></div>}
       <div className="pagination"><span>Showing {visible.length ? ((page - 1) * PAGE_SIZE) + 1 : 0}–{Math.min((page - 1) * PAGE_SIZE + visible.length, (serverMode ? total || page * PAGE_SIZE : calls.length))} {serverMode && !total ? '' : `of ${serverMode ? total : calls.length}`}</span><div><button type="button" className="secondary-button" disabled={page === 1} onClick={() => setPage((value) => value - 1)}>Previous</button><span>Page {page}{serverMode && !total ? '' : ` / ${pages}`}</span><button type="button" className="secondary-button" disabled={serverMode ? !cursors[page + 1] : page >= pages} onClick={() => setPage((value) => value + 1)}>Next</button></div></div>
    </section>
    {selected && <aside className="detail-drawer" role="dialog" aria-label="Call details"><div className="drawer-header"><div><p className="eyebrow">Incident detail</p><h3>#{selected.incident_number}</h3></div><button type="button" className="close-btn" onClick={() => setSelected(null)} aria-label="Close details">×</button></div><div className="drawer-body"><span className="call-status">{selected.call_type}</span><dl><div><dt>Reported</dt><dd>{new Date(selected.timestamp).toLocaleString()}</dd></div><div><dt>Location</dt><dd>{selected.address}{selected.intersection && ` · ${selected.intersection}`}</dd></div><div><dt>Station</dt><dd>{selected.station}</dd></div><div><dt>Units</dt><dd>{vehicles(selected.vehicles).join(', ') || 'No units listed'}</dd></div><div><dt>Node</dt><dd>{selected.client_id}</dd></div></dl>{selected.text && <div className="raw-message"><dt>Source message</dt><p>{selected.text}</p></div>}</div><div className="drawer-footer"><button type="button" className="secondary-button" onClick={() => setSelected(null)}>Close</button>{user?.role === 'admin' && <span className="small-text">Admin actions remain available in the legacy dashboard.</span>}</div></aside>}
  </div>;
}
