import { useEffect, useMemo, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { CallMap } from './CallMap';
import { DashboardCallMap } from './DashboardCallMap';
import './components.css';

interface Message {
  id: string;
  timestamp: string;
  source: string;
  text: string;
  type: string;
  page_id: string;
  call_type: string;
  incident_number: string;
  vehicles: string[];
  address: string;
  intersection: string;
  latitude: number;
  longitude: number;
  client_id: string;
  station: string;
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

function formatVehicles(rawVehicles: string[] | string | null | undefined) {
  const vehicles = extractVehicles(rawVehicles);
  return vehicles.length > 0 ? vehicles.join(', ') : '-';
}

interface MessageFeedProps {
  selectedVehicles: string[];
}

export function MessageFeed({ selectedVehicles }: MessageFeedProps) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selectedCall, setSelectedCall] = useState<Message | null>(null);
  const [deletingId, setDeletingId] = useState<string | null>(null);
  const { token, user } = useAuth();

  const fetchMessages = async () => {
    try {
      const res = await fetch('/api/messages', {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      if (!res.ok) throw new Error('Failed to fetch messages');
      const data = await res.json();
      setMessages(data || []);
      setError('');
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError(String(err));
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!token) return;
    fetchMessages();
    const interval = setInterval(fetchMessages, 5000); // refresh every 5s
    return () => clearInterval(interval);
  }, [token]);

  const deleteCall = async (message: Message) => {
    if (deletingId) return;

    const confirmed = window.confirm(`Delete incident ${message.incident_number || message.id} from the database?`);
    if (!confirmed) return;

    try {
      setDeletingId(message.id);
      const res = await fetch(`/api/admin/messages/${message.id}`, {
        method: 'DELETE',
        headers: {
          Authorization: `Bearer ${token}`
        }
      });
      if (!res.ok) throw new Error('Failed to delete call');

      setMessages((current) => current.filter((currentMessage) => currentMessage.id !== message.id));
      setSelectedCall((current) => (current?.id === message.id ? null : current));
      setError('');
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError(String(err));
      }
    } finally {
      setDeletingId(null);
    }
  };

  const calls = useMemo(
    () => messages.filter((message) => {
      if (message.type !== 'CALL') return false;
      if (selectedVehicles.length === 0) return true;

      const vehicles = extractVehicles(message.vehicles);
      return selectedVehicles.some((selected) =>
        vehicles.includes(selected) || (vehicles.length === 0 && selected === 'Unknown Vehicle')
      );
    }),
    [messages, selectedVehicles]
  );
  const otherMessages = messages.filter(m => m.type !== 'CALL');

  return (
    <div className="feeds-wrapper">
      {error && <div className="error">{error}</div>}

      {selectedCall && (
        <div className="modal-overlay" onClick={() => setSelectedCall(null)}>
          <div className="modal-content" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>Incident #{selectedCall.incident_number} - {selectedCall.call_type}</h3>
              <div className="modal-actions">
                {user?.role === 'admin' && (
                  <button
                    type="button"
                    className="danger-btn"
                    onClick={() => void deleteCall(selectedCall)}
                    disabled={deletingId === selectedCall.id}
                  >
                    {deletingId === selectedCall.id ? 'Deleting...' : 'Delete Call'}
                  </button>
                )}
                <button className="close-btn" onClick={() => setSelectedCall(null)}>&times;</button>
              </div>
            </div>
            <CallMap 
              lat={selectedCall.latitude} 
              lng={selectedCall.longitude} 
              address={selectedCall.address} 
              callType={selectedCall.call_type}
              incidentNumber={selectedCall.incident_number}
            />
            <div className="modal-footer">
              <p><strong>Location:</strong> {selectedCall.address} {selectedCall.intersection ? `(${selectedCall.intersection})` : ''}</p>
              <p><strong>Vehicles:</strong> {formatVehicles(selectedCall.vehicles)}</p>
              <p className="small-text">Timestamp: {new Date(selectedCall.timestamp).toLocaleString()}</p>
            </div>
          </div>
        </div>
      )}
      
      <section className="feed-section">
        <div className="card">
          <div className="card-header">
            <h2>Call Map</h2>
            <span className="count-badge secondary">{calls.length} plotted calls</span>
          </div>
          {selectedVehicles.length > 0 && (
            <p className="small-text filter-summary">Map filtered to units: <strong>{selectedVehicles.join(', ')}</strong>.</p>
          )}
          <DashboardCallMap calls={calls} />
        </div>
      </section>

      <section className="feed-section">
        <div className="card">
          <div className="card-header">
            <h2>Active Calls</h2>
            <span className="count-badge">{calls.length}</span>
          </div>
          {selectedVehicles.length > 0 && (
            <p className="small-text filter-summary">Filtered to units: <strong>{selectedVehicles.join(', ')}</strong>.</p>
          )}
          {loading && messages.length === 0 ? (
            <p>Loading calls...</p>
          ) : calls.length === 0 ? (
            <p className="empty-state">No active calls reported.</p>
          ) : (
            <div className="table-container">
              <table className="message-table">
                <thead>
                  <tr>
                    <th>Time</th>
                    <th>Station</th>
                    <th>Incident #</th>
                    <th>Call Type</th>
                    <th>Location</th>
                    <th>Vehicles</th>
                    <th>Node</th>
                    {user?.role === 'admin' && <th>Actions</th>}
                  </tr>
                </thead>
                <tbody>
                  {calls.map((msg) => (
                    <tr 
                      key={msg.id} 
                      className="row-type-call clickable"
                      onClick={() => setSelectedCall(msg)}
                    >
                      <td><span className="time-text">{new Date(msg.timestamp).toLocaleTimeString()}</span></td>
                      <td><span className="station-badge">{msg.station ? `ST ${msg.station}` : '-'}</span></td>
                      <td><span className="incident-text">#{msg.incident_number || 'N/A'}</span></td>
                      <td><span className="call-type-text">{msg.call_type}</span></td>
                      <td>
                        <div className="location-cell">
                          <div className="address">{msg.address}</div>
                          {msg.intersection && <div className="intersection">Int: {msg.intersection}</div>}
                        </div>
                      </td>
                      <td><div className="vehicles-text">{formatVehicles(msg.vehicles)}</div></td>
                      <td><small className="client-text">{msg.client_id || '?'}</small></td>
                      {user?.role === 'admin' && (
                        <td>
                          <button
                            type="button"
                            className="danger-btn danger-btn-small"
                            onClick={(event) => {
                              event.stopPropagation();
                              void deleteCall(msg);
                            }}
                            disabled={deletingId === msg.id}
                          >
                            {deletingId === msg.id ? 'Deleting...' : 'Delete'}
                          </button>
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </section>

      <section className="feed-section">
        <div className="card">
          <div className="card-header">
            <h2>General Messages</h2>
            <span className="count-badge secondary">{otherMessages.length}</span>
          </div>
          {loading && messages.length === 0 ? (
            <p>Loading messages...</p>
          ) : otherMessages.length === 0 ? (
            <p className="empty-state">No recent messages.</p>
          ) : (
            <div className="table-container">
              <table className="message-table">
                <thead>
                  <tr>
                    <th>Time</th>
                    <th>Type</th>
                    <th>Content</th>
                    <th>Source</th>
                    <th>Node</th>
                  </tr>
                </thead>
                <tbody>
                  {otherMessages.map((msg) => (
                    <tr key={msg.id} className={`row-type-${msg.type?.toLowerCase()}`}>
                      <td><span className="time-text">{new Date(msg.timestamp).toLocaleTimeString()}</span></td>
                      <td><span className="badge">{msg.type}</span></td>
                      <td><div className="content-text">{msg.text}</div></td>
                      <td><small>{msg.source}</small></td>
                      <td><small className="client-text">{msg.client_id || '?'}</small></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
