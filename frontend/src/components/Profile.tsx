import { useEffect, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import './components.css';

interface UserProfile {
  id: number;
  email: string;
  name: string;
  phone_number: string;
  capcodes: string[];
  vehicles: string[];
  stations: string[];
  call_types: string[];
  notification_types: string[];
  role: string;
  pushover_user_key: string;
  pushover_app_token: string;
  invited_by: number | null;
  invited_by_name?: string;
}

interface Invite {
  id: number;
  code: string;
  created_at: string;
  expires_at?: string | null;
  expiresAt?: string | null;
  link?: string;
  invite_url?: string;
  revoked_at?: string | null;
  created_by_name?: string;
  is_used: boolean;
  used_by: number | null;
  used_by_name?: string;
}

const inviteApiPaths = ['/api/invites', '/api/profile/invites'];

function inviteExpiry(invite: Invite) {
  const value = invite.expires_at || invite.expiresAt;
  return value ? new Date(value) : null;
}

function inviteIsExpired(invite: Invite) {
  const expiry = inviteExpiry(invite);
  return Boolean(expiry && !Number.isNaN(expiry.getTime()) && expiry.getTime() <= Date.now() && !invite.is_used);
}

function inviteStatus(invite: Invite) {
  if (invite.is_used) return 'Used';
  if (invite.revoked_at) return 'Revoked';
  if (inviteIsExpired(invite)) return 'Expired';
  return 'Active';
}

function inviteLink(invite: Invite) {
  return invite.link || invite.invite_url || `${window.location.origin}/register?invite_code=${encodeURIComponent(invite.code)}`;
}

function InviteManagement({ token }: { token: string | null }) {
  const [invites, setInvites] = useState<Invite[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [copied, setCopied] = useState<string | null>(null);

  const requestInvites = async (options?: RequestInit) => {
    let response: Response | null = null;
    for (const path of inviteApiPaths) {
      response = await fetch(path, {
        ...options,
        headers: { Authorization: `Bearer ${token}`, ...(options?.headers || {}) },
      });
      if (response.status !== 404 && response.status !== 405) break;
    }
    if (!response) throw new Error('Unable to reach the invitation service.');
    return response;
  };

  const loadInvites = async () => {
    setLoading(true);
    setError('');
    try {
      const response = await requestInvites();
      if (!response.ok) throw new Error('Unable to load invitations. Please try again.');
      const data = await response.json();
      setInvites(Array.isArray(data) ? data : (data.invites || data.invitations || []));
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Unable to load invitations.');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { if (token) loadInvites(); }, [token]);

  const createInvite = async () => {
    setCreating(true);
    setError('');
    setNotice('');
    try {
      const response = await requestInvites({ method: 'POST' });
      if (!response.ok) throw new Error('Unable to create an invitation. Please try again.');
      const data = await response.json();
      const created = data.invite || data;
      setInvites((current) => [created, ...current]);
      setNotice('Invitation created. Copy the code or link to share it.');
    } catch (createError) {
      setError(createError instanceof Error ? createError.message : 'Unable to create an invitation.');
    } finally {
      setCreating(false);
    }
  };

  const copyInvite = async (invite: Invite, value: string) => {
    setError('');
    try {
      await navigator.clipboard.writeText(value);
      setCopied(`${invite.id}-${value}`);
      setNotice('Copied to clipboard.');
      window.setTimeout(() => setCopied(null), 1800);
    } catch {
      setError('Copy failed. Select and copy the invitation manually.');
    }
  };

  return (
    <section className="card invite-management" aria-labelledby="invite-heading">
      <div className="card-header">
        <div>
          <h2 id="invite-heading">Invite someone to FirePager</h2>
          <p className="small-text">Create a one-time invitation and share its code or link. Invitations may expire.</p>
        </div>
        <button type="button" onClick={createInvite} disabled={creating} aria-busy={creating}>
          {creating ? 'Creating…' : 'Create invitation'}
        </button>
      </div>
      {error && <div className="error" role="alert">{error}</div>}
      {notice && <div className="subscription-notice" role="status">{notice}</div>}
      {loading ? <p className="invite-loading" role="status">Loading invitations…</p> : invites.length === 0 ? (
        <p className="empty-state">You have not created any invitations yet.</p>
      ) : (
        <div className="invite-list">
          {invites.map((invite) => {
            const link = inviteLink(invite);
            const status = inviteStatus(invite);
            const expiry = inviteExpiry(invite);
            return <article className="invite-item" key={invite.id}>
              <div className="invite-item-main">
                <div className="invite-code-row"><code>{invite.code}</code><span className={`badge invite-${status.toLowerCase()}`}>{status}</span></div>
                <p className="small-text">Created {new Date(invite.created_at).toLocaleDateString()} · {invite.is_used ? 'Redeemed' : expiry ? `Expires ${expiry.toLocaleDateString()}` : 'No expiry provided'}</p>
              </div>
              {!invite.is_used && !invite.revoked_at && !inviteIsExpired(invite) && <div className="invite-actions">
                <button type="button" className="secondary-button" onClick={() => copyInvite(invite, invite.code)} aria-label={`Copy invitation code ${invite.code}`}>
                  {copied === `${invite.id}-${invite.code}` ? 'Copied' : 'Copy code'}
                </button>
                <button type="button" className="secondary-button" onClick={() => copyInvite(invite, link)} aria-label={`Copy invitation link for ${invite.code}`}>
                  {copied === `${invite.id}-${link}` ? 'Copied' : 'Copy link'}
                </button>
              </div>}
            </article>;
          })}
        </div>
      )}
    </section>
  );
}

interface TagInputProps {
  label: string;
  tags: string[];
  onAdd: (tag: string) => void;
  onDelete: (index: number) => void;
  placeholder?: string;
}

type SubscriptionValues = {
  capcodes: string[];
  vehicles: string[];
  stations: string[];
  call_types: string[];
};

type Catalog = {
  stations: string[];
  call_types: string[];
  channels: string[];
};

const CHANNELS = ['SMS', 'WhatsApp', 'Email', 'Push', 'Pushover'];

const uniqueStrings = (items: unknown): string[] => Array.isArray(items)
  ? [...new Set(items.filter((item): item is string => typeof item === 'string' && item.trim().length > 0).map((item) => item.trim()))]
  : [];

function SubscriptionSettings({
  token,
  initialValues,
  initialChannels,
  phoneNumber,
}: {
  token: string | null;
  initialValues: SubscriptionValues;
  initialChannels: string[];
  phoneNumber: string;
}) {
  const [values, setValues] = useState<SubscriptionValues>(initialValues);
  const [channels, setChannels] = useState<string[]>(initialChannels);
  const [catalog, setCatalog] = useState<Catalog>({ stations: [], call_types: [], channels: CHANNELS });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [whatsappConsent, setWhatsappConsent] = useState(false);
  const [verificationCode, setVerificationCode] = useState('');
  const [verificationRequested, setVerificationRequested] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setLoading(true);
      try {
        const headers = { Authorization: `Bearer ${token}` };
        const [subscriptionResponse, catalogResponse] = await Promise.all([
          fetch('/api/profile/subscriptions', { headers }),
          fetch('/api/profile/subscriptions/catalog', { headers }),
        ]);
        if (!cancelled && subscriptionResponse.ok) {
          const data = await subscriptionResponse.json();
          const source = data.subscriptions || data;
          setValues((current) => ({
            capcodes: uniqueStrings(source.capcodes ?? current.capcodes),
            vehicles: uniqueStrings(source.vehicles ?? current.vehicles),
            stations: uniqueStrings(source.stations ?? current.stations),
            call_types: uniqueStrings(source.call_types ?? current.call_types),
          }));
          if (Array.isArray(source.notification_types)) setChannels(uniqueStrings(source.notification_types));
        }
        if (!cancelled && catalogResponse.ok) {
          const data = await catalogResponse.json();
          const source = data.catalog || data;
          setCatalog({
            stations: uniqueStrings(source.stations ?? source.station),
            call_types: uniqueStrings(source.call_types ?? source.callTypes ?? source.call_types_catalog),
            channels: uniqueStrings(source.channels).length ? uniqueStrings(source.channels) : CHANNELS,
          });
        }
      } catch {
        // The catalog is optional: manually entered values remain usable when the API is unavailable.
      } finally {
        if (!cancelled) setLoading(false);
      }
    };
    load();
    return () => { cancelled = true; };
  }, [token]);

  const updateList = (key: keyof SubscriptionValues, next: string[]) => {
    setValues((current) => ({ ...current, [key]: next }));
  };

  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    setNotice('');
    setSaving(true);
    const payload = { ...values, notification_types: channels };
    try {
      let response = await fetch('/api/profile/subscriptions', {
        method: 'PUT',
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      // Older deployments do not have the subscription resource yet.
      if (response.status === 404 || response.status === 405) {
        response = await fetch('/api/profile', {
          method: 'PUT',
          headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
      }
      if (!response.ok) throw new Error('Unable to save subscription settings. Please try again.');
      setNotice('Subscription settings saved.');
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : 'Unable to save subscription settings.');
    } finally {
      setSaving(false);
    }
  };

  const toggleChannel = (channel: string) => {
    setChannels((current) => current.includes(channel) ? current.filter((item) => item !== channel) : [...current, channel]);
  };

  const requestVerification = () => {
    setVerificationRequested(true);
    setNotice('Verification is ready to connect when WhatsApp verification is enabled by the service.');
  };

  return (
    <div className="card subscription-settings">
      <div className="section-heading">
        <div>
          <h2>Subscription settings</h2>
          <p className="small-text">Choose the stations, call types, and channels that should reach you.</p>
        </div>
        {loading && <span className="small-text" role="status">Loading catalog…</span>}
      </div>
      {error && <div className="error" role="alert">{error}</div>}
      {notice && <div className="subscription-notice" role="status">{notice}</div>}
      <form onSubmit={save}>
        <div className="subscription-grid">
          <TagInput label="Capcodes" tags={values.capcodes} onAdd={(item) => updateList('capcodes', [...values.capcodes, item].filter((v, i, all) => all.indexOf(v) === i))} onDelete={(index) => updateList('capcodes', values.capcodes.filter((_, i) => i !== index))} placeholder="e.g. 0123456" />
          <TagInput label="Vehicles" tags={values.vehicles} onAdd={(item) => updateList('vehicles', [...values.vehicles, item].filter((v, i, all) => all.indexOf(v) === i))} onDelete={(index) => updateList('vehicles', values.vehicles.filter((_, i) => i !== index))} placeholder="e.g. E101" />
          <TagInput label="Stations" tags={values.stations} onAdd={(item) => updateList('stations', [...values.stations, item].filter((v, i, all) => all.indexOf(v) === i))} onDelete={(index) => updateList('stations', values.stations.filter((_, i) => i !== index))} placeholder={catalog.stations[0] || 'e.g. Station 10'} />
          <TagInput label="Call types" tags={values.call_types} onAdd={(item) => updateList('call_types', [...values.call_types, item].filter((v, i, all) => all.indexOf(v) === i))} onDelete={(index) => updateList('call_types', values.call_types.filter((_, i) => i !== index))} placeholder={catalog.call_types[0] || 'e.g. Structure Fire'} />
        </div>

        {(catalog.stations.length > 0 || catalog.call_types.length > 0) && <p className="catalog-hint">Catalog suggestions: {[...catalog.stations, ...catalog.call_types].slice(0, 8).join(' · ')}</p>}

        <fieldset className="channel-options">
          <legend>Delivery channels</legend>
          <p className="small-text">Opt in only to channels you can receive. Current selections are shown below.</p>
          <div className="channel-grid">
            {(catalog.channels.length ? catalog.channels : CHANNELS).map((channel) => (
              <label className="channel-option" key={channel}>
                <input type="checkbox" checked={channels.includes(channel)} onChange={() => toggleChannel(channel)} />
                <span><strong>{channel}</strong><small>{channels.includes(channel) ? 'Opted in' : 'Not opted in'}</small></span>
              </label>
            ))}
          </div>
        </fieldset>

        <div className="whatsapp-panel">
          <div><h3>WhatsApp verification</h3><p className="small-text">Verification is required before WhatsApp delivery can be enabled.</p></div>
          <label className="consent-option"><input type="checkbox" checked={whatsappConsent} onChange={(event) => setWhatsappConsent(event.target.checked)} /> <span>I consent to receiving FirePager alerts on WhatsApp at <strong>{phoneNumber || 'the phone number on my profile'}</strong>.</span></label>
          <div className="verification-controls">
            <button type="button" onClick={requestVerification} disabled={!whatsappConsent || !phoneNumber || verificationRequested}>Request verification code</button>
            <input aria-label="WhatsApp verification code" inputMode="numeric" value={verificationCode} onChange={(event) => setVerificationCode(event.target.value)} placeholder="Code (when received)" disabled={!verificationRequested} />
          </div>
          <p className="small-text">No message is sent by this screen yet; this is a consent and verification-ready scaffold.</p>
        </div>

        <div className="subscription-actions"><button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save subscription settings'}</button></div>
      </form>
    </div>
  );
}

function TagInput({ label, tags, onAdd, onDelete, placeholder }: TagInputProps) {
  const [value, setValue] = useState('');

  const handleAdd = () => {
    if (value.trim()) {
      onAdd(value.trim());
      setValue('');
    }
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleAdd();
    }
  };

  return (
    <div className="form-group">
      <label>{label}</label>
      <div className="tag-input-wrapper">
        <input 
          value={value} 
          onChange={(e) => setValue(e.target.value)} 
          onKeyDown={handleKeyPress}
          placeholder={placeholder || `Add ${label.toLowerCase()}...`} 
        />
        <button type="button" className="tag-add-btn" onClick={handleAdd}>Add</button>
      </div>
      <div className="tag-container">
        {tags.length === 0 ? (
          <span style={{ fontSize: '12px', color: '#888' }}>No {label.toLowerCase()} added.</span>
        ) : (
          tags.map((tag, idx) => (
            <div key={`${tag}-${idx}`} className="tag">
              {tag}
              <button type="button" className="tag-delete" onClick={() => onDelete(idx)}>×</button>
            </div>
          ))
        )}
      </div>
    </div>
  );
}

export function Profile() {
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [invites, setInvites] = useState<Invite[]>([]);
  const [inviteLoading, setInviteLoading] = useState(false);
  const { token } = useAuth();

  const [formData, setFormData] = useState({
    email: '',
    name: '',
    phone_number: '',
    password: '',
    pushover_user_key: '',
    pushover_app_token: '',
  });

  const [subscriptions, setSubscriptions] = useState({
    capcodes: [] as string[],
    vehicles: [] as string[],
    stations: [] as string[],
    call_types: [] as string[],
  });

  const [notifications, setNotifications] = useState({
    SMS: false,
    WhatsApp: false,
    Email: false,
    Push: false,
    Pushover: false,
  });

  const fetchProfile = async () => {
    try {
      const res = await fetch('/api/profile', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (!res.ok) throw new Error('Failed to fetch profile');
      const data: UserProfile = await res.json();
      setProfile(data);
      
      setFormData({
        email: data.email,
        name: data.name,
        phone_number: data.phone_number,
        password: '',
        pushover_user_key: data.pushover_user_key || '',
        pushover_app_token: data.pushover_app_token || '',
      });

      setSubscriptions({
        capcodes: data.capcodes || [],
        vehicles: data.vehicles || [],
        stations: data.stations || [],
        call_types: data.call_types || [],
      });

      setNotifications({
        SMS: data.notification_types?.includes('SMS') || false,
        WhatsApp: data.notification_types?.includes('WhatsApp') || data.notification_types?.includes('WHATSAPP') || false,
        Email: data.notification_types?.includes('Email') || false,
        Push: data.notification_types?.includes('Push') || false,
        Pushover: data.notification_types?.includes('Pushover') || false,
      });
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const fetchInvites = async () => {
    try {
      setInviteLoading(true);
      const res = await fetch('/api/admin/invites', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (!res.ok) throw new Error('Failed to fetch invites');
      const data: Invite[] = await res.json();
      setInvites(data || []);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setInviteLoading(false);
    }
  };

  useEffect(() => {
    if (token) {
      fetchProfile();
    }
  }, [token]);

  useEffect(() => {
    if (token && profile?.role === 'admin') {
      fetchInvites();
    }
  }, [token, profile?.role]);

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setFormData({ ...formData, [e.target.name]: e.target.value });
  };

  const handleNotifyChange = (type: keyof typeof notifications) => {
    setNotifications({ ...notifications, [type]: !notifications[type] });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess('');
    
    const selectedNotifications = Object.entries(notifications)
      .filter(([_, value]) => value)
      .map(([key, _]) => key);

    const updateData = {
      ...formData,
      ...subscriptions,
      notification_types: selectedNotifications,
    };

    try {
      const res = await fetch('/api/profile', {
        method: 'PUT',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(updateData),
      });
      if (!res.ok) throw new Error('Failed to update profile');
      setSuccess('Profile updated successfully!');
      setTimeout(() => setSuccess(''), 3000);
      fetchProfile();
    } catch (err: any) {
      setError(err.message);
    }
  };

  const createInvite = async () => {
    try {
      setError('');
      const res = await fetch('/api/admin/invites', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
        },
      });
      if (!res.ok) throw new Error('Failed to create invite');
      fetchInvites();
      setSuccess('Invite code generated successfully!');
      setTimeout(() => setSuccess(''), 3000);
    } catch (err: any) {
      setError(err.message);
    }
  };

  if (loading) return <p>Loading profile...</p>;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '24px' }}>
      {error && <div className="error">{error}</div>}
      {success && <div className="card" style={{ background: '#dcfce7', color: '#166534', padding: '12px 24px', border: '1px solid #bbf7d0', marginBottom: '0' }}>{success}</div>}

      <div className="card">
        <h2>Personal Information</h2>
        <div className="profile-meta">
          <div className="profile-meta-item">
            <span className="profile-meta-label">Role</span>
            <span className={`badge ${profile?.role === 'admin' ? 'role-admin' : ''}`}>{profile?.role}</span>
          </div>
          <div className="profile-meta-item">
            <span className="profile-meta-label">Invited By</span>
            <strong>{profile?.invited_by_name || 'Self-created / seed account'}</strong>
          </div>
        </div>
        <form onSubmit={handleSubmit} className="user-form" style={{ maxWidth: 'none' }}>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(250px, 1fr))', gap: '24px' }}>
            <section>
              <div className="form-group">
                <label>Name</label>
                <input name="name" value={formData.name} onChange={handleInputChange} required />
              </div>
              <div className="form-group">
                <label>Email</label>
                <input name="email" type="email" value={formData.email} onChange={handleInputChange} required />
              </div>
            </section>
            <section>
              <div className="form-group">
                <label>Phone Number</label>
                <input name="phone_number" value={formData.phone_number} onChange={handleInputChange} />
              </div>
              <div className="form-group">
                <label>Pushover User Key</label>
                <input name="pushover_user_key" value={formData.pushover_user_key} onChange={handleInputChange} placeholder="Pushover User Key" />
              </div>
              <div className="form-group">
                <label>Pushover App Token</label>
                <input name="pushover_app_token" value={formData.pushover_app_token} onChange={handleInputChange} placeholder="Pushover App Token" />
              </div>
              <div className="form-group">
                <label>New Password (leave blank to keep current)</label>
                <input name="password" type="password" value={formData.password} onChange={handleInputChange} />
              </div>
            </section>
          </div>

          <div style={{ marginTop: '24px' }}>
            <h3>Notification Preferences</h3>
            <div style={{ display: 'flex', gap: '24px', marginTop: '10px' }}>
              {Object.keys(notifications).map((type) => (
                <label key={type} style={{ display: 'flex', alignItems: 'center', gap: '10px', cursor: 'pointer', fontSize: '15px' }}>
                  <input 
                    type="checkbox" 
                    checked={notifications[type as keyof typeof notifications]} 
                    onChange={() => handleNotifyChange(type as keyof typeof notifications)} 
                    style={{ width: '18px', height: '18px' }}
                  />
                  {type}
                </label>
              ))}
            </div>
          </div>
          
          <div style={{ marginTop: '24px', borderTop: '1px solid var(--border)', paddingTop: '20px' }}>
            <button type="submit">Update Profile</button>
          </div>
        </form>
      </div>

      <InviteManagement token={token} />

      {profile?.role === 'admin' && (
        <div className="card">
          <div className="card-header">
            <div>
              <h2>Invite Codes</h2>
              <p className="small-text">Generate new codes and see which user each code ultimately onboarded.</p>
            </div>
            <button type="button" onClick={createInvite}>Generate Code</button>
          </div>

          <div className="profile-meta profile-meta-invites">
            <div className="profile-meta-item">
              <span className="profile-meta-label">Active</span>
              <strong>{invites.filter((invite) => !invite.is_used).length}</strong>
            </div>
            <div className="profile-meta-item">
              <span className="profile-meta-label">Used</span>
              <strong>{invites.filter((invite) => invite.is_used).length}</strong>
            </div>
            <div className="profile-meta-item">
              <span className="profile-meta-label">Total</span>
              <strong>{invites.length}</strong>
            </div>
          </div>

          <div className="table-container">
            <table>
              <thead>
                <tr>
                  <th>Code</th>
                  <th>Created By</th>
                  <th>Created</th>
                  <th>Invited User</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {inviteLoading ? (
                  <tr>
                    <td colSpan={5} className="empty-table-cell">Loading invite codes...</td>
                  </tr>
                ) : invites.length > 0 ? invites.map((invite) => (
                  <tr key={invite.id}>
                    <td><code>{invite.code}</code></td>
                    <td>{invite.created_by_name || 'Unknown'}</td>
                    <td><small>{new Date(invite.created_at).toLocaleDateString()}</small></td>
                    <td>{invite.used_by_name || 'Not redeemed'}</td>
                    <td>
                      {invite.is_used ? (
                        <span className="badge invite-used">Used</span>
                      ) : (
                        <span className="badge invite-active">Active</span>
                      )}
                    </td>
                  </tr>
                )) : (
                  <tr>
                    <td colSpan={5} className="empty-table-cell">No invite codes generated.</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      <SubscriptionSettings
        token={token}
        initialValues={subscriptions}
        initialChannels={Object.entries(notifications).filter(([, enabled]) => enabled).map(([channel]) => channel)}
        phoneNumber={formData.phone_number}
      />
    </div>
  );
}
