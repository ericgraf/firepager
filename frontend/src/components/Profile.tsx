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
  created_by_name?: string;
  is_used: boolean;
  used_by: number | null;
  used_by_name?: string;
}

interface TagInputProps {
  label: string;
  tags: string[];
  onAdd: (tag: string) => void;
  onDelete: (index: number) => void;
  placeholder?: string;
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

  const addSubscriptionItem = (key: keyof typeof subscriptions, item: string) => {
    if (!subscriptions[key].includes(item)) {
      setSubscriptions({ ...subscriptions, [key]: [...subscriptions[key], item] });
    }
  };

  const removeSubscriptionItem = (key: keyof typeof subscriptions, index: number) => {
    const newList = [...subscriptions[key]];
    newList.splice(index, 1);
    setSubscriptions({ ...subscriptions, [key]: newList });
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

      <div className="card">
        <h2>Subscription Management</h2>
        <p style={{ marginBottom: '24px', color: '#666' }}>These subscriptions determine which pager messages and incidents trigger notifications for you.</p>
        
        <form onSubmit={handleSubmit}>
          <div className="subscription-grid">
            <TagInput 
              label="Capcodes" 
              tags={subscriptions.capcodes} 
              onAdd={(item) => addSubscriptionItem('capcodes', item)} 
              onDelete={(idx) => removeSubscriptionItem('capcodes', idx)} 
              placeholder="e.g. 0123456"
            />
            <TagInput 
              label="Vehicles" 
              tags={subscriptions.vehicles} 
              onAdd={(item) => addSubscriptionItem('vehicles', item)} 
              onDelete={(idx) => removeSubscriptionItem('vehicles', idx)} 
              placeholder="e.g. E101, T5"
            />
            <TagInput 
              label="Stations" 
              tags={subscriptions.stations} 
              onAdd={(item) => addSubscriptionItem('stations', item)} 
              onDelete={(idx) => removeSubscriptionItem('stations', idx)} 
              placeholder="e.g. Station 10"
            />
            <TagInput 
              label="Call Types" 
              tags={subscriptions.call_types} 
              onAdd={(item) => addSubscriptionItem('call_types', item)} 
              onDelete={(idx) => removeSubscriptionItem('call_types', idx)} 
              placeholder="e.g. Structure Fire"
            />
          </div>

          <div style={{ marginTop: '32px', borderTop: '1px solid var(--border)', paddingTop: '20px' }}>
            <button type="submit" style={{ background: 'var(--text-h)' }}>Save All Subscriptions</button>
          </div>
        </form>
      </div>
    </div>
  );
}
