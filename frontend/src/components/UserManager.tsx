import { useEffect, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import './components.css';

interface User {
  id: number;
  email: string;
  name: string;
  phone_number: string;
  capcodes: string[];
  role: string;
  pushover_user_key: string;
  pushover_app_token: string;
  invited_by_name?: string;
}

export function UserManager() {
  const [users, setUsers] = useState<User[]>([]);
  const [error, setError] = useState('');
  const { token, user: currentUser } = useAuth();

  const fetchUsers = async () => {
    try {
      const res = await fetch('/api/users', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (!res.ok) throw new Error('Failed to fetch users');
      const data = await res.json();
      setUsers(data || []);
    } catch (err: any) {
      setError(err.message);
    }
  };

  useEffect(() => {
    if (token) {
      fetchUsers();
    }
  }, [token]);

  if (currentUser?.role !== 'admin') {
    return (
      <div className="card">
        <h2>Settings</h2>
        <p>Logged in as: <strong>{currentUser?.name}</strong> ({currentUser?.email})</p>
        <p>Only administrators can manage users and invites.</p>
      </div>
    );
  }

  return (
    <div className="card">
      <div className="card-header">
        <div>
          <h2>Administration</h2>
          <p className="small-text">User invite ownership is now visible in profile and admin views.</p>
        </div>
      </div>
      {error && <div className="error">{error}</div>}

      <section>
        <h3>Users</h3>
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Email</th>
                <th>Invited By</th>
                <th>Role</th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id}>
                  <td>{u.name}</td>
                  <td>{u.email}</td>
                  <td>{u.invited_by_name || 'Self-created / seed'}</td>
                  <td><span className="badge">{u.role}</span></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
