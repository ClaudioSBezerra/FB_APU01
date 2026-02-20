import { BrowserRouter, Routes, Route, Link, Navigate, useLocation } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Toaster } from "@/components/ui/sonner";
import ImportarEFD from './pages/ImportarEFD';
import Mercadorias from './pages/Mercadorias';
import OperacoesSimplesNacional from './pages/OperacoesSimplesNacional';
import Dashboard from './pages/Dashboard';
import ExecutiveSummary from './pages/ExecutiveSummary';
import TabelaAliquotas from './pages/TabelaAliquotas';
import TabelaCFOP from './pages/TabelaCFOP';
import TabelaFornSimples from './pages/TabelaFornSimples';
import GestaoAmbiente from './pages/GestaoAmbiente';
import Managers from './pages/Managers';
import RFBCredentials from './pages/RFBCredentials';
import RFBApuracao from './pages/RFBApuracao';
import GestaoCredIBSCBS from './pages/GestaoCredIBSCBS';
import AdminUsers from './pages/AdminUsers';
import Login from './pages/Login';
import Register from './pages/Register';
import ForgotPassword from './pages/ForgotPassword';
import ResetPassword from './pages/ResetPassword';
import { Button } from '@/components/ui/button';
import { SidebarProvider, SidebarTrigger, SidebarInset } from '@/components/ui/sidebar';
import { AppSidebar } from '@/components/AppSidebar';
import { Separator } from '@/components/ui/separator';
import { AuthProvider, useAuth } from './contexts/AuthContext';

const queryClient = new QueryClient();

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
            <Route path="/operacoes/simples" element={<OperacoesSimplesNacional />} />
            <Route path="/dashboards" element={<Dashboard />} />
            <Route path="/relatorios/resumo-executivo" element={<ExecutiveSummary />} />
            
            {/* Configurações */}
            <Route path="/config/aliquotas" element={<TabelaAliquotas />} />
            <Route path="/config/cfop" element={<TabelaCFOP />} />
            <Route path="/config/forn-simples" element={<TabelaFornSimples />} />
            <Route path="/config/gestores" element={<Managers />} />
            
            {/* Admin Routes */}
            <Route path="/config/usuarios" element={
              <AdminRoute>
                <AdminUsers />
              </AdminRoute>
            } />
            <Route path="/config/ambiente" element={
              <ProtectedRoute>
                <GestaoAmbiente />
              </ProtectedRoute>
            } />
            
            {/* Apuração */}
            <Route path="/apuracao/entrada" element={<ComingSoon title="Importar XMLs Entrada" />} />
            <Route path="/apuracao/saida" element={<ComingSoon title="Importar XMLs Saída" />} />
            <Route path="/apuracao/nfse" element={<ComingSoon title="Importar XMLs NFS-e" />} />
            
            {/* RFB */}
            <Route path="/rfb/credenciais" element={<RFBCredentials />} />
            <Route path="/rfb/apuracao" element={<RFBApuracao />} />
            <Route path="/rfb/gestao-creditos" element={<GestaoCredIBSCBS />} />
            <Route path="/rfb/creditos-cbs" element={<ComingSoon title="Créditos CBS mês corrente" />} />
            <Route path="/rfb/pagamentos-cbs" element={<ComingSoon title="Pagamentos CBS mês corrente" />} />
            <Route path="/rfb/pagamentos-fornecedores" element={<ComingSoon title="Pagamentos CBS a Fornecedores" />} />
            <Route path="/rfb/concluir-apuracao" element={<ComingSoon title="Concluir apuração mês anterior" />} />
          </Routes>
        </div>
        <Toaster />
      </SidebarInset>
    </SidebarProvider>
  );
}

function App() {
  console.log("App Version: 5.6.0 - Gestão Créditos IBS/CBS");
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <AuthProvider>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/register" element={<Register />} />
            <Route path="/forgot-password" element={<ForgotPassword />} />
            <Route path="/reset-senha" element={<ResetPassword />} />
            <Route path="/*" element={
              <ProtectedRoute>
                <AppLayout />
              </ProtectedRoute>
            } />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

export default App;
