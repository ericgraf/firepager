import { type ReactNode } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate, Link } from 'react-router-dom';
import { AdminDashboard } from './components/AdminDashboard';
import { Login } from './components/Login';
import { Register } from './components/Register';
import { Profile } from './components/Profile';
import { OperatorDashboard } from './components/OperatorDashboard';
import { AuthProvider, useAuth } from './context/AuthContext';
import './App.css';

function ProtectedRoute({ children, adminOnly }: { children: ReactNode, adminOnly?: boolean }) {
  const { token, loading, user } = useAuth();

  if (loading) return <div>Loading...</div>;
  if (!token) return <Navigate to="/login" replace />;
  if (adminOnly && user?.role !== 'admin') return <Navigate to="/" replace />;

  return <>{children}</>;
}

function DashboardLayout({ children }: { children: ReactNode }) {
  const { logout, user } = useAuth();

  return (
    <div className="app-container">
      <header className="app-header">
        <div className="topbar-brand"><Link to="/" className="brand-link"><span className="brand-mark">F</span><span><strong>firepager</strong><small>operations</small></span></Link></div>
        <nav className="topbar-nav"><Link to="/">Operations</Link>{user?.role === 'admin' && <Link to="/admin">Admin</Link>}<Link to="/profile">Profile</Link><span className="user-chip">{user?.name || 'Operator'}</span><button type="button" className="logout-button" onClick={logout}>Sign out</button></nav>
      </header>

      <main className="app-main">
        {children}
      </main>
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
