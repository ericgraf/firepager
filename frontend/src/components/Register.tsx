import { useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { useNavigate, Link } from 'react-router-dom';
import './components.css';

export function Register() {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  const [inviteCode, setInviteCode] = useState(() => new URLSearchParams(window.location.search).get('invite_code') || new URLSearchParams(window.location.search).get('invite') || '');
  const [error, setError] = useState('');
  
  const { login } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    try {
      const res = await fetch('/api/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email,
          password,
          name,
          phone_number: phone,
          invite_code: inviteCode,
        }),
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'Registration failed');
      
      login(data.token, data.user);
      navigate('/');
    } catch (err: unknown) {
      if (err instanceof Error) {
        setError(err.message);
      } else {
        setError(String(err));
      }
    }
  };

  return (
    <div className="card" style={{ maxWidth: '400px', margin: '50px auto' }}>
      <h2>Register</h2>
      {error && <div className="error">{error}</div>}
      <form onSubmit={handleSubmit} className="user-form">
        <div className="form-group">
          <label>Email:</label>
          <input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="user@example.com" />
        </div>
        <div className="form-group">
          <label>Password:</label>
          <input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} placeholder="••••••••" />
        </div>
        <div className="form-group">
          <label>Full Name:</label>
          <input type="text" required value={name} onChange={(e) => setName(e.target.value)} placeholder="John Doe" />
        </div>
        <div className="form-group">
          <label>Phone Number:</label>
          <input type="text" required value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+1234567890" />
        </div>
        <div className="form-group">
          <label>Invite Code:</label>
          <input type="text" required value={inviteCode} onChange={(e) => setInviteCode(e.target.value)} placeholder="a1b2c3d4e5f6" />
        </div>
        <button type="submit">Register</button>
      </form>
      <p style={{ marginTop: '16px' }}>
        Already have an account? <Link to="/login">Login</Link>
      </p>
    </div>
  );
}
