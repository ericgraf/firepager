import { useEffect, useMemo, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { BarChart } from './BarChart';
import './components.css';

interface Message {
  id: string;
  timestamp: string;
  type: string;
  call_type: string;
  incident_number: string;
  vehicles: string[];
  client_id: string;
  station: string;
}

interface Stat {
  label: string;
  value: number;
}

type TimeRange = 'all' | '24h' | '7d';

function normalizeLabel(value: string | null | undefined, fallback: string) {
  const label = value?.trim();
  return label ? label : fallback;
}

function buildStats(values: string[]): Stat[] {
  const counts = new Map<string, number>();

  values.forEach((value) => {
    counts.set(value, (counts.get(value) || 0) + 1);
  });

  return [...counts.entries()]
    .map(([label, value]) => ({ label, value }))
    .sort((a, b) => b.value - a.value || a.label.localeCompare(b.label));
}

function extractVehicles(rawVehicles: string[] | string | null | undefined) {
  if (Array.isArray(rawVehicles)) {
    return rawVehicles.map((vehicle) => vehicle.trim()).filter(Boolean);
  }

  return (rawVehicles || '')
    .split(/[,/;]+/)
    .map((vehicle) => vehicle.trim())
    .filter(Boolean);
}

interface MetricsProps {
  selectedVehicles: string[];
  onToggleVehicle: (vehicle: string | null) => void;
}

export function Metrics({ selectedVehicles, onToggleVehicle }: MetricsProps) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [timeRange, setTimeRange] = useState<TimeRange>('24h');
  const [stationFilter, setStationFilter] = useState('all');
  const [callTypeFilter, setCallTypeFilter] = useState('all');
  const { token } = useAuth();

  const fetchMessages = async () => {
    try {
      const res = await fetch('/api/messages', {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      if (!res.ok) throw new Error('Failed to fetch metrics');
      const data = await res.json();
      setMessages(data || []);
      setError('');
    } catch (err: unknown) {
      if (err instanceof Error) setError(err.message);
      else setError(String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!token) return;
    fetchMessages();
    const interval = setInterval(fetchMessages, 10000);
    return () => clearInterval(interval);
  }, [token]);

  const calls = useMemo(
    () => messages.filter((message) => message.type === 'CALL'),
    [messages]
  );

  const stations = useMemo(
    () => buildStats(calls.map((call) => normalizeLabel(call.station, 'Unknown Station'))),
    [calls]
  );

  const callTypes = useMemo(
    () => buildStats(calls.map((call) => normalizeLabel(call.call_type, 'Unclassified'))),
    [calls]
  );

  const filteredCalls = useMemo(() => {
    const now = Date.now();

    return calls.filter((call) => {
      const callTime = new Date(call.timestamp).getTime();
      const matchesTime =
        timeRange === 'all' ||
        (timeRange === '24h' && now - callTime <= 24 * 60 * 60 * 1000) ||
        (timeRange === '7d' && now - callTime <= 7 * 24 * 60 * 60 * 1000);
      const station = normalizeLabel(call.station, 'Unknown Station');
      const callType = normalizeLabel(call.call_type, 'Unclassified');
      const vehicles = extractVehicles(call.vehicles);
      const matchesVehicle =
        selectedVehicles.length === 0 ||
        selectedVehicles.some((selected) =>
          vehicles.includes(selected) || (vehicles.length === 0 && selected === 'Unknown Vehicle')
        );

      return matchesTime &&
        (stationFilter === 'all' || station === stationFilter) &&
        (callTypeFilter === 'all' || callType === callTypeFilter) &&
        matchesVehicle;
    });
  }, [calls, timeRange, stationFilter, callTypeFilter, selectedVehicles]);

  const filteredCallTypes = useMemo(
    () => buildStats(filteredCalls.map((call) => normalizeLabel(call.call_type, 'Unclassified'))).slice(0, 8),
    [filteredCalls]
  );

  const filteredStations = useMemo(
    () => buildStats(filteredCalls.map((call) => normalizeLabel(call.station, 'Unknown Station'))).slice(0, 8),
    [filteredCalls]
  );

  const filteredVehicles = useMemo(
    () => buildStats(
      filteredCalls.flatMap((call) => {
        const parsedVehicles = extractVehicles(call.vehicles);
        return parsedVehicles.length > 0 ? parsedVehicles : ['Unknown Vehicle'];
      })
    ).slice(0, 10),
    [filteredCalls]
  );

  const allVehicles = useMemo(
    () => buildStats(
      calls.flatMap((call) => {
        const parsedVehicles = extractVehicles(call.vehicles);
        return parsedVehicles.length > 0 ? parsedVehicles : ['Unknown Vehicle'];
      })
    ),
    [calls]
  );

  const filteredNodes = useMemo(
    () => buildStats(filteredCalls.map((call) => normalizeLabel(call.client_id, 'Unknown Node'))).slice(0, 6),
    [filteredCalls]
  );

  const incidentCount = useMemo(
    () => new Set(filteredCalls.map((call) => normalizeLabel(call.incident_number, call.id))).size,
    [filteredCalls]
  );

  const activeStationCount = useMemo(
    () => new Set(filteredCalls.map((call) => normalizeLabel(call.station, 'Unknown Station'))).size,
    [filteredCalls]
  );

  const deployedVehicleCount = useMemo(
    () => filteredCalls.reduce((count, call) => count + Math.max(extractVehicles(call.vehicles).length, 1), 0),
    [filteredCalls]
  );

  if (loading && messages.length === 0) return <p>Loading metrics...</p>;
  if (error) return <div className="error">{error}</div>;

  return (
    <section className="metrics-section">
      <div className="card metrics-filters-card">
        <div className="card-header">
          <h2>Call Metrics</h2>
          <span className="count-badge">{filteredCalls.length} calls</span>
        </div>

        <div className="metrics-filter-grid">
          <div className="form-group">
            <label htmlFor="metrics-time-range">Time Window</label>
            <select id="metrics-time-range" value={timeRange} onChange={(event) => setTimeRange(event.target.value as TimeRange)}>
              <option value="24h">Last 24 Hours</option>
              <option value="7d">Last 7 Days</option>
              <option value="all">All Available</option>
            </select>
          </div>

          <div className="form-group">
            <label htmlFor="metrics-station">Station</label>
            <select id="metrics-station" value={stationFilter} onChange={(event) => setStationFilter(event.target.value)}>
              <option value="all">All Stations</option>
              {stations.map((station) => (
                <option key={station.label} value={station.label}>{station.label}</option>
              ))}
            </select>
          </div>

          <div className="form-group">
            <label htmlFor="metrics-call-type">Call Type</label>
            <select id="metrics-call-type" value={callTypeFilter} onChange={(event) => setCallTypeFilter(event.target.value)}>
              <option value="all">All Call Types</option>
              {callTypes.map((callType) => (
                <option key={callType.label} value={callType.label}>{callType.label}</option>
              ))}
            </select>
          </div>

          <div className="form-group">
            <label>Selected Units</label>
            <div className="metrics-selected-unit">
              {selectedVehicles.length > 0 ? (
                <>
                  {selectedVehicles.map((v) => (
                    <span key={v} className="unit-pill active" onClick={() => onToggleVehicle(v)} style={{ cursor: 'pointer' }}>
                      {v} &times;
                    </span>
                  ))}
                  <button type="button" className="unit-clear-button" onClick={() => onToggleVehicle(null)}>
                    Clear All
                  </button>
                </>
              ) : (
                <span className="small-text">All units</span>
              )}
            </div>
          </div>
        </div>

        <div className="metrics-unit-browser">
          <div className="card-header">
            <h3>Units</h3>
            <span className="count-badge secondary">{allVehicles.length} tracked</span>
          </div>
          <div className="unit-grid" role="list" aria-label="Vehicle units">
            <button
              type="button"
              className={`unit-chip${selectedVehicles.length === 0 ? ' active' : ''}`}
              onClick={() => onToggleVehicle(null)}
            >
              All Units
            </button>
            {allVehicles.map((vehicle) => (
              <button
                key={vehicle.label}
                type="button"
                className={`unit-chip${selectedVehicles.includes(vehicle.label) ? ' active' : ''}`}
                onClick={() => onToggleVehicle(vehicle.label)}
              >
                <span>{vehicle.label}</span>
                <span className="unit-chip-count">{vehicle.value}</span>
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="metrics-container">
        <div className="metric-card">
          <h3>Filtered Calls</h3>
          <p className="metric-value">{filteredCalls.length}</p>
          <p className="small-text">Calls matching the current filters</p>
        </div>

        <div className="metric-card">
          <h3>Distinct Incidents</h3>
          <p className="metric-value">{incidentCount}</p>
          <p className="small-text">Unique incident numbers in view</p>
        </div>

        <div className="metric-card">
          <h3>Active Stations</h3>
          <p className="metric-value">{activeStationCount}</p>
          <p className="small-text">Stations represented in filtered calls</p>
        </div>

        <div className="metric-card">
          <h3>Vehicle Mentions</h3>
          <p className="metric-value">{deployedVehicleCount}</p>
          <p className="small-text">Vehicle assignments across filtered calls</p>
        </div>

        <div className="metric-card">
          <div className="card-header">
            <h2>Calls By Type</h2>
            <span className="count-badge secondary">{filteredCallTypes.length} groups</span>
          </div>
          <BarChart data={filteredCallTypes} color="#2563eb" />
        </div>

        <div className="metric-card">
          <div className="card-header">
            <h2>Calls By Station</h2>
            <span className="count-badge secondary">{filteredStations.length} stations</span>
          </div>
          <BarChart data={filteredStations} color="#ef4444" />
        </div>

        <div className="metric-card">
          <div className="card-header">
            <h2>Vehicle Counts</h2>
            <span className="count-badge secondary">{filteredVehicles.length} vehicles</span>
          </div>
          <BarChart data={filteredVehicles} color="#f97316" />
        </div>

        <div className="metric-card">
          <div className="card-header">
            <h2>Calls By Node</h2>
            <span className="count-badge secondary">{filteredNodes.length} nodes</span>
          </div>
          <BarChart data={filteredNodes} color="#10b981" />
        </div>
      </div>
    </section>
  );
}
