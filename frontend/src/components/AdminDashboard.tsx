import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { useIsMobile } from '../hooks/useIsMobile';
import { BarChart } from './BarChart';
import './components.css';

interface User {
  id: number;
  email: string;
  name: string;
  role: string;
  invited_by_name?: string;
}

interface Config {
  key: string;
  value: string;
}

interface ConfigMeta {
  label: string;
  description?: string;
  placeholder?: string;
  type?: 'text' | 'password';
}

const CONFIG_META: Record<string, ConfigMeta> = {
  SMS_GATEWAY_ENABLED: {
    label: 'SMS Gateway Enabled',
    description: 'Enable or disable outbound SMS delivery globally.',
    placeholder: 'true or false'
  },
  WHATSAPP_GATEWAY_ENABLED: {
    label: 'WhatsApp Gateway Enabled',
    description: 'Enable or disable shared WhatsApp delivery for opted-in accounts globally.',
    placeholder: 'true or false'
  },
  SMS_PROVIDER: {
    label: 'SMS Provider',
    description: 'Active SMS provider implementation.',
    placeholder: 'twilio'
  },
  TWILIO_ACCOUNT_SID: {
    label: 'Twilio Account SID',
    description: 'Twilio account identifier used for API requests.',
    placeholder: 'ACxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
  },
  TWILIO_AUTH_TOKEN: {
    label: 'Twilio Auth Token',
    description: 'Secret token used to authenticate Twilio API requests.',
    placeholder: 'Twilio auth token',
    type: 'password'
  },
  TWILIO_FROM_NUMBER: {
    label: 'Twilio From Number',
    description: 'Twilio sender number in E.164 format.',
    placeholder: '+15551234567'
  },
  TWILIO_WHATSAPP_FROM_NUMBER: {
    label: 'Twilio WhatsApp From Number',
    description: 'Twilio WhatsApp-enabled sender number. Accepts raw E.164 or whatsapp:+15551234567.',
    placeholder: 'whatsapp:+15551234567'
  },
  GEOCODING_PROVIDER: {
    label: 'Geocoding Provider',
    description: 'Address lookup provider for map coordinates.',
    placeholder: 'nominatim'
  },
  CITY_REGION_CONTEXT: {
    label: 'City Region Context',
    description: 'Context appended to ambiguous call addresses during geocoding.',
    placeholder: 'Ottawa, ON'
  }
};

type WhatsAppPairingStatus = { connection: string; qr?: string };

function WhatsAppPairingPanel({ token }: { token: string | null }) {
  const [status, setStatus] = useState<WhatsAppPairingStatus | null>(null);
  const [requested, setRequested] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    if (!token) return;
    setBusy(true);
    setError('');
    try {
      const response = await fetch('/api/admin/whatsapp/pairing', {
        headers: { Authorization: `Bearer ${token}` }, cache: 'no-store'
      });
      if (!response.ok) throw new Error('WhatsApp pairing is temporarily unavailable. Try again shortly.');
      setStatus(await response.json());
      setRequested(true);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Unable to load WhatsApp pairing.');
    } finally {
      setBusy(false);
    }
  }, [token]);

  useEffect(() => {
    if (!requested || !token || status?.connection === 'open') return;
    let cancelled = false;
    const poll = async () => {
      try {
        const response = await fetch('/api/admin/whatsapp/pairing', {
          headers: { Authorization: `Bearer ${token}` }, cache: 'no-store'
        });
        if (response.ok && !cancelled) setStatus(await response.json());
      } catch { /* Keep polling; manual refresh remains available. */ }
    };
    const timer = window.setInterval(poll, 4000);
    return () => { cancelled = true; window.clearInterval(timer); };
  }, [requested, status?.connection, token]);

  return <section className="admin-section card">
    <div className="card-header"><h2>Shared WhatsApp account</h2></div>
    <div className="whatsapp-pairing">
      <div><h4>Link the organization’s sender account</h4><p className="small-text">This single WhatsApp account sends alerts for all users who have individually verified their phone number, consented, and enabled WhatsApp notifications. Scan with WhatsApp → Linked devices → Link a device.</p></div>
      <button type="button" onClick={() => void refresh()} disabled={busy}>{busy ? 'Loading…' : status?.connection === 'open' ? 'Linked' : status ? 'Refresh pairing QR' : 'Show pairing QR'}</button>
      {error && <p className="error" role="alert">{error}</p>}
      {status && <>
        <p className="small-text" role="status" aria-live="polite">Sender status: {status.connection === 'open' ? 'Linked and connected' : status.connection}</p>
        {status.qr ? <figure className="whatsapp-pairing-qr"><img src={status.qr} alt="Shared WhatsApp sender pairing QR code" /><figcaption>This QR grants access to the shared sender account. Only scan it on the authorized Firepager server.</figcaption></figure> : status.connection !== 'open' && <p className="small-text">No active QR is available yet. Refresh in a moment.</p>}
      </>}
    </div>
  </section>;
}

interface Stat {
  label: string;
  value: number;
}

interface MetricsData {
  total_messages: number;
  by_type: Stat[];
  by_client: Stat[];
  by_station: Stat[];
}

interface ReceiverStatus {
  receiver_id: string;
  host: string;
  device_identifier: string;
  channel_name: string;
  description: string;
  nominal_frequency_hz: number | null;
  configured_gain_db: number | null;
  configured_ppm: number | null;
  status: string;
  role: string;
  frequency_hz: number | null;
  gain_db: number | null;
  ppm: number | null;
  active_detector: boolean;
  decoded_messages_count: number;
  calibrations_accepted_count: number;
  last_calibration_utc: string | null;
  observed_at: string;
}

interface WaterfallRow {
  captured_at: string;
  power_db: number[];
}

interface WaterfallSnapshot {
  receiver_id: string;
  captured_at: string;
  start_hz: number;
  step_hz: number;
  rows: WaterfallRow[];
}

function WaterfallCanvas({ snapshot }: { snapshot: WaterfallSnapshot }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || snapshot.rows.length === 0) return;
    const bins = Math.min(...snapshot.rows.map((row) => row.power_db.length));
    const height = snapshot.rows.length;
    if (bins < 2 || height < 1) return;
    canvas.width = bins;
    canvas.height = height;
    const context = canvas.getContext('2d');
    if (!context) return;
    const values = snapshot.rows.flatMap((row) => row.power_db.slice(0, bins));
    const sorted = [...values].sort((a, b) => a - b);
    const low = sorted[Math.floor(sorted.length * 0.05)] ?? -100;
    const high = sorted[Math.floor(sorted.length * 0.995)] ?? low + 1;
    const range = Math.max(1, high - low);
    const image = context.createImageData(bins, height);
    snapshot.rows.forEach((row, y) => row.power_db.slice(0, bins).forEach((power, x) => {
      const t = Math.max(0, Math.min(1, (power - low) / range));
      const red = Math.round(255 * Math.max(0, Math.min(1, (t - 0.55) * 2.2)));
      const green = Math.round(220 * Math.max(0, Math.min(1, (t - 0.18) * 1.7)));
      const blue = Math.round(255 * Math.max(0, 1 - t * 1.25));
      const offset = (y * bins + x) * 4;
      image.data[offset] = red;
      image.data[offset + 1] = green;
      image.data[offset + 2] = blue;
      image.data[offset + 3] = 255;
    }));
    context.putImageData(image, 0, 0);
  }, [snapshot]);

  const highHz = snapshot.start_hz + snapshot.step_hz * snapshot.rows[0].power_db.length;
  return <div className="receiver-waterfall-plot">
    <canvas ref={canvasRef} aria-label={`Waterfall spectrum for ${snapshot.receiver_id}`} />
    <div className="receiver-waterfall-axis"><span>{(snapshot.start_hz / 1e6).toFixed(3)} MHz</span><span>{((snapshot.start_hz + highHz) / 2e6).toFixed(3)} MHz</span><span>{(highHz / 1e6).toFixed(3)} MHz</span></div>
  </div>;
}

export function AdminDashboard() {
  const isMobile = useIsMobile();
  const [users, setUsers] = useState<User[]>([]);
  const [configs, setConfigs] = useState<Config[]>([]);
  const [metrics, setMetrics] = useState<MetricsData | null>(null);
  const [receivers, setReceivers] = useState<ReceiverStatus[]>([]);
  const [receiverError, setReceiverError] = useState('');
  const [waterfallSnapshots, setWaterfallSnapshots] = useState<WaterfallSnapshot[]>([]);
  const [waterfallError, setWaterfallError] = useState('');
  const [draftConfigs, setDraftConfigs] = useState<Record<string, string>>({});
  const [search, setSearch] = useState('');
  const [statusMessage, setStatusMessage] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const { token } = useAuth();

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      setError('');

      const [usersRes, configsRes, metricsRes] = await Promise.all([
        fetch('/api/admin/users', { headers: { Authorization: `Bearer ${token}` } }),
        fetch('/api/configs', { headers: { Authorization: `Bearer ${token}` } }),
        fetch('/api/metrics', { headers: { Authorization: `Bearer ${token}` } })
      ]);

      if (!usersRes.ok || !configsRes.ok || !metricsRes.ok) {
        throw new Error('Failed to fetch admin data');
      }

      const usersData = await usersRes.json();
      const configsData = await configsRes.json();
      const metricsData = await metricsRes.json();

      setUsers(usersData || []);
      setConfigs(configsData || []);
      setMetrics(metricsData || null);
      setDraftConfigs(
        Object.fromEntries((configsData || []).map((config: Config) => [config.key, config.value]))
      );
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [token]);

  const fetchReceivers = useCallback(async () => {
    if (!token) return;
    try {
      const response = await fetch('/api/admin/receivers', {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!response.ok) throw new Error('Unable to load receiver status');
      setReceivers(await response.json());
      setReceiverError('');
    } catch (err: unknown) {
      setReceiverError(err instanceof Error ? err.message : String(err));
    }
  }, [token]);

  const fetchWaterfalls = useCallback(async () => {
    if (!token) return;
    try {
      const response = await fetch('/api/admin/receivers/host-sdr-0/waterfalls?limit=12', {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!response.ok) throw new Error('Unable to load receiver waterfalls');
      const result = await response.json();
      setWaterfallSnapshots(Array.isArray(result) ? result : result.snapshots || []);
      setWaterfallError('');
    } catch (err: unknown) {
      setWaterfallError(err instanceof Error ? err.message : String(err));
    }
  }, [token]);

  useEffect(() => {
    if (!token) return;
    fetchData();
    fetchReceivers();
    fetchWaterfalls();
    const timer = window.setInterval(fetchReceivers, 15000);
    const waterfallTimer = window.setInterval(fetchWaterfalls, 300000);
    return () => {
      window.clearInterval(timer);
      window.clearInterval(waterfallTimer);
    };
  }, [token, fetchData, fetchReceivers, fetchWaterfalls]);

  const handleRoleChange = async (userId: number, newRole: string) => {
    try {
      setError('');
      setStatusMessage('');

      const res = await fetch(`/api/admin/users/${userId}/role`, {
        method: 'PUT',
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ role: newRole })
      });

      if (!res.ok) throw new Error('Failed to update role');

      setStatusMessage('User role updated.');
      setUsers((current) => current.map((user) => (
        user.id === userId ? { ...user, role: newRole } : user
      )));
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const handleConfigUpdate = async (key: string, value: string) => {
    try {
      setError('');
      setStatusMessage('');

      const res = await fetch('/api/admin/configs', {
        method: 'PUT',
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ key, value })
      });

      if (!res.ok) throw new Error('Failed to update config');

      setConfigs((current) => current.map((config) => (
        config.key === key ? { ...config, value } : config
      )));
      setStatusMessage(`Saved ${key}.`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const filteredUsers = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return users;

    return users.filter((user) =>
      user.name.toLowerCase().includes(query) ||
      user.email.toLowerCase().includes(query) ||
      user.role.toLowerCase().includes(query)
    );
  }, [search, users]);

  const roleData = useMemo(() => ([
    { label: 'Admins', value: users.filter((user) => user.role === 'admin').length },
    { label: 'Users', value: users.filter((user) => user.role === 'user').length }
  ]), [users]);

  const topStations = useMemo(() => {
    return [...(metrics?.by_station || [])]
      .sort((a, b) => b.value - a.value)
      .slice(0, 4);
  }, [metrics]);

  const topClients = useMemo(() => {
    return [...(metrics?.by_client || [])]
      .sort((a, b) => b.value - a.value)
      .slice(0, 4);
  }, [metrics]);

  const orderedConfigs = useMemo(() => {
    const priority = [
      'SMS_GATEWAY_ENABLED',
      'WHATSAPP_GATEWAY_ENABLED',
      'SMS_PROVIDER',
      'TWILIO_ACCOUNT_SID',
      'TWILIO_AUTH_TOKEN',
      'TWILIO_FROM_NUMBER',
      'TWILIO_WHATSAPP_FROM_NUMBER',
      'GEOCODING_PROVIDER',
      'CITY_REGION_CONTEXT'
    ];

    const priorityIndex = new Map(priority.map((key, index) => [key, index]));

    return [...configs].sort((left, right) => {
      const leftIndex = priorityIndex.get(left.key);
      const rightIndex = priorityIndex.get(right.key);

      if (leftIndex !== undefined && rightIndex !== undefined) return leftIndex - rightIndex;
      if (leftIndex !== undefined) return -1;
      if (rightIndex !== undefined) return 1;
      return left.key.localeCompare(right.key);
    });
  }, [configs]);

  if (loading) return <p>Loading Admin Dashboard...</p>;

  const adminCount = users.filter((user) => user.role === 'admin').length;
  const configHealth = configs.filter((config) => config.value.trim()).length;
  const adminCoverage = users.length > 0 ? Math.round((adminCount / users.length) * 100) : 0;

  return (
    <div className="admin-dashboard">
      <section className="admin-hero card">
        <div className="admin-hero-copy">
          <p className="eyebrow">Control Room</p>
          <h2>Admin Dashboard</h2>
          <p className="admin-hero-text">
            Operate user access, validate config state, and watch system traffic from a single view.
          </p>
        </div>
        <div className="admin-hero-actions">
          <button type="button" onClick={fetchData}>Refresh data</button>
          <div className="admin-sync-state">
            <span className="status-dot" />
            <span>{statusMessage || 'Live data loaded'}</span>
          </div>
        </div>
      </section>

      {error && <div className="error">{error}</div>}

      <section className="admin-kpis">
        <div className="metric-card admin-kpi">
          <h3>Total Messages</h3>
          <p className="metric-value">{metrics?.total_messages ?? 0}</p>
          <p className="small-text">Across all connected pager nodes</p>
        </div>

        <div className="metric-card admin-kpi">
          <h3>User Accounts</h3>
          <p className="metric-value">{users.length}</p>
          <p className="small-text">{adminCoverage}% admin coverage</p>
        </div>

        <div className="metric-card admin-kpi">
          <h3>Config Health</h3>
          <p className="metric-value">{configHealth}/{configs.length}</p>
          <p className="small-text">Non-empty configuration values</p>
        </div>

        <div className="metric-card admin-kpi">
          <h3>Traffic Sources</h3>
          <p className="metric-value">{metrics?.by_client?.length ?? 0}</p>
          <p className="small-text">Nodes actively reporting</p>
        </div>
      </section>

      <section className="admin-section card receiver-overview">
        <div className="card-header">
          <div>
            <h2>Receiver Fleet</h2>
            <p className="small-text">Broker-reported receiver state; refreshes every 15 seconds.</p>
          </div>
          <button type="button" onClick={fetchReceivers}>Refresh receivers</button>
        </div>
        {receiverError && <div className="error">{receiverError}</div>}
        {isMobile ? (
          <ul className="receiver-cards">
            {receivers.length ? receivers.map((receiver) => (
              <li key={receiver.receiver_id} className="receiver-card">
                <div className="receiver-card-head">
                  <strong>{receiver.receiver_id}</strong>
                  <span className={`badge receiver-status-${receiver.status}`}>
                    {receiver.status === 'stale' ? 'stale' : receiver.active_detector ? 'detector (active)' : receiver.status}
                  </span>
                </div>
                <dl className="receiver-card-detail">
                  <div><dt>Role</dt><dd>{receiver.role || '—'}</dd></div>
                  <div><dt>Freq now</dt><dd>{receiver.frequency_hz == null ? '—' : `${(receiver.frequency_hz / 1e6).toFixed(6)} MHz`}</dd></div>
                  <div><dt>Gain / PPM</dt><dd>{receiver.gain_db == null ? '—' : `${receiver.gain_db} dB`} / {receiver.ppm == null ? '—' : `${receiver.ppm} ppm`}</dd></div>
                  <div><dt>Reported</dt><dd>{Number.isNaN(Date.parse(receiver.observed_at)) ? '—' : new Date(receiver.observed_at).toLocaleTimeString()}</dd></div>
                </dl>
              </li>
            )) : <li className="empty-state">No receiver reports received yet.</li>}
          </ul>
        ) : (
          <div className="table-container">
            <table>
              <thead><tr>
                <th>Receiver / Channel</th><th>Host / Device</th><th>Role</th><th>Status</th>
                <th>Nominal / Current Frequency</th><th>Configured Gain / PPM</th><th>Description</th><th>Last report</th>
              </tr></thead>
              <tbody>
                {receivers.length ? receivers.map((receiver) => (
                  <tr key={receiver.receiver_id}>
                    <td><strong>{receiver.receiver_id}</strong><br />{receiver.channel_name || '—'}</td>
                    <td>{receiver.host || '—'}<br /><span className="small-text">{receiver.device_identifier || '—'}</span></td>
                    <td>{receiver.role}</td>
                    <td><span className={`badge receiver-status-${receiver.status}`}>
                      {receiver.status === 'stale' ? 'stale' : receiver.active_detector ? 'detector (active)' : receiver.status}
                    </span></td>
                    <td>{receiver.nominal_frequency_hz == null ? '—' : `${(receiver.nominal_frequency_hz / 1e6).toFixed(6)} MHz`}<br />
                      <span className="small-text">Now: {receiver.frequency_hz == null ? '—' : `${(receiver.frequency_hz / 1e6).toFixed(6)} MHz`}</span></td>
                    <td>Config: {receiver.configured_gain_db == null ? '—' : `${receiver.configured_gain_db} dB`} / {receiver.configured_ppm == null ? '—' : `${receiver.configured_ppm} ppm`}<br />
                      <span className="small-text">Now: {receiver.gain_db == null ? '—' : `${receiver.gain_db} dB`} / {receiver.ppm == null ? '—' : `${receiver.ppm} ppm`}</span></td>
                    <td>{receiver.description || '—'}</td>
                    <td>{Number.isNaN(Date.parse(receiver.observed_at)) ? '—' : new Date(receiver.observed_at).toLocaleTimeString()}</td>
                  </tr>
                )) : <tr><td colSpan={8} className="empty-table">No receiver reports received yet.</td></tr>}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="admin-section card receiver-waterfall">
        <div className="card-header">
          <div>
            <h2>Host SDR 0 Waterfall</h2>
            <p className="small-text">Spectrum-only Host SDR 0 · snapshots every 5 minutes · refreshed automatically</p>
          </div>
          <button type="button" onClick={fetchWaterfalls}>Refresh waterfall</button>
        </div>
        {waterfallError && <div className="error">{waterfallError}</div>}
        {waterfallSnapshots.length > 0 ? (
          <>
            <p className="small-text">Latest capture: {new Date(waterfallSnapshots[0].captured_at).toLocaleString()} · {waterfallSnapshots[0].rows.length} time rows · {waterfallSnapshots[0].rows[0]?.power_db.length ?? 0} frequency bins</p>
            <WaterfallCanvas snapshot={waterfallSnapshots[0]} />
          </>
        ) : !waterfallError ? <p className="empty-state">Waiting for the first measured Pi spectrum snapshot.</p> : null}
      </section>

      <section className="admin-grid">
        <div className="card admin-panel">
          <div className="card-header">
            <h2>Role Distribution</h2>
            <span className="count-badge">{users.length} users</span>
          </div>
          <BarChart data={roleData} color="#f97316" />
        </div>

        <div className="card admin-panel">
          <div className="card-header">
            <h2>Message Mix</h2>
            <span className="count-badge secondary">{metrics?.by_type?.length ?? 0} types</span>
          </div>
          <BarChart data={metrics?.by_type || []} color="#2563eb" />
        </div>

        <div className="card admin-panel">
          <div className="card-header">
            <h2>Top Stations</h2>
            <span className="count-badge secondary">{topStations.length} active</span>
          </div>
          <ul className="admin-list">
            {topStations.length > 0 ? topStations.map((station) => (
              <li key={station.label}>
                <span>{station.label}</span>
                <strong>{station.value}</strong>
              </li>
            )) : <li>No station activity yet.</li>}
          </ul>
        </div>

        <div className="card admin-panel">
          <div className="card-header">
            <h2>Node Activity</h2>
            <span className="count-badge secondary">{topClients.length} nodes</span>
          </div>
          <ul className="admin-list">
            {topClients.length > 0 ? topClients.map((client) => (
              <li key={client.label}>
                <span>{client.label}</span>
                <strong>{client.value}</strong>
              </li>
            )) : <li>No client traffic yet.</li>}
          </ul>
        </div>
      </section>

      <section className="admin-section card">
        <div className="card-header">
          <h2>User Management</h2>
          <span className="count-badge">{filteredUsers.length} visible</span>
        </div>

        <div className="admin-toolbar">
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter by name, email, or role"
            aria-label="Filter users"
            className="admin-search"
          />
        </div>

        {isMobile ? (
          <ul className="user-cards">
            {filteredUsers.length > 0 ? filteredUsers.map((user) => (
              <li key={user.id} className="user-card">
                <div className="user-card-head">
                  <strong>{user.name}</strong>
                  <span className={`badge ${user.role === 'admin' ? 'role-admin' : ''}`}>{user.role}</span>
                </div>
                <p className="user-card-email">{user.email}</p>
                <p className="small-text">{user.invited_by_name || 'Self-created / seed'}</p>
                <label className="user-card-role">
                  <span>Role</span>
                  <select
                    value={user.role}
                    onChange={(event) => handleRoleChange(user.id, event.target.value)}
                    className="role-select"
                    aria-label={`Role for ${user.name}`}
                  >
                    <option value="user">User</option>
                    <option value="admin">Admin</option>
                  </select>
                </label>
              </li>
            )) : <li className="empty-state">No users match the current filter.</li>}
          </ul>
        ) : (
          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Invited By</th>
                  <th>Role</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                {filteredUsers.length > 0 ? filteredUsers.map((user) => (
                  <tr key={user.id}>
                    <td>{user.name}</td>
                    <td>{user.email}</td>
                    <td>{user.invited_by_name || 'Self-created / seed'}</td>
                    <td>
                      <span className={`badge ${user.role === 'admin' ? 'role-admin' : ''}`}>
                        {user.role}
                      </span>
                    </td>
                    <td>
                      <select
                        value={user.role}
                        onChange={(event) => handleRoleChange(user.id, event.target.value)}
                        className="role-select"
                      >
                        <option value="user">User</option>
                        <option value="admin">Admin</option>
                      </select>
                    </td>
                  </tr>
                )) : (
                  <tr>
                    <td colSpan={5} className="empty-table">No users match the current filter.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <WhatsAppPairingPanel token={token} />

      <section className="admin-section card">
        <div className="card-header">
          <h2>System Configurations</h2>
          <span className="count-badge secondary">{configs.length} entries</span>
        </div>

        <div className="config-grid">
          {orderedConfigs.map((config) => {
            const meta = CONFIG_META[config.key];

            return (
            <div key={config.key} className="config-item card">
              <label htmlFor={config.key}><strong>{meta?.label || config.key.replace(/_/g, ' ')}</strong></label>
              {meta?.description && (
                <p className="config-description">{meta.description}</p>
              )}
              <div className="config-controls">
                <input
                  id={config.key}
                  type={meta?.type || 'text'}
                  value={draftConfigs[config.key] ?? ''}
                  onChange={(event) => setDraftConfigs((current) => ({
                    ...current,
                    [config.key]: event.target.value
                  }))}
                  onBlur={(event) => handleConfigUpdate(config.key, event.target.value)}
                  placeholder={meta?.placeholder || 'Value'}
                />
                <small className="small-text">Auto-saves on blur</small>
              </div>
            </div>
            );
          })}
        </div>
      </section>
    </div>
  );
}
