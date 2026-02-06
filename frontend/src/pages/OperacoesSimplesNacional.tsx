import { useState, useEffect, useCallback } from 'react';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { 
  BarChart, 
  Bar, 
  XAxis, 
  YAxis, 
  CartesianGrid, 
  Tooltip, 
  Legend, 
  ResponsiveContainer 
} from 'recharts';
import { Download, RefreshCcw, DollarSign, AlertCircle } from "lucide-react";
import { formatCurrency } from "@/lib/utils";
import { useAuth } from '@/contexts/AuthContext';

interface SimplesSupplierData {
  fornecedor_nome: string;
  fornecedor_cnpj: string;
  total_valor: number;
  total_icms: number;
  lost_ibs: number;
  lost_cbs: number;
  total_lost: number;
}

// Função para formatar CNPJ com máscara
const formatCNPJ = (cnpj: string) => {
  if (!cnpj || cnpj.length !== 14) return cnpj;
  return cnpj.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, "$1.$2.$3/$4-$5");
};

// Função para truncar nome
const truncateName = (name: string, maxLength: number = 50) => {
  if (name.length <= maxLength) return name;
  return name.substring(0, maxLength) + "...";
};

export default function OperacoesSimplesNacional() {
  const { token, companyId } = useAuth();
  const [data, setData] = useState<SimplesSupplierData[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedMonth, setSelectedMonth] = useState<string>("all");
  const [projectionYear, setProjectionYear] = useState<string>("2033");
  const [availableMonths, setAvailableMonths] = useState<string[]>([]);

  // Fetch data
  const fetchData = useCallback(async () => {
    if (!token || !companyId) return;

    setLoading(true);
    try {
      let url = `/api/dashboard/simples-nacional?projection_year=${projectionYear}`;
      if (selectedMonth && selectedMonth !== "all") {
        url += `&mes_ano=${selectedMonth}`;
      }

      const res = await fetch(url, {
        method: 'GET',
        headers: {
          'Authorization': `Bearer ${token}`,
          'X-Company-ID': companyId
        }
      });

      if (!res.ok) {
        const errText = await res.text();
        throw new Error(`Error ${res.status}: ${errText.slice(0, 100)}`);
      }

      const text = await res.text();
      const result = text ? JSON.parse(text) : [];
      
      setData(result || []);
    } catch (err) {
      console.error("Failed to fetch Simples Nacional data", err);
    } finally {
      setLoading(false);
    }
  }, [token, companyId, selectedMonth, projectionYear]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  // Extract available months (mock or future implementation)
  // For now we just use "all" or specific if passed.
  // Ideally we would fetch distinct months from API.

  const totalLost = data.reduce((acc, item) => acc + item.total_lost, 0);
  const totalValue = data.reduce((acc, item) => acc + item.total_valor, 0);

  // Top 10 for Chart
  const chartData = data.slice(0, 10).map(item => ({
    name: item.fornecedor_nome.substring(0, 20),
    Perdido: item.total_lost,
    Valor: item.total_valor
  }));

  const handleExport = () => {
    // Simple CSV export
    const headers = ["Fornecedor", "CNPJ", "Total Valor", "Total ICMS", "Perda IBS", "Perda CBS", "Perda Total"];
    const csvContent = [
      headers.join(";"),
      ...data.map(item => [
        `"${item.fornecedor_nome}"`,
        `"${item.fornecedor_cnpj}"`,
        item.total_valor.toFixed(2).replace(".", ","),
        item.total_icms.toFixed(2).replace(".", ","),
        item.lost_ibs.toFixed(2).replace(".", ","),
        item.lost_cbs.toFixed(2).replace(".", ","),
        item.total_lost.toFixed(2).replace(".", ",")
      ].join(";"))
    ].join("\n");

    const blob = new Blob([csvContent], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.setAttribute("download", "simples_nacional_perdas.csv");
    document.body.appendChild(link);
    link.click();
  };

  return (
    <div className="p-2 md:p-4 space-y-4 animate-in fade-in duration-500">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-2">
        <div>
          <h2 className="text-lg md:text-xl lg:text-2xl font-bold tracking-tight">Operações Simples Nacional</h2>
          <p className="text-[10px] md:text-sm text-muted-foreground mt-1">
            Análise de impacto e perdas de crédito (IBS/CBS) em fornecedores do Simples Nacional.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Select value={projectionYear} onValueChange={setProjectionYear}>
            <SelectTrigger className="w-[180px]">
              <SelectValue placeholder="Ano de Projeção" />
            </SelectTrigger>
            <SelectContent>
              {Array.from({ length: 7 }, (_, i) => 2027 + i).map((year) => (
                <SelectItem key={year} value={year.toString()}>
                  Projeção {year}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Button variant="outline" size="sm" onClick={fetchData} disabled={loading}>
            <RefreshCcw className={`mr-2 h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
            Atualizar
          </Button>
          <Button variant="outline" size="sm" onClick={handleExport} disabled={data.length === 0}>
            <Download className="mr-2 h-4 w-4" />
            Exportar CSV
          </Button>
        </div>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-base font-medium">Total Compras (Simples)</CardTitle>
            <DollarSign className="h-5 w-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-base font-bold">{formatCurrency(totalValue)}</div>
            <p className="text-xs text-muted-foreground">
              Base de cálculo potencial
            </p>
          </CardContent>
        </Card>
        
        <Card className="border-l-4 border-l-red-500">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-base font-medium">Crédito Perdido (Estimado)</CardTitle>
            <AlertCircle className="h-5 w-5 text-red-500" />
          </CardHeader>
          <CardContent>
            <div className="text-base font-bold text-red-600">{formatCurrency(totalLost)}</div>
            <p className="text-xs text-muted-foreground">
              Projeção IBS + CBS ({projectionYear})
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-base font-medium">Fornecedores Impactados</CardTitle>
            <AlertCircle className="h-5 w-5 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-base font-bold">{data.length}</div>
            <p className="text-xs text-muted-foreground">
              Fornecedores cadastrados no Simples
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Chart */}
      <Card>
        <CardHeader>
          <CardTitle>Top 10 Fornecedores - Maior Impacto de Perda</CardTitle>
          <CardDescription>
            Valores de crédito que deixariam de ser aproveitados (IBS + CBS)
          </CardDescription>
        </CardHeader>
        <CardContent className="h-[200px] md:h-[250px] w-full p-2">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={chartData} margin={{ top: 20, right: 30, left: 20, bottom: 5 }}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="name" fontSize={12} tickLine={false} axisLine={false} />
              <YAxis 
                fontSize={12} 
                tickLine={false} 
                axisLine={false}
                tickFormatter={(value) => `R$ ${(value / 1000).toFixed(0)}k`}
              />
              <Tooltip 
                formatter={(value: number) => formatCurrency(value)}
                cursor={{ fill: 'transparent' }}
              />
              <Legend />
              <Bar dataKey="Perdido" fill="#ef4444" radius={[4, 4, 0, 0]} name="Crédito Perdido" />
              <Bar dataKey="Valor" fill="#cbd5e1" radius={[4, 4, 0, 0]} name="Valor Total" />
            </BarChart>
          </ResponsiveContainer>
        </CardContent>
      </Card>

      {/* Detailed Table */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Detalhamento por Fornecedor</CardTitle>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-xs">Fornecedor</TableHead>
                <TableHead className="text-xs">CNPJ</TableHead>
                <TableHead className="text-xs text-right">Total</TableHead>
                <TableHead className="text-xs text-right">ICMS</TableHead>
                <TableHead className="text-xs text-right">IBS</TableHead>
                <TableHead className="text-xs text-right">CBS</TableHead>
                <TableHead className="text-xs text-right text-red-600">Perda</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                <TableRow>
                  <TableCell colSpan={7} className="text-center h-24 text-xs">Carregando...</TableCell>
                </TableRow>
              ) : data.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="text-center h-24 text-xs">Nenhum registro encontrado.</TableCell>
                </TableRow>
              ) : (
                data.map((item) => (
                  <TableRow key={item.fornecedor_cnpj}>
                    <TableCell className="font-medium text-xs truncate max-w-[150px]" title={item.fornecedor_nome}>
                      {truncateName(item.fornecedor_nome, 50)}
                    </TableCell>
                    <TableCell className="font-mono text-xs">{formatCNPJ(item.fornecedor_cnpj)}</TableCell>
                    <TableCell className="text-right text-xs">{formatCurrency(item.total_valor)}</TableCell>
                    <TableCell className="text-right text-xs">{formatCurrency(item.total_icms)}</TableCell>
                    <TableCell className="text-right text-xs text-muted-foreground">{formatCurrency(item.lost_ibs)}</TableCell>
                    <TableCell className="text-right text-xs text-muted-foreground">{formatCurrency(item.lost_cbs)}</TableCell>
                    <TableCell className="text-right font-bold text-xs text-red-600">{formatCurrency(item.total_lost)}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </CardContent>
      </Card>
    </div>
  );
}
