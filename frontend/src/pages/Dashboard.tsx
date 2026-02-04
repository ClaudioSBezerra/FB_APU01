import { useState, useEffect } from 'react';
import { useAuth } from '@/contexts/AuthContext';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Loader2, RefreshCw } from 'lucide-react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { formatCurrency } from '@/lib/utils'; // Assuming this exists, otherwise I'll define it locally

// Helper for currency if not available
const formatMoney = (value: number) => {
  return new Intl.NumberFormat('pt-BR', {
    style: 'currency',
    currency: 'BRL',
  }).format(value);
};

interface ProjectionPoint {
  ano: number;
  vl_icms: number;
  vl_ibs: number;
  vl_cbs: number;
  vl_saldo: number;
  vl_base: number;
  perc_reduc_icms: number;
}

export default function Dashboard() {
  const { token } = useAuth();
  const [data, setData] = useState<ProjectionPoint[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedMonth, setSelectedMonth] = useState<string>(''); // Format: MM/YYYY
  const [availableMonths, setAvailableMonths] = useState<string[]>([]);

  // Mock available months or fetch them (For now, I'll hardcode some recent ones or fetch from an endpoint if available)
  // Ideally we should fetch distinct months from API. 
  // For this MVP, I'll fetch the projection without filter first (aggregate all), 
  // and if the user wants to filter, we'd need a list.
  // I'll skip the dropdown population logic for now and just allow typing or default to "All".
  // Actually, let's just show "Consolidado (Todas Competências)" as default.

  const fetchData = async (mesAno?: string) => {
    setLoading(true);
    try {
      const query = mesAno ? `?mes_ano=${mesAno}` : '';
      const response = await fetch(`/api/dashboard/projection${query}`, {
        headers: {
          Authorization: `Bearer ${token || localStorage.getItem('token')}`
        }
      });
      if (response.ok) {
        const result = await response.json();
        setData(result);
      } else {
        console.error("Failed to fetch dashboard data");
      }
    } catch (error) {
      console.error("Error fetching dashboard data:", error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleFilterChange = (value: string) => {
    setSelectedMonth(value);
    fetchData(value === 'all' ? undefined : value);
  };

  return (
    <div className="space-y-6 p-6">
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-bold tracking-tight">Dashboard da Reforma Tributária</h1>
        <p className="text-muted-foreground">
          Projeção de impacto tributário (2027-2033) baseada nas operações consolidadas.
        </p>
      </div>

      <div className="flex items-center gap-4">
        {/* Simple Month Selector (In a real app, fetch distinct months) */}
        {/* <Select onValueChange={handleFilterChange} defaultValue="all">
          <SelectTrigger className="w-[180px]">
            <SelectValue placeholder="Selecione o Período" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Consolidado (Todos)</SelectItem>
            <SelectItem value="01/2025">Janeiro/2025</SelectItem>
            <SelectItem value="02/2025">Fevereiro/2025</SelectItem>
             Add more dynamically if possible 
          </SelectContent>
        </Select> */}
        
        <Button variant="outline" size="icon" onClick={() => fetchData(selectedMonth === 'all' ? undefined : selectedMonth)}>
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Evolução da Carga Tributária (Transição)</CardTitle>
          <CardDescription>
            Comparativo ICMS vs IBS/CBS ao longo do período de transição.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="h-[400px] w-full">
            {loading ? (
              <div className="flex h-full items-center justify-center">
                <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
              </div>
            ) : data.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%" minHeight={300}>
                <LineChart
                  data={data}
                  margin={{
                    top: 5,
                    right: 30,
                    left: 20,
                    bottom: 5,
                  }}
                >
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="ano" />
                  <YAxis tickFormatter={(val) => `R$ ${(val / 1000000).toFixed(1)}M`} />
                  <Tooltip 
                    formatter={(value: number) => formatMoney(value)}
                    labelFormatter={(label) => `Ano: ${label}`}
                  />
                  <Legend />
                  
                  {/* ICMS - Red */}
                  <Line 
                    type="monotone" 
                    dataKey="vl_icms" 
                    name="ICMS" 
                    stroke="#ef4444" 
                    strokeWidth={3}
                    dot={{ r: 4 }}
                    activeDot={{ r: 8 }} 
                  />
                  
                  {/* IBS - Brown (using hex for brown-ish) */}
                  <Line 
                    type="monotone" 
                    dataKey="vl_ibs" 
                    name="IBS" 
                    stroke="#855a30" 
                    strokeWidth={3}
                  />
                  
                  {/* CBS - Orange */}
                  <Line 
                    type="monotone" 
                    dataKey="vl_cbs" 
                    name="CBS" 
                    stroke="#f97316" 
                    strokeWidth={3}
                  />
                  
                  {/* Saldo a Pagar - Blue */}
                  <Line 
                    type="monotone" 
                    dataKey="vl_saldo" 
                    name="Saldo a Pagar" 
                    stroke="#3b82f6" 
                    strokeWidth={4}
                    strokeDasharray="5 5"
                  />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div className="flex h-full items-center justify-center text-muted-foreground">
                Nenhum dado disponível para projeção. Importe arquivos SPED primeiro.
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        {data.length > 0 && (
           <>
             <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Carga Tributária 2027</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{formatMoney(data.find(d => d.ano === 2027)?.vl_saldo || 0)}</div>
                  <p className="text-xs text-muted-foreground">Início da Transição</p>
                </CardContent>
             </Card>
             <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm font-medium">Carga Tributária 2033</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="text-2xl font-bold">{formatMoney(data.find(d => d.ano === 2033)?.vl_saldo || 0)}</div>
                  <p className="text-xs text-muted-foreground">Modelo Final (IBS/CBS Full)</p>
                </CardContent>
             </Card>
           </>
        )}
      </div>
    </div>
  );
}
