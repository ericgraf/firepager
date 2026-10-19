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
  phone_verification_status?: string;
  whatsapp_consented?: boolean;
  whatsapp_consent_policy_version?: string;
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

type CatalogEntry = { code: string; label: string };

type Catalog = {
  stations: CatalogEntry[];
  call_types: CatalogEntry[];
};

type Provider = { type: string; label: string; description: string; normalized: boolean };
type ChannelRecord = { type: string; address?: string; enabled?: boolean; verified?: boolean };
type ChannelEligibility = { id?: string; type?: string; configured?: boolean; verified?: boolean; disabled?: boolean; eligible?: boolean; usable?: boolean; delivery_implemented?: boolean };
type WhatsAppStatus = { phone_masked?: string; verified?: boolean; consent?: boolean; eligible?: boolean; status?: string };
type DeliveryRecord = { status?: string; state?: string; created_at?: string; sent_at?: string; updated_at?: string; error?: string };

const PROVIDERS: Provider[] = [
  { type: 'sms', label: 'SMS', description: 'Text messages to your profile phone.', normalized: true },
  { type: 'email', label: 'Email', description: 'Email alerts to your profile address.', normalized: true },
  { type: 'whatsapp', label: 'WhatsApp', description: 'Requires consent and service verification.', normalized: true },
  { type: 'telegram', label: 'Telegram', description: 'Not available in this deployment.', normalized: false },
  { type: 'signal', label: 'Signal', description: 'Not available in this deployment.', normalized: false },
  { type: 'pushover', label: 'Pushover', description: 'Legacy profile channel; requires an app token and user key.', normalized: false },
  { type: 'ntfy', label: 'ntfy', description: 'Not available in this deployment.', normalized: false },
  { type: 'push', label: 'Web push', description: 'Requires a browser push subscription.', normalized: true },
  { type: 'discord', label: 'Discord', description: 'Not available in this deployment.', normalized: false },
  { type: 'slack', label: 'Slack', description: 'Not available in this deployment.', normalized: false },
  { type: 'teams', label: 'Teams', description: 'Not available in this deployment.', normalized: false },
  { type: 'voice', label: 'Voice call', description: 'Not available in this deployment.', normalized: false },
];
const CHANNEL_LABELS: Record<string, string> = Object.fromEntries(PROVIDERS.map((provider) => [provider.type, provider.label]));
const CHANNEL_HELP: Record<string, string> = {
  sms: 'Uses the phone number on your profile.',
  whatsapp: 'Requires a phone number, consent, and service verification.',
  email: 'Uses your profile email address.',
  push: 'Available when a browser subscription is configured by the service.',
};

const uniqueStrings = (items: unknown): string[] => Array.isArray(items)
  ? [...new Set(items.filter((item): item is string => typeof item === 'string' && item.trim().length > 0).map((item) => item.trim()))]
  : [];

function maskPhone(phone: string) {
  const digits = phone.replace(/\D/g, '');
  return digits.length > 4 ? `${phone.trim().startsWith('+') ? '+' : ''}${'•'.repeat(Math.max(0, digits.length - 4))}${digits.slice(-4)}` : phone || 'No phone number configured';
}

function safeError(response: Response, fallback: string) {
  return response.status === 401 || response.status === 403 ? 'You are not authorized to perform this action.' : fallback;
}

function SubscriptionSettings({
  token,
  initialValues,
  initialChannels,
  phoneNumber,
  email,
  pushoverConfigured,
  backendEligible,
}: {
  token: string | null;
  initialValues: SubscriptionValues;
  initialChannels: string[];
  phoneNumber: string;
  email: string;
  pushoverConfigured: boolean;
  backendEligible: boolean;
}) {
  const [values, setValues] = useState<SubscriptionValues>(initialValues);
  const [channels, setChannels] = useState<string[]>(initialChannels.map((channel) => channel.toLowerCase()).filter((channel) => CHANNEL_LABELS[channel]));
  const [catalog, setCatalog] = useState<Catalog>({ stations: [], call_types: [] });
  const [channelRecords, setChannelRecords] = useState<ChannelRecord[]>([]);
  const [search, setSearch] = useState({ stations: '', call_types: '' });
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');
  const [whatsappConsent, setWhatsappConsent] = useState(backendEligible);
  const [policyAccepted, setPolicyAccepted] = useState(backendEligible);
  const [verificationCode, setVerificationCode] = useState('');
  const [verificationRequested, setVerificationRequested] = useState(false);
  const [verificationBusy, setVerificationBusy] = useState(false);
  const [whatsappEligibility, setWhatsappEligibility] = useState<ChannelEligibility | null>(null);
  const [whatsappStatus, setWhatsappStatus] = useState<WhatsAppStatus | null>(null);
  const [delivery, setDelivery] = useState<DeliveryRecord[]>([]);
  const [deliveryError, setDeliveryError] = useState('');
  const [testAvailable, setTestAvailable] = useState(false);
  const [testBusy, setTestBusy] = useState(false);
  const policyVersion = '2026-01';

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
          const rules = Array.isArray(source.rules) ? source.rules : [];
          setValues((current) => ({
            // The normalized API only stores station/call_type rules. Legacy
            // vehicle and capcode preferences remain owned by /api/profile.
            capcodes: current.capcodes,
            vehicles: current.vehicles,
            stations: uniqueStrings(rules.map((rule: { station?: string }) => rule.station)),
            call_types: uniqueStrings(rules.map((rule: { call_type?: string }) => rule.call_type)),
          }));
          if (Array.isArray(source.channels)) {
            const records = source.channels.filter((channel: unknown): channel is ChannelRecord => typeof channel === 'object' && channel !== null && typeof (channel as ChannelRecord).type === 'string');
            setChannelRecords(records);
            const returnedTypes: unknown = records.length ? records.filter((channel: ChannelRecord) => channel.enabled !== false).map((channel: ChannelRecord) => channel.type) : source.channels;
            setChannels(uniqueStrings(returnedTypes).map((channel: string) => channel.toLowerCase()));
          }
        }
        if (!cancelled && catalogResponse.ok) {
          const data = await catalogResponse.json();
          const source = data.catalog || data;
           const entries = (items: unknown): CatalogEntry[] => Array.isArray(items) ? items.map((item) => typeof item === 'string' ? { code: item, label: item } : item as CatalogEntry).filter((item) => item?.code) : [];
            setCatalog({
             stations: entries(source.stations),
             call_types: entries(source.call_types ?? source.callTypes ?? source.call_types_catalog),
            });
            const channel = (Array.isArray(source.channels) ? source.channels : []).find((item: ChannelEligibility) => (item.id || item.type)?.toLowerCase() === 'whatsapp');
            setWhatsappEligibility(channel || null);
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

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    const loadWhatsApp = async () => {
      const headers = { Authorization: `Bearer ${token}` };
      const statusPaths = ['/api/profile/whatsapp/status', '/api/profile/subscriptions/whatsapp/status'];
      for (const path of statusPaths) {
        try {
          const response = await fetch(path, { headers });
          if (response.status === 404 || response.status === 405) continue;
          if (response.ok) {
            const data = await response.json();
            if (!cancelled) setWhatsappStatus(data.status || data);
          }
          break;
        } catch { break; }
      }
      const deliveryPaths = ['/api/profile/whatsapp/deliveries', '/api/profile/whatsapp/delivery-status'];
      for (const path of deliveryPaths) {
        try {
          const response = await fetch(path, { headers });
          if (response.status === 404 || response.status === 405) continue;
          if (!response.ok) { if (!cancelled) setDeliveryError(safeError(response, 'Recent WhatsApp delivery status is unavailable.')); break; }
          const data = await response.json();
          const records = Array.isArray(data) ? data : (data.deliveries || data.items || []);
          if (!cancelled) setDelivery(Array.isArray(records) ? records.slice(0, 5) : []);
          break;
        } catch { if (!cancelled) setDeliveryError('Recent WhatsApp delivery status is unavailable.'); break; }
      }
      const testPaths = ['/api/profile/whatsapp/test', '/api/profile/subscriptions/whatsapp/test'];
      for (const path of testPaths) {
        try {
          const response = await fetch(path, { headers });
          if (response.status === 404) continue;
          if (!cancelled) setTestAvailable(response.ok || response.status === 405);
          break;
        } catch { break; }
      }
    };
    loadWhatsApp();
    return () => { cancelled = true; };
  }, [token]);

  const updateList = (key: keyof SubscriptionValues, next: string[]) => {
    setValues((current) => ({ ...current, [key]: next }));
    setDirty(true);
  };

  const save = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    setNotice('');
    setSaving(true);
    const ruleStations = values.stations.length ? values.stations : [''];
    const ruleCallTypes = values.call_types.length ? values.call_types : [''];
    const rules = ruleStations.flatMap((station) => ruleCallTypes.map((call_type) => ({ station, call_type, enabled: true })));
    const addressFor = (channel: string) => channel === 'email' ? email : phoneNumber;
    const payload = {
      enabled: true,
      rules,
       channels: channels.filter((type) => PROVIDERS.find((provider) => provider.type === type)?.normalized && (type !== 'whatsapp' || whatsappEligible)).map((type) => ({ type, address: addressFor(type), enabled: true })),
    };
    try {
      let response = await fetch('/api/profile/subscriptions', {
        method: 'PUT',
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      let normalizedSaved = response.ok;
      if (response.status === 404 || response.status === 405) {
        normalizedSaved = false;
        response = await fetch('/api/profile', {
          method: 'PUT',
          headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...values, notification_types: channels.map((channel) => CHANNEL_LABELS[channel] || channel) }),
        });
      }
      if (!response.ok) throw new Error('Unable to save subscription settings. Please try again.');
      // Vehicles/capcodes are legacy profile preferences, not normalized rules.
      const legacyResponse = normalizedSaved ? await fetch('/api/profile', {
          method: 'PUT',
          headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...values, notification_types: channels.map((channel) => CHANNEL_LABELS[channel] || channel) }),
        }) : response;
      if (!legacyResponse.ok) throw new Error('Alert rules saved, but legacy vehicle/capcode preferences could not be saved.');
      setDirty(false);
      setNotice('Subscription settings saved.');
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : 'Unable to save subscription settings.');
    } finally {
      setSaving(false);
    }
  };

  const toggleChannel = (channel: string) => {
    setChannels((current) => current.includes(channel) ? current.filter((item) => item !== channel) : [...current, channel]);
    setDirty(true);
  };

  const whatsappEligible = Boolean((backendEligible || whatsappEligibility?.configured || whatsappEligibility?.usable || whatsappEligibility?.eligible) && !whatsappEligibility?.disabled);
  const whatsappVerified = Boolean(backendEligible || whatsappStatus?.verified || whatsappEligibility?.verified || channelRecords.find((record) => record.type.toLowerCase() === 'whatsapp')?.verified);

  const requestVerification = async () => {
    if (!whatsappConsent || !policyAccepted || !phoneNumber || !whatsappEligible) return;
    setVerificationBusy(true); setError(''); setNotice('');
    try {
      let response: Response | null = null;
      for (const path of ['/api/profile/phone/verify', '/api/profile/phone/verify/resend', '/api/profile/whatsapp/verification/start', '/api/profile/whatsapp/verify/start']) {
        response = await fetch(path, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ phone_number: phoneNumber, consent: true }) });
        if (response.status !== 404 && response.status !== 405) break;
      }
      if (!response || !response.ok) throw new Error(response ? safeError(response, 'Unable to start WhatsApp verification.') : 'Unable to start WhatsApp verification.');
      setVerificationRequested(true); setNotice('A verification code was sent to your WhatsApp number.');
    } catch (requestError) { setError(requestError instanceof Error ? requestError.message : 'Unable to start WhatsApp verification.'); }
    finally { setVerificationBusy(false); }
  };

  const confirmVerification = async () => {
    if (!verificationCode.trim() || !whatsappConsent || !policyAccepted || !whatsappEligible) return;
    setVerificationBusy(true); setError(''); setNotice('');
    try {
      let response: Response | null = null;
      for (const path of ['/api/profile/phone/verify/confirm', '/api/profile/whatsapp/verification/confirm', '/api/profile/whatsapp/verify/confirm']) {
        response = await fetch(path, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ code: verificationCode.trim(), consent: true }) });
        if (response.status !== 404 && response.status !== 405) break;
      }
      if (!response || !response.ok) throw new Error(response ? safeError(response, 'The verification code could not be confirmed.') : 'The verification code could not be confirmed.');
      setWhatsappStatus((current) => ({ ...current, verified: true, consent: true })); setNotice('WhatsApp is verified.'); setVerificationCode('');
    } catch (confirmError) { setError(confirmError instanceof Error ? confirmError.message : 'The verification code could not be confirmed.'); }
    finally { setVerificationBusy(false); }
  };

  const sendTestAlert = async () => {
    setTestBusy(true); setError(''); setNotice('');
    try {
      let response: Response | null = null;
      for (const path of ['/api/profile/whatsapp/test', '/api/profile/subscriptions/whatsapp/test']) {
        response = await fetch(path, { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ consent: true }) });
        if (response.status !== 404 && response.status !== 405) break;
      }
      if (!response || !response.ok) throw new Error(response ? safeError(response, 'The test alert could not be sent.') : 'The test alert endpoint is not available.');
      setNotice('Test alert requested. Check recent delivery status for the result.');
    } catch (testError) { setError(testError instanceof Error ? testError.message : 'The test alert could not be sent.'); }
    finally { setTestBusy(false); }
  };

  const toggleCatalog = (key: 'stations' | 'call_types', code: string) => {
    updateList(key, values[key].includes(code) ? values[key].filter((item) => item !== code) : [...values[key], code]);
  };

  const filtered = (key: 'stations' | 'call_types') => catalog[key].filter((entry) => `${entry.label} ${entry.code}`.toLowerCase().includes(search[key].toLowerCase()));
  const providerStatus = (provider: Provider) => {
    const record = channelRecords.find((channel) => channel.type.toLowerCase() === provider.type);
     if (provider.type === 'whatsapp' && whatsappVerified) return { label: 'Verified', className: 'verified' };
     if (record?.verified) return { label: 'Verified', className: 'verified' };
    if (record?.enabled && record.address) return { label: 'Configured', className: 'configured' };
    if (provider.type === 'sms' && phoneNumber) return { label: 'Available', className: 'available' };
    if (provider.type === 'email' && email) return { label: 'Available', className: 'available' };
    if (provider.type === 'pushover' && pushoverConfigured) return { label: 'Configured (legacy)', className: 'configured' };
     if (provider.type === 'whatsapp' && !whatsappEligible) return { label: 'Unavailable', className: 'unsupported' };
     if (provider.type === 'whatsapp' && phoneNumber) return { label: 'Verification required', className: 'pending' };
    if (provider.normalized && provider.type === 'push') return { label: 'Browser setup required', className: 'pending' };
    return { label: 'Unsupported', className: 'unsupported' };
  };

  return (
    <div className="card subscription-settings">
      <div className="section-heading">
        <div>
          <h2>Subscription settings</h2>
          <p className="small-text">Choose alert rules first, then choose how those alerts should be delivered.</p>
        </div>
        {loading && <span className="small-text" role="status">Loading catalog…</span>}
      </div>
      {error && <div className="error" role="alert">{error}</div>}
        {notice && <div className="subscription-notice" role="status" aria-live="polite">{notice}</div>}
        {dirty && !saving && <p className="save-state" role="status">Unsaved changes</p>}
        <form onSubmit={save}>
        <section className="rule-section" aria-labelledby="rule-heading">
          <div className="section-heading"><div><h3 id="rule-heading">Alert rules</h3><p className="small-text">Select any station and call type combination. Empty selections mean “all”.</p></div><span className="section-count">{values.stations.length + values.call_types.length} selected</span></div>
          <div className="subscription-grid">
            {(['stations', 'call_types'] as const).map((key) => <div className="catalog-control" key={key}>
              <label htmlFor={`${key}-search`}>{key === 'call_types' ? 'Call types' : 'Stations'}</label>
              <input id={`${key}-search`} type="search" value={search[key]} onChange={(event) => setSearch({ ...search, [key]: event.target.value })} placeholder={`Search ${key === 'call_types' ? 'call types' : 'stations'}…`} />
              <div className="catalog-list" role="group" aria-label={`Select ${key === 'call_types' ? 'call types' : 'stations'}`}>
                {filtered(key).map((entry) => <label className="catalog-option" key={entry.code}><input type="checkbox" checked={values[key].includes(entry.code)} onChange={() => toggleCatalog(key, entry.code)} /><span>{entry.label}<small>{entry.code !== entry.label ? entry.code : ''}</small></span></label>)}
                {filtered(key).length === 0 && <span className="empty-state">No catalog matches. Add a value below.</span>}
              </div>
              <TagInput label="" tags={values[key]} onAdd={(item) => updateList(key, [...values[key], item].filter((v, i, all) => all.indexOf(v) === i))} onDelete={(index) => updateList(key, values[key].filter((_, i) => i !== index))} placeholder={`Add ${key === 'call_types' ? 'call type' : 'station'}…`} />
            </div>)}
          </div>
          <div className="legacy-rule-grid"><TagInput label="Vehicles" tags={values.vehicles} onAdd={(item) => updateList('vehicles', [...values.vehicles, item].filter((v, i, all) => all.indexOf(v) === i))} onDelete={(index) => updateList('vehicles', values.vehicles.filter((_, i) => i !== index))} placeholder="e.g. E101" /><TagInput label="Capcodes" tags={values.capcodes} onAdd={(item) => updateList('capcodes', [...values.capcodes, item].filter((v, i, all) => all.indexOf(v) === i))} onDelete={(index) => updateList('capcodes', values.capcodes.filter((_, i) => i !== index))} placeholder="e.g. 0123456" /></div>
          <p className="catalog-hint">Vehicle and capcode filters are retained through the profile service. The normalized subscription API currently stores station and call type rules only.</p>
        </section>

        <fieldset className="channel-options">
          <legend>Delivery channels</legend>
          <p className="small-text">Opt in only to channels you can receive. Configured and verified providers are shown separately from unsupported options.</p>
          <p className="provider-legend" aria-label="Provider status legend"><span className="verified-key">Verified</span> <span className="configured-key">Configured</span> <span className="pending-key">Setup required</span> <span>Unsupported</span></p>
          <div className="channel-grid">
            {PROVIDERS.map((provider) => {
              const status = providerStatus(provider);
              const selectable = provider.normalized || provider.type === 'pushover';
              return <label className={`channel-option ${status.className}`} key={provider.type}>
               <input type="checkbox" checked={channels.includes(provider.type)} onChange={() => toggleChannel(provider.type)} disabled={!selectable || (provider.type === 'whatsapp' && (!phoneNumber || !whatsappEligible || !whatsappVerified))} />
                <span><strong>{provider.label}</strong><small>{CHANNEL_HELP[provider.type] || provider.description}</small><small className="provider-status">{status.label}{channels.includes(provider.type) ? ' · Selected' : ''}</small></span>
              </label>
            })}
          </div>
        </fieldset>

        <div className="whatsapp-panel">
           <div><h3>WhatsApp setup</h3><p className="small-text">Number on file: <strong>{whatsappStatus?.phone_masked || maskPhone(phoneNumber)}</strong></p><p className="small-text">{whatsappEligible ? 'This service is available for your account.' : 'WhatsApp is unavailable until the service is enabled and eligible by the server.'}</p></div>
           <label className="consent-option"><input type="checkbox" checked={whatsappConsent} onChange={(event) => { setWhatsappConsent(event.target.checked); setDirty(true); }} disabled={!whatsappEligible} /> <span>I consent to receiving FirePager alerts on WhatsApp at this number.</span></label>
           <label className="consent-option"><input type="checkbox" checked={policyAccepted} onChange={async (event) => {
             const accepted = event.target.checked;
             setPolicyAccepted(accepted); setDirty(true);
             if (!accepted) return;
             try {
               const response = await fetch('/api/profile/whatsapp/consent', { method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' }, body: JSON.stringify({ policy_version: policyVersion }) });
               if (!response.ok) throw new Error(safeError(response, 'Unable to save WhatsApp consent.'));
               setWhatsappStatus((current) => ({ ...current, consent: true }));
             } catch (consentError) { setPolicyAccepted(false); setError(consentError instanceof Error ? consentError.message : 'Unable to save WhatsApp consent.'); }
           }} disabled={!whatsappEligible} /> <span>I have read and accept the <a href="/whatsapp-policy" target="_blank" rel="noreferrer">WhatsApp messaging policy</a>.</span></label>
           <div className="verification-controls">
             <button type="button" onClick={requestVerification} disabled={!whatsappConsent || !policyAccepted || !phoneNumber || !whatsappEligible || verificationRequested || verificationBusy}>{verificationBusy ? 'Working…' : 'Send verification code'}</button>
             <label className="sr-only" htmlFor="whatsapp-code">WhatsApp verification code</label><input id="whatsapp-code" inputMode="numeric" value={verificationCode} onChange={(event) => setVerificationCode(event.target.value)} placeholder="Code (when received)" disabled={!verificationRequested} />
             <button type="button" className="secondary-button" onClick={confirmVerification} disabled={!verificationRequested || !verificationCode.trim() || verificationBusy}>Confirm code</button>
           </div>
           {whatsappVerified && testAvailable && <button type="button" className="secondary-button" onClick={sendTestAlert} disabled={!whatsappEligible || testBusy}>{testBusy ? 'Sending…' : 'Send test alert'}</button>}
           <div className="delivery-status"><h4>Recent WhatsApp delivery</h4>{deliveryError ? <p className="small-text">{deliveryError}</p> : delivery.length ? <ul>{delivery.map((item, index) => <li key={`${item.created_at || item.updated_at || index}`}><strong>{item.status || item.state || 'Unknown'}</strong>{(item.created_at || item.sent_at || item.updated_at) && <span> · {new Date(item.created_at || item.sent_at || item.updated_at || '').toLocaleString()}</span>}</li>)}</ul> : <p className="small-text">No recent delivery status is available.</p>}</div>
        </div>

        <div className="subscription-actions"><span className="small-text" aria-live="polite">{saving ? 'Saving your preferences…' : dirty ? 'Changes are not saved' : 'All changes saved'}</span><button type="submit" disabled={saving}>{saving ? 'Saving…' : 'Save subscription settings'}</button></div>
      </form>
    </div>
  );
}

function TagInput({ label, tags, onAdd, onDelete, placeholder }: TagInputProps) {
  const [value, setValue] = useState('');
  const inputId = `tag-${label.toLowerCase().replace(/\s+/g, '-') || 'custom'}`;

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
      {label && <label htmlFor={inputId}>{label}</label>}
      <div className="tag-input-wrapper">
        <input 
          id={inputId}
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
          <button type="button" className="tag-delete" onClick={() => onDelete(idx)} aria-label={`Remove ${tag}`}>×</button>
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
        email={formData.email}
        pushoverConfigured={Boolean(formData.pushover_user_key && formData.pushover_app_token)}
        backendEligible={profile?.phone_verification_status === 'verified' && Boolean(profile.whatsapp_consented)}
      />
    </div>
  );
}
