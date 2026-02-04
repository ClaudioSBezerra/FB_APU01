import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { toast } from "sonner";
import { useNavigate, Link } from "react-router-dom";
import { useAuth } from "@/contexts/AuthContext";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AlertCircle } from "lucide-react";

const Login = () => {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const navigate = useNavigate();
  const { login } = useAuth();

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setErrorMsg(null);

    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      });

      const data = await res.json();

      if (!res.ok) {
        throw new Error(typeof data === 'string' ? data : "Credenciais inválidas");
      }

      login(data);
      toast.success("Login realizado com sucesso!");
      navigate("/mercadorias");
    } catch (error: any) {
      const msg = error.message || "Erro desconhecido";
      setErrorMsg(msg);
      toast.error(msg);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="flex items-center justify-center min-h-screen bg-gray-100 px-4">
      <div className="w-full max-w-[450px]">
        <Card className="w-full shadow-lg">
          <CardHeader className="flex flex-row items-center gap-4 space-y-0 pb-6">
            <div className="flex-shrink-0">
              <img 
                src="/logo.png" 
                alt="Logo da Empresa" 
                className="h-16 w-auto object-contain"
                onError={(e) => {
                  e.currentTarget.style.display = 'none'; // Hide if missing
                }} 
              />
            </div>
            <div className="flex flex-col space-y-1.5">
              <CardTitle className="text-xl font-bold">Acesse sua conta</CardTitle>
              <CardDescription>Entre com suas credenciais para continuar</CardDescription>
            </div>
          </CardHeader>
          <CardContent>
            {errorMsg && (
              <Alert variant="destructive" className="mb-4">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>Erro</AlertTitle>
                <AlertDescription>{errorMsg}</AlertDescription>
              </Alert>
            )}
            
            <form onSubmit={handleLogin} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="email">E-mail</Label>
              <Input
                id="email"
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="seu@email.com"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="password">Senha</Label>
              <Input
                id="password"
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            <div className="flex justify-end">
              <Link to="/forgot-password" className="text-sm text-blue-600 hover:underline">
                Esqueci minha senha
              </Link>
            </div>
            <Button type="submit" className="w-full" disabled={isLoading}>
              {isLoading ? "Entrando..." : "Entrar"}
            </Button>
            <div className="text-center text-sm text-gray-500 mt-4">
              Não tem uma conta?{" "}
              <Link to="/register" className="text-blue-600 hover:underline">
                Crie grátis
              </Link>
            </div>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
};

export default Login;
