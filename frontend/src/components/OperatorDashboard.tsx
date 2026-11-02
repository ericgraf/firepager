import { useCallback, useEffect, useMemo, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { AddressLink } from './AddressLink';
import { CallMap } from './CallMap';
import { useIsMobile } from '../hooks/useIsMobile';
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
  dv: string;
  sec: string;
  gd: string;
  alias: string;
  stations: string[];
  latitude?: number;
  longitude?: number;
  client_id: string;
  source?: string;
  text?: string;
  raw?: unknown;
};

type Range = '24h' | '7d' | 'all';
type Result = { items: Call[]; total: number; nextCursor?: string; hasMore: boolean; serverFiltered: boolean };
type PagerMessage = {
  id: string;
  timestamp: string;
  type: string;
  text: string;
  raw_text: string;
  client_id: string;
};

const PAGE_SIZE = 25;
const POLL_MS = 15_000;

function vehicles(value: Call['vehicles']) {
  if (Array.isArray(value)) return value.map(String).map((v) => v.trim()).filter(Boolean);
  return String(value || '').split(/[,/;]+/).map((v) => v.trim()).filter(Boolean);
}

function text(value: unknown, fallback = '—') {
  return value === null || value === undefined || String(value).trim() === '' ? fallback : String(value);
}

function stationCodes(value: unknown) {
  const source = Array.isArray(value) ? value : [];
  return [...new Set(source.map(String).map((station) => station.trim()).filter(Boolean))];
}

function normalize(raw: Record<string, unknown>, index: number): Call {
  return {
    id: text(raw.id ?? raw.message_id, `call-${index}`),
    timestamp: text(raw.timestamp ?? raw.created_at ?? raw.time, new Date().toISOString()),
    type: text(raw.type, 'CALL'), call_type: text(raw.call_type ?? raw.callType, 'Unclassified'),
    incident_number: text(raw.incident_number ?? raw.incidentNumber, 'N/A'),
    vehicles: raw.vehicles as Call['vehicles'], address: text(raw.address), intersection: text(raw.intersection, ''),
    dv: text(raw.dv, ''), sec: text(raw.sec, ''), gd: text(raw.gd, ''), alias: text(raw.alias, ''),
    latitude: Number(raw.latitude) || undefined, longitude: Number(raw.longitude) || undefined,
    client_id: text(raw.client_id ?? raw.clientId), stations: stationCodes(raw.stations),
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

function dispatchSummary(call: Call) {
  return [call.dv && `Dv ${call.dv}`, call.sec && `Sec ${call.sec}`, call.gd && `Gd ${call.gd}`, call.alias && `Alias ${call.alias}`]
    .filter(Boolean).join(' · ');
}

function isRecent(timestamp: string, range: Range) {
  if (range === 'all') return true;
  const age = Date.now() - new Date(timestamp).getTime();
  return age >= 0 && age <= (range === '24h' ? 1 : 7) * 86_400_000;
}

function CallDetailPage({ call, onBack }: { call: Call; onBack: () => void }) {
  const assignedUnits = vehicles(call.vehicles);
  const assignedStations = call.stations;
  const hasCoordinates = Number.isFinite(call.latitude) && Number.isFinite(call.longitude)
    && call.latitude !== 0 && call.longitude !== 0;
  const locationQuery = [call.address, call.intersection].filter(Boolean).join(', ');
  const directions = new URL('https://www.google.com/maps/dir/');
  directions.searchParams.set('api', '1');
  directions.searchParams.set('destination', hasCoordinates
    ? `${call.latitude},${call.longitude}`
    : locationQuery);
  const mapPreview = new URL('https://maps.google.com/maps');
  mapPreview.searchParams.set('q', hasCoordinates ? `${call.latitude},${call.longitude}` : locationQuery);
  mapPreview.searchParams.set('output', 'embed');

  return <main className="call-detail-page">
    <div className="call-detail-topbar">
      <button type="button" className="secondary-button" onClick={onBack}>← Back to calls</button>
      <span className="small-text">Incident detail</span>
    </div>
    <header className="call-detail-hero">
      <div>
        <p className="eyebrow">{assignedStations.length ? `${assignedStations.join(', ')} · ` : ''}{call.call_type}</p>
        <h2>#{call.incident_number}</h2>
        <p className="call-detail-reported">Reported {new Date(call.timestamp).toLocaleString()}</p>
      </div>
      <span className="call-status">{call.type}</span>
    </header>

    <div className="call-detail-grid">
      <div className="call-detail-main">
        <section className="call-location-card card">
          <div className="section-heading">
            <div><p className="eyebrow">Incident location</p><h3>Address</h3></div>
          </div>
          <h2 className="call-detail-address"><AddressLink address={call.address} intersection={call.intersection}>{call.address}</AddressLink></h2>
          {call.intersection && <p className="call-detail-intersection">Near <strong>{call.intersection}</strong></p>}
          <div className="call-location-actions">
            <AddressLink className="secondary-button" address={call.address} intersection={call.intersection}>Open in Google Maps ↗</AddressLink>
            {locationQuery && <a className="secondary-button" href={directions.toString()} target="_blank" rel="noopener noreferrer">Directions ↗</a>}
          </div>
          <div className="call-detail-map">
            {hasCoordinates
              ? <CallMap lat={call.latitude!} lng={call.longitude!} address={call.address} callType={call.call_type} incidentNumber={call.incident_number} />
              : locationQuery
                ? <iframe className="call-map-search-preview" src={mapPreview.toString()} title={`Map search for ${locationQuery}`} loading="lazy" referrerPolicy="no-referrer-when-downgrade" />
                : <div className="map-placeholder"><p>No address or verified map coordinates are recorded for this call.</p></div>}
          </div>
          {hasCoordinates
            ? <p className="call-coordinate-note">Verified coordinates recorded with this call: {call.latitude!.toFixed(5)}, {call.longitude!.toFixed(5)}</p>
            : locationQuery && <p className="call-coordinate-note">Address search preview only. FirePager has not verified or saved a map pin for this call.</p>}
        </section>

        <section className="card call-address-context">
          <div className="section-heading"><div><p className="eyebrow">Location context</p><h3>Details associated with this address</h3></div></div>
          <dl className="call-detail-facts">
            {call.intersection && <div><dt>Intersection / cross street</dt><dd>{call.intersection}</dd></div>}
            {call.alias && <div><dt>Location alias</dt><dd>{call.alias}</dd></div>}
            {call.dv && <div><dt>Division</dt><dd>{call.dv}</dd></div>}
            {call.sec && <div><dt>Section</dt><dd>{call.sec}</dd></div>}
            {call.gd && <div><dt>Grid</dt><dd>{call.gd}</dd></div>}
            {hasCoordinates && <div><dt>Latitude</dt><dd>{call.latitude!.toFixed(6)}</dd></div>}
            {hasCoordinates && <div><dt>Longitude</dt><dd>{call.longitude!.toFixed(6)}</dd></div>}
          </dl>
          <p className="call-location-data-note">These are the location details included with the dispatch. Property, parcel, photo, listing, and site-plan records are not connected yet.</p>
        </section>

        {call.text && <section className="card call-dispatch-card">
          <div className="section-heading"><div><p className="eyebrow">Source</p><h3>Dispatch message</h3></div></div>
          <pre className="call-dispatch-text">{call.text}</pre>
        </section>}
      </div>

      <aside className="call-detail-sidebar">
        <section className="card">
          <div className="section-heading"><div><p className="eyebrow">Response</p><h3>Assigned resources</h3></div></div>
          <dl className="call-detail-facts">
            <div><dt>Stations</dt><dd className="station-badges">{assignedStations.length ? assignedStations.map((station) => <span className="station-badge" key={station}>ST {station}</span>) : '—'}</dd></div>
            <div><dt>Units</dt><dd>{assignedUnits.length ? assignedUnits.join(', ') : 'No units listed'}</dd></div>
            <div><dt>Receiving node</dt><dd>{text(call.client_id)}</dd></div>
          </dl>
        </section>
        <section className="card">
          <div className="section-heading"><div><p className="eyebrow">Call record</p><h3>Incident details</h3></div></div>
          <dl className="call-detail-facts">
            <div><dt>Call type</dt><dd>{call.call_type}</dd></div>
            <div><dt>Incident number</dt><dd>#{call.incident_number}</dd></div>
            <div><dt>Reported</dt><dd>{new Date(call.timestamp).toLocaleString()}</dd></div>
            <div><dt>Message type</dt><dd>{text(call.type)}</dd></div>
            {call.source && <div><dt>Source</dt><dd>{call.source}</dd></div>}
          </dl>
        </section>
      </aside>
    </div>
  </main>;
}

export function OperatorDashboard() {
  const { token, user } = useAuth();
  const isMobile = useIsMobile();
  const [calls, setCalls] = useState<Call[]>([]);
  const [pagerMessages, setPagerMessages] = useState<PagerMessage[]>([]);
  const [messagesError, setMessagesError] = useState('');
  const [deletingMessageId, setDeletingMessageId] = useState<string | null>(null);
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

  const loadPagerMessages = useCallback(async () => {
    if (!token) return;
    try {
      const response = await fetch('/api/messages?limit=25', { headers: { Authorization: `Bearer ${token}` } });
      if (!response.ok) throw new Error(`Unable to load pager messages (${response.status})`);
      const result = await response.json() as { messages?: PagerMessage[] } | PagerMessage[];
      setPagerMessages(Array.isArray(result) ? result : result.messages || []);
      setMessagesError('');
    } catch (err) {
      setMessagesError(err instanceof Error ? err.message : 'Unable to load pager messages');
    }
  }, [token]);

  const deletePagerMessage = async (message: PagerMessage) => {
    if (user?.role !== 'admin' || deletingMessageId) return;
    if (!window.confirm(`Delete this pager message from the database? This cannot be undone.`)) return;
    setDeletingMessageId(message.id);
    try {
      const response = await fetch(`/api/admin/messages/${encodeURIComponent(message.id)}`, {
        method: 'DELETE',
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!response.ok) throw new Error(`Unable to delete pager message (${response.status})`);
      setPagerMessages((current) => current.filter((item) => item.id !== message.id));
      setMessagesError('');
    } catch (err) {
      setMessagesError(err instanceof Error ? err.message : 'Unable to delete pager message');
    } finally {
      setDeletingMessageId(null);
    }
  };

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
    void loadPagerMessages();
    const interval = window.setInterval(() => void loadPagerMessages(), POLL_MS);
    return () => window.clearInterval(interval);
  }, [loadPagerMessages]);
  useEffect(() => {
    const interval = window.setInterval(() => void load(true), POLL_MS);
    return () => window.clearInterval(interval);
  }, [load]);

  const visible = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (serverMode) return calls;
    return calls.filter((call) => isRecent(call.timestamp, range)
      && (station === 'all' || call.stations.includes(station))
      && (callType === 'all' || call.call_type === callType)
      && (!query || [call.incident_number, call.address, call.call_type, call.stations.join(' '), call.client_id].join(' ').toLowerCase().includes(query))
    ).slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
  }, [calls, serverMode, range, station, callType, search, page]);

  const facets = useMemo(() => ({
    stations: [...new Set(calls.flatMap((call) => call.stations))].slice(0, 100),
    types: [...new Set(calls.map((call) => call.call_type).filter((value) => value !== '—'))].slice(0, 100)
  }), [calls]);
  const stats = useMemo(() => {
    const values = serverMode ? calls : calls.filter((call) => isRecent(call.timestamp, range));
    const byType = new Map<string, number>();
    values.forEach((call) => byType.set(call.call_type, (byType.get(call.call_type) || 0) + 1));
    const summaryTypes = metricSummary?.by_type;
    if (Array.isArray(summaryTypes)) summaryTypes.forEach((item) => { const entry = item as Record<string, unknown>; const label = text(entry.label ?? entry.name, 'Unclassified'); const value = Number(entry.value ?? entry.count); if (Number.isFinite(value)) byType.set(label, value); });
    return { incidents: Number(metricSummary?.distinct_incidents) || new Set(values.map((call) => call.incident_number)).size, stations: Number(metricSummary?.active_stations) || new Set(values.flatMap((call) => call.stations)).size, units: Number(metricSummary?.vehicle_mentions) || values.reduce((sum, call) => sum + vehicles(call.vehicles).length, 0), byType: [...byType.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5) };
  }, [calls, serverMode, range, metricSummary]);
  const pages = serverMode ? Math.max(1, page + (calls.length === PAGE_SIZE && cursors[page + 1] ? 1 : 0)) : Math.max(1, Math.ceil(calls.length / PAGE_SIZE));

  const changeFilter = (setter: (value: string) => void, value: string) => { setter(value); setPage(1); setCursors({}); };

  if (selected) return <div className="operator-shell"><CallDetailPage call={selected} onBack={() => setSelected(null)} /></div>;

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
      {loading && calls.length === 0 ? <div className="skeleton-list" aria-label="Loading calls"><i /><i /><i /></div> : isMobile ? (
        <ul className="call-cards" aria-label="Calls">
          {visible.length ? visible.map((call) => (
            <li key={call.id}>
              <button type="button" className="call-card" onClick={() => setSelected(call)}>
                <span className="call-card-head">
                  <span className="call-status">{call.call_type}</span>
                  <strong className="incident-text">#{call.incident_number}</strong>
                  <span className="time-text">{formatTime(call.timestamp)}</span>
                </span>
                <span className="call-card-loc"><strong><AddressLink address={call.address} intersection={call.intersection} /></strong>{call.intersection && <small>{call.intersection}</small>}{dispatchSummary(call) && <small>{dispatchSummary(call)}</small>}</span>
                <span className="call-card-foot"><span className="station-badges">{call.stations.length ? call.stations.map((code) => <span className="station-badge" key={code}>ST {code}</span>) : '—'}</span><span className="client-text">{vehicles(call.vehicles).slice(0, 3).join(', ') || 'No units'}</span></span>
              </button>
            </li>
          )) : <li className="empty-state">{error ? 'Unable to display calls.' : 'No calls match these filters.'}</li>}
        </ul>
       ) : <div className="table-container operator-table-wrap"><table className="operator-table"><thead><tr><th>Time</th><th>Incident</th><th>Call type</th><th>Location</th><th>Stations</th><th>Units</th><th>Node</th></tr></thead><tbody>{visible.length ? visible.map((call) => <tr key={call.id} onClick={() => setSelected(call)} tabIndex={0} onKeyDown={(event) => { if (event.key === 'Enter') setSelected(call); }}><td className="time-text">{formatTime(call.timestamp)}</td><td><strong className="incident-text">#{call.incident_number}</strong></td><td><span className="call-status">{call.call_type}</span></td><td><strong><AddressLink address={call.address} intersection={call.intersection} /></strong>{call.intersection && <small>{call.intersection}</small>}{dispatchSummary(call) && <small>{dispatchSummary(call)}</small>}</td><td><span className="station-badges">{call.stations.length ? call.stations.map((code) => <span className="station-badge" key={code}>ST {code}</span>) : '—'}</span></td><td>{vehicles(call.vehicles).slice(0, 3).join(', ') || '—'}</td><td className="client-text">{call.client_id}</td></tr>) : <tr><td colSpan={7} className="empty-table">{error ? 'Unable to display calls.' : 'No calls match these filters.'}</td></tr>}</tbody></table></div>}
       <div className="pagination"><span>Showing {visible.length ? ((page - 1) * PAGE_SIZE) + 1 : 0}–{Math.min((page - 1) * PAGE_SIZE + visible.length, (serverMode ? total || page * PAGE_SIZE : calls.length))} {serverMode && !total ? '' : `of ${serverMode ? total : calls.length}`}</span><div><button type="button" className="secondary-button" disabled={page === 1} onClick={() => setPage((value) => value - 1)}>Previous</button><span>Page {page}{serverMode && !total ? '' : ` / ${pages}`}</span><button type="button" className="secondary-button" disabled={serverMode ? !cursors[page + 1] : page >= pages} onClick={() => setPage((value) => value + 1)}>Next</button></div></div>
    </section>
    <section className="calls-panel card"><div className="section-heading"><div><p className="eyebrow">Decoded receiver traffic</p><h3>Pager messages</h3><p className="operator-subtitle">Latest decoded pages received by FirePager.</p></div><span className="count-badge">{pagerMessages.length}</span></div>
      {messagesError && <div className="error operator-alert">{messagesError} <button type="button" onClick={() => void loadPagerMessages()}>Retry</button></div>}
      <div className="table-container operator-table-wrap"><table className="operator-table"><thead><tr><th>Time</th><th>Type</th><th>Message</th><th>Receiver</th>{user?.role === 'admin' && <th>Actions</th>}</tr></thead><tbody>
        {pagerMessages.length ? pagerMessages.map((message) => <tr key={message.id}><td className="time-text">{formatTime(message.timestamp)}</td><td><span className="call-status">{message.type || 'MESSAGE'}</span></td><td><div>{message.text || message.raw_text || '—'}</div>{message.raw_text && <details className="raw-page-details"><summary>View decoded page</summary><pre className="content-text">{message.raw_text}</pre></details>}</td><td className="client-text">{message.client_id || '—'}</td>{user?.role === 'admin' && <td><button type="button" className="danger-btn danger-btn-small" onClick={() => void deletePagerMessage(message)} disabled={deletingMessageId !== null}>{deletingMessageId === message.id ? 'Deleting...' : 'Delete'}</button></td>}</tr>) : <tr><td colSpan={user?.role === 'admin' ? 5 : 4} className="empty-table">{messagesError ? 'Unable to display pager messages.' : 'No decoded pager messages received yet.'}</td></tr>}
      </tbody></table></div>
    </section>
  </div>;
}
