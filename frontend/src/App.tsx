import { type ReactNode, useState } from 'react';
import { BrowserRouter as Router, Routes, Route, Navigate, Link } from 'react-router-dom';
import { MessageFeed } from './components/MessageFeed';
import { Metrics } from './components/Metrics';
import { AdminDashboard } from './components/AdminDashboard';
import { Login } from './components/Login';
import { Register } from './components/Register';
import { Profile } from './components/Profile';
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
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', width: '100%' }}>
          <div>
            <Link to="/" style={{ textDecoration: 'none', color: 'inherit' }}>
              <h1>Firepager</h1>
            </Link>
            <p>SDR Pager Message Dashboard</p>
          </div>
          <div style={{ textAlign: 'right', display: 'flex', alignItems: 'center', gap: '20px' }}>
            <nav style={{ display: 'flex', gap: '15px', fontWeight: 600 }}>
              <Link to="/" style={{ color: 'var(--accent)', textDecoration: 'none' }}>Home</Link>
              <Link to="/profile" style={{ color: 'var(--accent)', textDecoration: 'none' }}>Profile</Link>
            </nav>
            <div>
              <p style={{ margin: 0 }}>Logged in as <strong>{user?.name}</strong></p>
              <button onClick={logout} style={{ marginTop: '8px', padding: '4px 12px', fontSize: '14px' }}>Logout</button>
            </div>
          </div>
        </div>
      </header>

      <main className="app-main">
        {children}
      </main>
    </div>
  );
}

function MainView() {
  const [selectedVehicles, setSelectedVehicles] = useState<string[]>([]);

  const toggleVehicle = (vehicle: string | null) => {
    if (vehicle === null) {
      setSelectedVehicles([]);
      return;
    }

    setSelectedVehicles((current) =>
      current.includes(vehicle)
        ? current.filter((v) => v !== vehicle)
        : [...current, vehicle]
    );
  };

  return (
    <>
      <Metrics selectedVehicles={selectedVehicles} onToggleVehicle={toggleVehicle} />
      <MessageFeed selectedVehicles={selectedVehicles} />
    </>
  );
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
