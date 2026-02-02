import React, { createContext, useContext, useState, useEffect } from 'react';

interface User {
  id: string;
  email: string;
  full_name: string;
  trial_ends_at: string;
  role?: string;
}

interface AuthContextType {
  user: User | null;
  token: string | null;
  environment: string | null;
  group: string | null;
  company: string | null;
  companyId: string | null;
  cnpj: string | null;
  login: (data: any) => void;
  logout: () => void;
  isAuthenticated: boolean;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider = ({ children }: { children: React.ReactNode }) => {
  const [user, setUser] = useState<User | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [environment, setEnvironment] = useState<string | null>(null);
  const [group, setGroup] = useState<string | null>(null);
  const [company, setCompany] = useState<string | null>(null);
  const [companyId, setCompanyId] = useState<string | null>(null);
  const [cnpj, setCnpj] = useState<string | null>(null);

  useEffect(() => {
    // Restore session from localStorage
    const storedToken = localStorage.getItem('token');
    const storedUser = localStorage.getItem('user');
    const storedEnv = localStorage.getItem('environment');
    const storedGroup = localStorage.getItem('group');
    const storedCompany = localStorage.getItem('company');
    const storedCompanyId = localStorage.getItem('companyId');
    const storedCnpj = localStorage.getItem('cnpj');

    if (storedToken && storedUser) {
      setToken(storedToken);
      setUser(JSON.parse(storedUser));
      setEnvironment(storedEnv);
      setGroup(storedGroup);
      setCompany(storedCompany);
      setCompanyId(storedCompanyId);
      setCnpj(storedCnpj);
    }
  }, []);

  const login = (data: any) => {
    setToken(data.token);
    setUser(data.user);
    setEnvironment(data.environment_name);
    setGroup(data.group_name);
    setCompany(data.company_name);
    setCompanyId(data.company_id);
    setCnpj(data.cnpj);

    localStorage.setItem('token', data.token);
    localStorage.setItem('user', JSON.stringify(data.user));
    localStorage.setItem('environment', data.environment_name || '');
    localStorage.setItem('group', data.group_name || '');
    localStorage.setItem('company', data.company_name || '');
    localStorage.setItem('companyId', data.company_id || '');
    localStorage.setItem('cnpj', data.cnpj || '');
  };

  const logout = () => {
    setUser(null);
    setToken(null);
    setEnvironment(null);
    setGroup(null);
    setCompany(null);
    setCompanyId(null);
    setCnpj(null);
    localStorage.clear();
    window.location.href = '/login';
  };

  return (
    <AuthContext.Provider value={{ 
      user, 
      token, 
      environment, 
      group, 
      company,
      companyId,
      cnpj,
      login, 
      logout,
      isAuthenticated: !!user 
    }}>
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};
