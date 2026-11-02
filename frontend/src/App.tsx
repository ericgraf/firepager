import { Suspense, lazy, type ReactNode } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate, Link, NavLink } from 'react-router-dom';
import { AdminDashboard } from './components/AdminDashboard';
import { Login } from './components/Login';
import { Register } from './components/Register';
import { Profile } from './components/Profile';
import { OperatorDashboard } from './components/OperatorDashboard';
import { AuthProvider, useAuth } from './context/AuthContext';
import './App.css';

const Metrics = lazy(() => import('./components/Metrics').then((module) => ({ default: module.Metrics })));

function ProtectedRoute({ children, adminOnly }: { children: ReactNode, adminOnly?: boolean }) {
  const { token, loading, user } = useAuth();

  if (loading) return <div>Loading...</div>;
  if (!token) return <Navigate to="/login" replace />;
  if (adminOnly && user?.role !== 'admin') return <Navigate to="/" replace />;

  return <>{children}</>;
}

function TabIcon({ name }: { name: 'ops' | 'metrics' | 'admin' | 'profile' }) {
  const common = { width: 20, height: 20, viewBox: '0 0 24 24', fill: 'none', stroke: 'currentColor', strokeWidth: 1.8, strokeLinecap: 'round' as const, strokeLinejoin: 'round' as const, 'aria-hidden': true };
  if (name === 'ops') {
    return <svg {...common}><path d="M4 14h3l2-6 4 10 3-8 2 4h2" /></svg>;
  }
  if (name === 'admin') {
    return <svg {...common}><circle cx="12" cy="12" r="3.2" /><path d="M12 2.5v3M12 18.5v3M2.5 12h3M18.5 12h3M5.3 5.3l2.1 2.1M16.6 16.6l2.1 2.1M18.7 5.3l-2.1 2.1M7.4 16.6l-2.1 2.1" /></svg>;
  }
  if (name === 'metrics') {
    return <svg {...common}><path d="M4 19V5M4 19h16" /><path d="m7 15 3-4 3 2 5-7" /><circle cx="7" cy="15" r="1" fill="currentColor" /><circle cx="10" cy="11" r="1" fill="currentColor" /><circle cx="13" cy="13" r="1" fill="currentColor" /><circle cx="18" cy="6" r="1" fill="currentColor" /></svg>;
  }
  return <svg {...common}><circle cx="12" cy="8" r="3.6" /><path d="M4.5 20.5c1.2-3.6 4.1-5.5 7.5-5.5s6.3 1.9 7.5 5.5" /></svg>;
}

function DashboardLayout({ children }: { children: ReactNode }) {
  const { logout, user } = useAuth();
  const isAdmin = user?.role === 'admin';

  return (
    <div className="app-container">
      <header className="app-header">
        <div className="topbar-brand"><Link to="/" className="brand-link"><span className="brand-mark">F</span><span><strong>firepager</strong><small>operations</small></span></Link></div>
        <nav className="topbar-nav"><Link to="/">Operations</Link><Link to="/metrics">Metrics</Link>{isAdmin && <Link to="/admin">Admin</Link>}<Link to="/profile">Profile</Link><span className="user-chip">{user?.name || 'Operator'}</span><button type="button" className="logout-button" onClick={logout}>Sign out</button></nav>
      </header>

      <main className="app-main">
        {children}
      </main>

      {/* Phone navigation: thumb-reachable, 44px+ targets. Hidden on wide screens. */}
      <nav className="bottom-nav" aria-label="Primary">
        <NavLink to="/" end className={({ isActive }) => `tab-link${isActive ? ' active' : ''}`}>
          <TabIcon name="ops" /><span>Ops</span>
        </NavLink>
        <NavLink to="/metrics" className={({ isActive }) => `tab-link${isActive ? ' active' : ''}`}>
          <TabIcon name="metrics" /><span>Metrics</span>
        </NavLink>
        {isAdmin && <NavLink to="/admin" className={({ isActive }) => `tab-link${isActive ? ' active' : ''}`}>
          <TabIcon name="admin" /><span>Admin</span>
        </NavLink>}
        <NavLink to="/profile" className={({ isActive }) => `tab-link${isActive ? ' active' : ''}`}>
          <TabIcon name="profile" /><span>Profile</span>
        </NavLink>
        <button type="button" className="tab-link tab-signout" onClick={logout} aria-label="Sign out">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M9 4H5.5A1.5 1.5 0 0 0 4 5.5v13A1.5 1.5 0 0 0 5.5 20H9" /><path d="M15 8l4 4-4 4M19 12H9" /></svg>
          <span>Sign out</span>
        </button>
      </nav>
    </div>
  );
}

function MainView() {
  return <OperatorDashboard />;
}

function App() {
  return (
    <AuthProvider>
      <Router>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route 
            path="/" 
            element={
              <ProtectedRoute>
                <DashboardLayout>
                  <MainView />
                </DashboardLayout>
              </ProtectedRoute>
            } 
          />
          <Route 
            path="/admin" 
            element={
              <ProtectedRoute adminOnly>
                <DashboardLayout>
                  <AdminDashboard />
                </DashboardLayout>
              </ProtectedRoute>
            } 
          />
          <Route
            path="/metrics"
            element={
              <ProtectedRoute>
                <DashboardLayout>
                  <Suspense fallback={<div>Loading metrics…</div>}><Metrics /></Suspense>
                </DashboardLayout>
              </ProtectedRoute>
            }
          />
          <Route
            path="/profile"
            element={
              <ProtectedRoute>
                <DashboardLayout>
                  <Profile />
                </DashboardLayout>
              </ProtectedRoute>
            }
          />
        </Routes>
      </Router>
    </AuthProvider>
  );
}

export default App;
