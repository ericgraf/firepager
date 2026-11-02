import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
  type ChartOptions,
} from 'chart.js';
import { Bar, Line } from 'react-chartjs-2';
import { useAuth } from '../context/AuthContext';
import { DashboardCallMap, type DashboardCall } from './DashboardCallMap';
import './components.css';

ChartJS.register(CategoryScale, LinearScale, BarElement, LineElement, PointElement, Tooltip, Legend, Filler);

type TimeRange = '24h' | '7d' | 'all';
interface CallRecord extends DashboardCall {
  type: string;
  vehicles: string[];
  client_id: string;
  source?: string;
}
interface MessagePage {
  messages?: Record<string, unknown>[];
  items?: Record<string, unknown>[];
  next_cursor?: string;
  has_more?: boolean;
}
interface Count { label: string; value: number }

const PAGE_LIMIT = 500;

function display(value: unknown, fallback = '—') {
  return value === null || value === undefined || String(value).trim() === '' ? fallback : String(value);
}

function parseVehicles(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).map((item) => item.trim()).filter(Boolean);
  return String(value || '').split(/[,/;]+/).map((item) => item.trim()).filter(Boolean);
}

function normalizeCall(raw: Record<string, unknown>, index: number): CallRecord {
  return {
    id: display(raw.id ?? raw.message_id, `call-${index}`),
    timestamp: display(raw.timestamp ?? raw.created_at, ''),
    type: display(raw.type, 'CALL'),
    call_type: display(raw.call_type, 'Unclassified'),
    incident_number: display(raw.incident_number, 'N/A'),
    address: display(raw.address, ''),
    intersection: display(raw.intersection, ''),
    latitude: Number(raw.latitude),
    longitude: Number(raw.longitude),
    stations: Array.isArray(raw.stations) ? raw.stations.map(String).map((station) => station.trim()).filter(Boolean) : [],
    vehicles: parseVehicles(raw.vehicles),
    client_id: display(raw.client_id, 'Unknown Receiver'),
    source: display(raw.source, ''),
  };
}

function countBy(values: string[]): Count[] {
  const counts = new Map<string, number>();
  for (const value of values) counts.set(value, (counts.get(value) || 0) + 1);
  return [...counts].map(([label, value]) => ({ label, value }))
    .sort((a, b) => b.value - a.value || a.label.localeCompare(b.label));
}

function isMapped(call: CallRecord) {
  return Number.isFinite(call.latitude) && Number.isFinite(call.longitude)
    && (call.latitude !== 0 || call.longitude !== 0);
}

function trendSeries(calls: CallRecord[], range: TimeRange) {
  const now = new Date();
  const start = new Date(now);
  if (range === '24h') start.setHours(now.getHours() - 23, 0, 0, 0);
  else if (range === '7d') start.setDate(now.getDate() - 6);
  else {
    const times = calls.map((call) => Date.parse(call.timestamp)).filter(Number.isFinite);
    if (times.length) start.setTime(times.reduce((minimum, value) => Math.min(minimum, value), Infinity));
    else start.setDate(now.getDate() - 6);
  }

  const span = now.getTime() - start.getTime();
  const bucket: 'hour' | 'day' | 'month' = range === '24h' ? 'hour' : range === '7d' ? 'day' : span > 62 * 86_400_000 ? 'month' : 'day';
  const keyFor = (date: Date) => {
    const key = new Date(date);
    if (bucket === 'hour') key.setMinutes(0, 0, 0);
    else if (bucket === 'day') key.setHours(0, 0, 0, 0);
    else {
      key.setDate(1);
      key.setHours(0, 0, 0, 0);
    }
    return key.getTime();
  };
  const counts = new Map<number, number>();
  calls.forEach((call) => {
    const timestamp = Date.parse(call.timestamp);
    if (!Number.isFinite(timestamp)) return;
    const key = keyFor(new Date(timestamp));
    counts.set(key, (counts.get(key) || 0) + 1);
  });

  const keys = new Set(counts.keys());
  if (range !== 'all') {
    const cursor = new Date(keyFor(start));
    const end = keyFor(now);
    while (cursor.getTime() <= end) {
      keys.add(cursor.getTime());
      if (bucket === 'hour') cursor.setHours(cursor.getHours() + 1);
      else cursor.setDate(cursor.getDate() + 1);
    }
  }

  const sorted = [...keys].sort((a, b) => a - b);
  const labels = sorted.map((key) => new Intl.DateTimeFormat(undefined,
    bucket === 'hour' ? { hour: 'numeric', day: 'numeric', month: 'short' }
      : bucket === 'month' ? { month: 'short', year: '2-digit' }
        : { month: 'short', day: 'numeric' }
  ).format(new Date(key)));
  return { labels, values: sorted.map((key) => counts.get(key) || 0) };
}

async function loadCalls(token: string, range: TimeRange, signal: AbortSignal) {
  const calls: CallRecord[] = [];
  let cursor = '';
  const seenCursors = new Set<string>();
  const from = new Date(Date.now() - (range === '24h' ? 1 : 7) * 86_400_000).toISOString();

  let hasMore = true;
  while (hasMore) {
    const params = new URLSearchParams({ limit: String(PAGE_LIMIT), type: 'CALL' });
    if (range !== 'all') params.set('from', from);
    if (cursor) params.set('cursor', cursor);
    const response = await fetch(`/api/messages?${params.toString()}`, {
      headers: { Authorization: `Bearer ${token}` },
      signal,
    });
    if (!response.ok) throw new Error(`Could not load metrics (${response.status})`);
    const payload = await response.json() as MessagePage | Record<string, unknown>[];
    const rows = Array.isArray(payload) ? payload : payload.messages || payload.items || [];
    calls.push(...rows.map((row, index) => normalizeCall(row, calls.length + index)));
    if (Array.isArray(payload)) {
      hasMore = false;
      break;
    }
    const nextCursor = payload.next_cursor;
    if (!payload.has_more || !nextCursor || seenCursors.has(nextCursor)) {
      hasMore = false;
      break;
    }
    cursor = nextCursor;
    seenCursors.add(cursor);
  }

  return calls;
}

export function Metrics() {
  const { token } = useAuth();
  const [calls, setCalls] = useState<CallRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [timeRange, setTimeRange] = useState<TimeRange>('24h');
  const [stationFilter, setStationFilter] = useState('all');
  const [callTypeFilter, setCallTypeFilter] = useState('all');
  const [selectedVehicles, setSelectedVehicles] = useState<string[]>([]);
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  const refresh = useCallback(() => {
    setLoading(true);
    setError('');
    setRefreshKey((value) => value + 1);
  }, []);
  useEffect(() => {
    if (!token) return;
    const controller = new AbortController();
    loadCalls(token, timeRange, controller.signal)
      .then((result) => {
        setCalls(result);
        setUpdatedAt(new Date());
      })
      .catch((reason: unknown) => {
        if (controller.signal.aborted) return;
        setError(reason instanceof Error ? reason.message : 'Could not load metrics');
      })
      .finally(() => { if (!controller.signal.aborted) setLoading(false); });
    return () => controller.abort();
  }, [token, timeRange, refreshKey]);

  const stations = useMemo(() => countBy(calls.flatMap((call) => call.stations)), [calls]);
  const callTypes = useMemo(() => countBy(calls.map((call) => call.call_type)), [calls]);
  const allVehicles = useMemo(() => countBy(calls.flatMap((call) => call.vehicles.length ? call.vehicles : ['Unknown Vehicle'])), [calls]);
  const filteredCalls = useMemo(() => calls.filter((call) =>
    (stationFilter === 'all' || call.stations.includes(stationFilter))
    && (callTypeFilter === 'all' || call.call_type === callTypeFilter)
    && (!selectedVehicles.length || selectedVehicles.some((vehicle) => call.vehicles.includes(vehicle) || (!call.vehicles.length && vehicle === 'Unknown Vehicle')))
  ), [calls, stationFilter, callTypeFilter, selectedVehicles]);

  const counts = useMemo(() => ({
    incidents: new Set(filteredCalls.map((call) => call.incident_number === 'N/A' ? call.id : call.incident_number)).size,
    stations: new Set(filteredCalls.flatMap((call) => call.stations)).size,
    vehicleMentions: filteredCalls.reduce((sum, call) => sum + call.vehicles.length, 0),
    byType: countBy(filteredCalls.map((call) => call.call_type)),
    byStation: countBy(filteredCalls.flatMap((call) => call.stations)).slice(0, 12),
    byVehicle: countBy(filteredCalls.flatMap((call) => call.vehicles.length ? call.vehicles : ['Unknown Vehicle'])).slice(0, 12),
  }), [filteredCalls]);
  const trend = useMemo(() => trendSeries(filteredCalls, timeRange), [filteredCalls, timeRange]);
  const mappedCalls = useMemo(() => filteredCalls.filter(isMapped).map((call) => ({
    ...call,
    vehicles: call.vehicles,
  })), [filteredCalls]);
  const unlocatedCount = filteredCalls.length - mappedCalls.length;

  const trendData = useMemo(() => ({
    labels: trend.labels,
    datasets: [{
      label: 'Calls', data: trend.values, fill: true,
      borderColor: '#2563eb', backgroundColor: 'rgba(37, 99, 235, .14)',
      pointRadius: trend.values.length > 60 ? 0 : 2, pointHoverRadius: 5, tension: .25,
    }],
  }), [trend]);
  const chartOptions = useMemo<ChartOptions<'line'>>(() => ({
    responsive: true, maintainAspectRatio: false,
    interaction: { intersect: false, mode: 'index' },
    plugins: { legend: { display: false }, tooltip: { enabled: true } },
    scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
  }), []);
  const barOptions = useMemo<ChartOptions<'bar'>>(() => ({
    responsive: true, maintainAspectRatio: false,
    plugins: { legend: { display: false }, tooltip: { enabled: true } },
    scales: { y: { beginAtZero: true, ticks: { precision: 0 } } },
  }), []);
  const vehicleOptions = useMemo<ChartOptions<'bar'>>(() => ({
    ...barOptions, indexAxis: 'y',
  }), [barOptions]);
  const byTypeData = useMemo(() => ({
    labels: counts.byType.map((item) => item.label),
    datasets: [{ label: 'Calls', data: counts.byType.map((item) => item.value), backgroundColor: '#2563eb', borderRadius: 5 }],
  }), [counts.byType]);
  const byStationData = useMemo(() => ({
    labels: counts.byStation.map((item) => item.label),
    datasets: [{ label: 'Calls', data: counts.byStation.map((item) => item.value), backgroundColor: '#ef4444', borderRadius: 5 }],
  }), [counts.byStation]);
  const byVehicleData = useMemo(() => ({
    labels: counts.byVehicle.map((item) => item.label),
    datasets: [{ label: 'Unit mentions', data: counts.byVehicle.map((item) => item.value), backgroundColor: '#f97316', borderRadius: 5 }],
  }), [counts.byVehicle]);

  const selectCallType = (_event: unknown, elements: { index: number }[]) => {
    if (elements.length) setCallTypeFilter((current) => current === counts.byType[elements[0].index]?.label ? 'all' : counts.byType[elements[0].index]?.label || 'all');
  };
  const selectStation = (_event: unknown, elements: { index: number }[]) => {
    if (elements.length) setStationFilter((current) => current === counts.byStation[elements[0].index]?.label ? 'all' : counts.byStation[elements[0].index]?.label || 'all');
  };
  const toggleVehicle = (vehicle: string) => setSelectedVehicles((current) =>
    current.includes(vehicle) ? current.filter((item) => item !== vehicle) : [...current, vehicle]
  );

  return <section className="metrics-section metrics-dashboard">
    <header className="metrics-page-header">
      <div><p className="eyebrow">Operations / analytics</p><h1>Metrics</h1><p className="operator-subtitle">Explore call volume, response patterns, and mapped locations.</p></div>
      <div className="metrics-refresh"><span className="small-text">{updatedAt ? `Updated ${updatedAt.toLocaleTimeString()}` : 'Live call data'}</span><button type="button" className="secondary-button" onClick={refresh} disabled={loading}>{loading ? 'Loading…' : 'Refresh'}</button></div>
    </header>

    {error && <div className="error operator-alert"><strong>Metrics unavailable.</strong> {error} <button type="button" onClick={refresh}>Retry</button></div>}

    <section className="card metrics-filters-card">
      <div className="section-heading"><div><p className="eyebrow">Filter the dataset</p><h2>Call selection</h2></div><span className="count-badge">{loading ? 'Loading…' : `${filteredCalls.length.toLocaleString()} calls`}</span></div>
      <div className="metrics-filter-grid">
        <div className="form-group"><label htmlFor="metrics-time-range">Time window</label><select id="metrics-time-range" value={timeRange} onChange={(event) => { setLoading(true); setError(''); setTimeRange(event.target.value as TimeRange); }}><option value="24h">Last 24 hours</option><option value="7d">Last 7 days</option><option value="all">All available</option></select></div>
        <div className="form-group"><label htmlFor="metrics-station">Station</label><select id="metrics-station" value={stationFilter} onChange={(event) => setStationFilter(event.target.value)}><option value="all">All stations</option>{stations.map((item) => <option key={item.label} value={item.label}>{item.label} ({item.value})</option>)}</select></div>
        <div className="form-group"><label htmlFor="metrics-call-type">Call type</label><select id="metrics-call-type" value={callTypeFilter} onChange={(event) => setCallTypeFilter(event.target.value)}><option value="all">All call types</option>{callTypes.map((item) => <option key={item.label} value={item.label}>{item.label} ({item.value})</option>)}</select></div>
      </div>
      <div className="metrics-unit-browser">
        <div className="section-heading"><div><p className="eyebrow">Optional filter</p><h3>Units</h3></div><span className="count-badge secondary">{allVehicles.length} tracked</span></div>
        <div className="unit-grid" role="group" aria-label="Filter by vehicle unit">
          {allVehicles.map((vehicle) => <button type="button" key={vehicle.label} className={`unit-chip${selectedVehicles.includes(vehicle.label) ? ' active' : ''}`} aria-pressed={selectedVehicles.includes(vehicle.label)} onClick={() => toggleVehicle(vehicle.label)}><span>{vehicle.label}</span><span className="unit-chip-count">{vehicle.value}</span></button>)}
          {selectedVehicles.length > 0 && <button type="button" className="unit-clear-button" onClick={() => setSelectedVehicles([])}>Clear selected units</button>}
        </div>
      </div>
      {(stationFilter !== 'all' || callTypeFilter !== 'all' || selectedVehicles.length > 0) && <div className="metrics-filter-summary"><span>Chart clicks and filters are interactive. Click a selected filter again to clear it.</span><button type="button" className="unit-clear-button" onClick={() => { setStationFilter('all'); setCallTypeFilter('all'); setSelectedVehicles([]); }}>Clear filters</button></div>}
    </section>

    <div className="metrics-container metrics-kpis">
      <article className="metric-card"><h3>Calls in selection</h3><p className="metric-value">{filteredCalls.length.toLocaleString()}</p><p className="small-text">Matching the current filters</p></article>
      <article className="metric-card"><h3>Distinct incidents</h3><p className="metric-value">{counts.incidents.toLocaleString()}</p><p className="small-text">Unique incident numbers</p></article>
      <article className="metric-card"><h3>Stations represented</h3><p className="metric-value">{counts.stations.toLocaleString()}</p><p className="small-text">In the current selection</p></article>
      <article className="metric-card"><h3>Vehicle mentions</h3><p className="metric-value">{counts.vehicleMentions.toLocaleString()}</p><p className="small-text">Assignments across selected calls</p></article>
    </div>

    <div className="metrics-chart-grid">
      <section className="metric-card metrics-chart-card metrics-chart-wide"><div className="section-heading"><div><p className="eyebrow">Trend</p><h2>Calls over time</h2></div><span className="small-text">Hover for exact counts</span></div><div className="metrics-chart-canvas"><Line data={trendData} options={chartOptions} /></div></section>
      <section className="metric-card metrics-chart-card"><div className="section-heading"><div><p className="eyebrow">Distribution</p><h2>Calls by type</h2></div><span className="small-text">Select a bar to filter</span></div><div className="metrics-chart-canvas"><Bar data={byTypeData} options={{ ...barOptions, onClick: selectCallType }} /></div></section>
      <section className="metric-card metrics-chart-card"><div className="section-heading"><div><p className="eyebrow">Distribution</p><h2>Calls by station</h2></div><span className="small-text">Top 12 · select a bar to filter</span></div><div className="metrics-chart-canvas"><Bar data={byStationData} options={{ ...barOptions, onClick: selectStation }} /></div></section>
      <section className="metric-card metrics-chart-card metrics-chart-wide"><div className="section-heading"><div><p className="eyebrow">Resources</p><h2>Unit mentions</h2></div><span className="small-text">Top 12 units</span></div><div className="metrics-chart-canvas metrics-chart-horizontal"><Bar data={byVehicleData} options={vehicleOptions} /></div></section>
    </div>

    <section className="card metrics-map-card">
      <div className="section-heading"><div><p className="eyebrow">Geographic distribution</p><h2>Selected call locations</h2><p className="operator-subtitle">Every selected call with valid coordinates is plotted together; dense areas are clustered for readability.</p></div><span className="count-badge">{mappedCalls.length.toLocaleString()} mapped</span></div>
      <div className="metrics-map-summary"><span>{filteredCalls.length.toLocaleString()} selected calls</span><span>{unlocatedCount.toLocaleString()} without coordinates</span></div>
      <DashboardCallMap calls={mappedCalls} />
      {unlocatedCount > 0 && <p className="call-location-data-note">{unlocatedCount.toLocaleString()} selected calls are not shown because their latitude/longitude is missing or invalid. They will appear here when coordinates are available.</p>}
    </section>
  </section>;
}
