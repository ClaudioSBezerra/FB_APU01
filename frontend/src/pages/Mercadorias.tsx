import { useState, useEffect } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { Download, Filter, FileText, Calculator } from "lucide-react";
import { exportToExcel } from "@/lib/exportToExcel";
import { formatCurrency } from "@/lib/utils";

interface AggregatedData {
  filial_nome: string;
  mes_ano: string;
  valor: number;
  pis: number;
  cofins: number;
  icms: number;
  vl_icms_projetado: number;
  vl_ibs_projetado: number;
  vl_cbs_projetado: number;
  tipo: 'ENTRADA' | 'SAIDA';
}

const Mercadorias = () => {
  const [activeTab, setActiveTab] = useState("dashboard");
  const [selectedYear, setSelectedYear] = useState<string>("2027");
  const [selectedFilial, setSelectedFilial] = useState<string>("all");
  const [selectedMonth, setSelectedMonth] = useState<string>("all");
  const [data, setData] = useState<AggregatedData[]>([]);
  const [loading, setLoading] = useState(true);

  const [error, setError] = useState<string | null>(null);

  // Fetch data from backend
  useEffect(() => {
    setLoading(true);
    fetch(`/api/reports/mercadorias?target_year=${selectedYear}`)
      .then(res => {
        if (!res.ok) throw new Error(`Erro na API: ${res.status} ${res.statusText}`);
        return res.json();
      })
      .then(data => {
        console.log("Dados recebidos:", data);
        setData(data || []);
        setLoading(false);
      })
      .catch(err => {
        console.error("Failed to fetch data:", err);
        setError(err.message);
        setLoading(false);
      });
  }, [selectedYear]);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-xl animate-pulse">Carregando dados fiscais...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="container mx-auto p-6">
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
          <p className="font-bold">Erro ao carregar dados</p>
          <p>{error}</p>
          <p className="text-sm mt-2">Verifique se o backend está rodando em http://localhost:8081</p>
        </div>
      </div>
    );
  }

  if (data.length === 0) {
    return (
      <div className="container mx-auto p-6 space-y-8">
         <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
            <h1 className="text-3xl font-bold text-gray-900">Mercadorias</h1>
         </div>
         <div className="text-center py-20 bg-gray-50 rounded-lg border border-dashed border-gray-300">
            <h2 className="text-xl font-semibold text-gray-700">Nenhum dado encontrado</h2>
            <p className="text-gray-500 mt-2">O banco de dados está vazio ou a importação falhou.</p>
            <Button className="mt-4" onClick={() => window.location.reload()}>Tentar Novamente</Button>
         </div>
      </div>
    );
  }

  // Extract unique values for filters
  const uniqueFiliais = Array.from(new Set(data.map(item => item.filial_nome))).sort();
  const uniqueMonths = Array.from(new Set(data.map(item => item.mes_ano))).sort((a, b) => {
    const [ma, ya] = a.split('/').map(Number);
    const [mb, yb] = b.split('/').map(Number);
    return ya - yb || ma - mb;
  });

  // Filter data
  const filteredData = data.filter(item => {
    const matchFilial = selectedFilial === "all" || item.filial_nome === selectedFilial;
    const matchMonth = selectedMonth === "all" || item.mes_ano === selectedMonth;
    return matchFilial && matchMonth;
  });

  const handleExport = () => {
    const exportData = filteredData.map(item => ({
      'Filial': item.filial_nome,
      'Mês/Ano': item.mes_ano,
      'Tipo': item.tipo,
      'Valor Total': item.valor,
      'PIS/COFINS': item.pis + item.cofins,
      'ICMS': item.icms,
      'ICMS Projetado': item.vl_icms_projetado,
      'IBS Projetado': item.vl_ibs_projetado,
      'CBS Projetado': item.vl_cbs_projetado
    }));
    exportToExcel(exportData, 'relatorio_mercadorias');
  };

  const chartData = filteredData.reduce((acc: any[], curr) => {
    const existing = acc.find(item => item.name === curr.mes_ano);
    if (existing) {
      if (curr.tipo === 'SAIDA') {
        existing.Saídas += curr.valor;
        existing.Impostos += (curr.pis + curr.cofins);
      } else {
        existing.Entradas += curr.valor;
        existing.Créditos += (curr.pis + curr.cofins);
      }
    } else {
      acc.push({
        name: curr.mes_ano,
        Saídas: curr.tipo === 'SAIDA' ? curr.valor : 0,
        Entradas: curr.tipo === 'ENTRADA' ? curr.valor : 0,
        Impostos: curr.tipo === 'SAIDA' ? (curr.pis + curr.cofins) : 0,
        Créditos: curr.tipo === 'ENTRADA' ? (curr.pis + curr.cofins) : 0,
      });
    }
    return acc;
  }, []);

  return (
    <div className="container mx-auto p-6 space-y-8">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Mercadorias</h1>
          <p className="text-gray-500 mt-1">Análise detalhada de movimentação fiscal</p>
        </div>
        <div className="flex gap-2 items-center">
          <div className="flex items-center gap-2 bg-white p-1 rounded-md border">
            <span className="text-sm font-medium text-gray-700 ml-2">Simulação:</span>
            <Select value={selectedYear} onValueChange={setSelectedYear}>
              <SelectTrigger className="w-[80px] h-8 border-none focus:ring-0">
                <SelectValue placeholder="Ano" />
              </SelectTrigger>
              <SelectContent>
                {[2027, 2028, 2029, 2030, 2031, 2032, 2033].map((year) => (
                  <SelectItem key={year} value={year.toString()}>
                    {year}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <Select value={selectedFilial} onValueChange={setSelectedFilial}>
            <SelectTrigger className="w-[180px] h-8">
              <SelectValue placeholder="Filial: Todas" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Filial: Todas</SelectItem>
              {uniqueFiliais.map((f) => (
                <SelectItem key={f} value={f}>{f}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select value={selectedMonth} onValueChange={setSelectedMonth}>
            <SelectTrigger className="w-[130px] h-8">
              <SelectValue placeholder="Mês: Todos" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">Mês: Todos</SelectItem>
              {uniqueMonths.map((m) => (
                <SelectItem key={m} value={m}>{m}</SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Button variant="default" size="sm" onClick={handleExport}>
            <Download className="w-4 h-4 mr-2" />
            Exportar
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total de Saídas</CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {formatCurrency(filteredData.filter(d => d.tipo === 'SAIDA').reduce((sum, item) => sum + item.valor, 0))}
            </div></div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total de Entradas</CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {formatCurrency(data.filter(d => d.tipo === 'ENTRADA').reduce((sum, item) => sum + item.valor, 0))}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Saldo de PIS/COFINS</CardTitle>
            <Calculator className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {formatCurrency(data.reduce((sum, item) => {
                const total = item.pis + item.cofins;
                return item.tipo === 'SAIDA' ? sum - total : sum + total;
              }, 0))}
            </div>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="dashboard" className="w-full" onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="dashboard">Dashboard</TabsTrigger>
          <TabsTrigger value="detalhado">Relatório Detalhado</TabsTrigger>
          <TabsTrigger value="projecao">Simulação Reforma Tributária</TabsTrigger>
        </TabsList>

        <TabsContent value="dashboard" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Projeção de Movimentação ({selectedYear})</CardTitle>
            </CardHeader>
            <CardContent className="h-[400px]">
              <ResponsiveContainer width="100%" height="100%">
                <BarChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="name" />
                  <YAxis />
                  <Tooltip formatter={(value) => formatCurrency(Number(value))} />
                  <Legend />
                  <Bar dataKey="Saídas" fill="#ef4444" />
                  <Bar dataKey="Entradas" fill="#22c55e" />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="detalhado">
          <Card>
            <CardHeader>
              <CardTitle>Detalhamento por Filial</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Filial</TableHead>
                    <TableHead>Mês/Ano</TableHead>
                    <TableHead>Tipo</TableHead>
                    <TableHead className="text-right">Valor Contábil</TableHead>
                    <TableHead className="text-right">PIS/COFINS</TableHead>
                    <TableHead className="text-right">ICMS Atual</TableHead>
                    <TableHead className="text-right bg-blue-50">ICMS Projetado</TableHead>
                    <TableHead className="text-right bg-blue-50">IBS</TableHead>
                    <TableHead className="text-right bg-blue-50">CBS</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.map((row, i) => (
                    <TableRow key={i}>
                      <TableCell>{row.filial_nome}</TableCell>
                      <TableCell>{row.mes_ano}</TableCell>
                      <TableCell>
                        <span className={`px-2 py-1 rounded-full text-xs font-semibold ${
                          row.tipo === 'SAIDA' ? 'bg-red-100 text-red-800' : 'bg-green-100 text-green-800'
                        }`}>
                          {row.tipo}
                        </span>
                      </TableCell>
                      <TableCell className="text-right">{formatCurrency(row.valor)}</TableCell>
                      <TableCell className="text-right">{formatCurrency(row.pis + row.cofins)}</TableCell>
                      <TableCell className="text-right">{formatCurrency(row.icms)}</TableCell>
                      <TableCell className="text-right bg-blue-50 font-medium text-blue-700">{formatCurrency(row.vl_icms_projetado)}</TableCell>
                      <TableCell className="text-right bg-blue-50 font-medium text-blue-700">{formatCurrency(row.vl_ibs_projetado)}</TableCell>
                      <TableCell className="text-right bg-blue-50 font-medium text-blue-700">{formatCurrency(row.vl_cbs_projetado)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="projecao">
           <div className="grid gap-4 md:grid-cols-3 mb-6">
              <Card>
                 <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-medium">IBS Projetado</CardTitle>
                 </CardHeader>
                 <CardContent>
                    <div className="text-2xl font-bold text-blue-600">
                       {formatCurrency(data.reduce((acc, curr) => acc + curr.vl_ibs_projetado, 0))}
                    </div>
                 </CardContent>
              </Card>
              <Card>
                 <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-medium">CBS Projetado</CardTitle>
                 </CardHeader>
                 <CardContent>
                    <div className="text-2xl font-bold text-purple-600">
                       {formatCurrency(data.reduce((acc, curr) => acc + curr.vl_cbs_projetado, 0))}
                    </div>
                 </CardContent>
              </Card>
              <Card>
                 <CardHeader className="pb-2">
                    <CardTitle className="text-sm font-medium">ICMS (Reduzido)</CardTitle>
                 </CardHeader>
                 <CardContent>
                    <div className="text-2xl font-bold text-green-600">
                       {formatCurrency(data.reduce((acc, curr) => acc + curr.vl_icms_projetado, 0))}
                    </div>
                 </CardContent>
              </Card>
           </div>
        </TabsContent>
      </Tabs>
    </div>
  );
};

export default Mercadorias;