import { BrowserRouter, Routes, Route, Link, Navigate, useLocation } from 'react-router-dom';
import { Toaster } from "@/components/ui/sonner";
import ImportarEFD from './pages/ImportarEFD';
import Mercadorias from './pages/Mercadorias';
import Energia from './pages/Energia';
import Transporte from './pages/Transporte';
import Comunicacoes from './pages/Comunicacoes';
import TabelaAliquotas from './pages/TabelaAliquotas';
import TabelaCFOP from './pages/TabelaCFOP';
import GestaoAmbiente from './pages/GestaoAmbiente';
import AdminUsers from './pages/AdminUsers';
import Login from './pages/Login';
import Register from './pages/Register';
import { Button } from '@/components/ui/button';
import { SidebarProvider, SidebarTrigger, SidebarInset } from '@/components/ui/sidebar';
import { AppSidebar } from '@/components/AppSidebar';
import { Separator } from '@/components/ui/separator';
import { AuthProvider, useAuth } from './contexts/AuthContext';

function Home() {
  return (
    <div className="p-8 space-y-4">
      <h1 className="text-3xl font-bold">Bem-vindo ao FB_APU01</h1>
      <p className="text-muted-foreground">Sistema de Apuração Assistida - Reforma Tributária</p>
      <div className="flex gap-4">
        <Link to="/importar-efd">
          <Button>Começar Importação</Button>
        </Link>
        <Link to="/mercadorias">
          <Button variant="outline">Ver Operações Comerciais</Button>
        </Link>
      </div>
    </div>
  );
}

function ComingSoon({ title }: { title: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-[50vh] space-y-4">
      <h1 className="text-2xl font-bold text-muted-foreground">{title}</h1>
      <p className="text-sm text-muted-foreground">Este módulo está em desenvolvimento.</p>
    </div>
  );
}

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth();
  const location = useLocation();

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  return <>{children}</>;
}

function AdminRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, user } = useAuth();
  const location = useLocation();

  if (!isAuthenticated) {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  if (user?.role !== 'admin') {
    return <Navigate to="/" replace />;
  }

  return <>{children}</>;
}

function AppLayout() {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset>
        <header className="flex h-16 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <div className="flex items-center gap-2 text-sm font-medium">
            FB_APU01 / Painel
          </div>
        </header>
        <div className="flex-1 space-y-4 p-4 pt-6">
          <Routes>
            <Route path="/" element={<Home />} />
            
            {/* Simulador da RT */}
            <Route path="/importar-efd" element={<ImportarEFD />} />
            <Route path="/mercadorias" element={<Mercadorias />} />
            <Route path="/energia" element={<Energia />} />
            <Route path="/transporte" element={<Transporte />} />
            <Route path="/comunicacoes" element={<Comunicacoes />} />
            <Route path="/dashboards" element={<ComingSoon title="Dashboards" />} />
            
            {/* Configurações */}
            <Route path="/config/aliquotas" element={<TabelaAliquotas />} />
            <Route path="/config/cfop" element={<TabelaCFOP />} />
            
            {/* Admin Routes */}
            <Route path="/config/usuarios" element={
              <AdminRoute>
                <AdminUsers />
              </AdminRoute>
            } />
            <Route path="/config/ambiente" element={
              <AdminRoute>
                <GestaoAmbiente />
              </AdminRoute>
            } />
            
            {/* Apuração */}
            <Route path="/apuracao/entrada" element={<ComingSoon title="Importar XMLs Entrada" />} />
            <Route path="/apuracao/saida" element={<ComingSoon title="Importar XMLs Saída" />} />
            <Route path="/apuracao/nfse" element={<ComingSoon title="Importar XMLs NFS-e" />} />
            
            {/* RFB */}
            <Route path="/rfb/importar" element={<ComingSoon title="Importar Apuração RFB" />} />
          </Routes>
        </div>
        <Toaster />
      </SidebarInset>
    </SidebarProvider>
  );
}

function App() {
  console.log("App Version: 0.3.0 - Auth & Onboarding Added");
  return (
    <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route path="/*" element={
            <ProtectedRoute>
              <AppLayout />
            </ProtectedRoute>
          } />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}

export default App;
