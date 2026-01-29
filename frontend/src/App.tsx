import { BrowserRouter, Routes, Route, Link } from 'react-router-dom';
import { Toaster } from "@/components/ui/sonner";
import ImportarEFD from './pages/ImportarEFD';
import { Button } from '@/components/ui/button';

function Home() {
  return (
    <div className="p-8 space-y-4">
      <h1 className="text-3xl font-bold">Bem-vindo ao FB_APU01</h1>
      <p className="text-muted-foreground">Sistema de Apuração Assistida</p>
      <div className="flex gap-4">
        <Link to="/importar-efd">
          <Button>Começar Importação</Button>
        </Link>
      </div>
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <div className="min-h-screen bg-background font-sans antialiased">
        <header className="sticky top-0 z-50 w-full border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
          <div className="container flex h-14 items-center">
            <div className="mr-4 flex">
              <Link to="/" className="mr-6 flex items-center space-x-2 font-bold">
                FB_APU01
              </Link>
              <nav className="flex items-center space-x-6 text-sm font-medium">
                <Link to="/importar-efd" className="transition-colors hover:text-foreground/80">Importar</Link>
              </nav>
            </div>
          </div>
        </header>
        <main>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/importar-efd" element={<ImportarEFD />} />
          </Routes>
        </main>
        <Toaster />
      </div>
    </BrowserRouter>
  );
}

export default App;