import { useState, useEffect } from 'react';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { Download, Filter, FileText, Calculator } from "lucide-react";
import { exportToExcel } from "@/lib/exportToExcel";
import { formatCurrency } from "@/lib/utils";

interface AggregatedData {
  filial_id: string;
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

const Comunicacoes = () => {
  const [activeTab, setActiveTab] = useState("dashboard");
  const [data, setData] = useState<AggregatedData[]>([]);
  const [loading, setLoading] = useState(true);

  const [error, setError] = useState<string | null>(null);

  // Fetch data from backend
  useEffect(() => {
    setLoading(true);
    fetch('/api/reports/comunicacoes')
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
  }, []);

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-xl animate-pulse">Carregando dados de Comunicações...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="container mx-auto p-6">
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
          <p className="font-bold">Erro ao carregar dados</p>
          <p>{error}</p>
          <p className="text-sm mt-2">Verifique se o backend está rodando em http://localhost:8080</p>
        </div>
      </div>
    );
  }

  if (data.length === 0) {
    return (
      <div className="container mx-auto p-6 space-y-8">
         <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
            <div>
               <h1 className="text-3xl font-bold text-gray-900">Serviços de Comunicação</h1>
               <p className="text-gray-500 mt-1">Análise de aquisições de serviços de comunicação e telecomunicação</p>
            </div>
         </div>
         <div className="text-center py-20 bg-gray-50 rounded-lg border border-dashed border-gray-300">
            <h2 className="text-xl font-semibold text-gray-700">Nenhum registro de Comunicação (D500) encontrado</h2>
            <p className="text-gray-500 mt-2">O banco de dados não possui registros D500.</p>
            <Button className="mt-4" onClick={() => window.location.reload()}>Tentar Novamente</Button>
         </div>
      </div>
    );
  }

  const handleExport = () => {
    const exportData = data.map(item => ({
      'Filial': item.filial_nome,
      'Mês/Ano': item.mes_ano,
      'Tipo': item.tipo,
      'Valor Total': item.valor,
      'PIS': item.pis,
      'COFINS': item.cofins,
      'ICMS': item.icms,
      'ICMS Projetado': item.vl_icms_projetado,
      'IBS Projetado': item.vl_ibs_projetado,
      'CBS Projetado': item.vl_cbs_projetado
    }));
    exportToExcel(exportData, 'relatorio_comunicacoes');
  };

  const chartData = data.reduce((acc: any[], curr) => {
    const existing = acc.find(item => item.name === curr.mes_ano);
    if (existing) {
      existing.Valor += curr.valor;
      existing.PIS += curr.pis;
      existing.COFINS += curr.cofins;
      existing.ICMS += curr.icms;
    } else {
      acc.push({
        name: curr.mes_ano,
        Valor: curr.valor,
        PIS: curr.pis,
        COFINS: curr.cofins,
        ICMS: curr.icms
      });
    }
    return acc;
  }, []);

  const projectionData = data.reduce((acc: any[], curr) => {
    const existing = acc.find(item => item.name === curr.mes_ano);
    if (existing) {
      existing.IBS += curr.vl_ibs_projetado;
      existing.CBS += curr.vl_cbs_projetado;
      existing.ICMS_PROJ += curr.vl_icms_projetado;
    } else {
      acc.push({
        name: curr.mes_ano,
        IBS: curr.vl_ibs_projetado,
        CBS: curr.vl_cbs_projetado,
        ICMS_PROJ: curr.vl_icms_projetado
      });
    }
    return acc;
  }, []);

  return (
    <div className="container mx-auto p-6 space-y-8">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
        <div>
          <h1 className="text-3xl font-bold text-gray-900">Serviços de Comunicação</h1>
          <p className="text-gray-500 mt-1">Análise de aquisições de serviços de comunicação e telecomunicação</p>
        </div>
        <div className="flex gap-2">
           <Button variant="outline" onClick={handleExport}>
             <Download className="mr-2 h-4 w-4" />
             Exportar Excel
           </Button>
        </div>
      </div>

      <Tabs defaultValue="dashboard" className="space-y-4" onValueChange={setActiveTab}>
        <TabsList>
          <TabsTrigger value="dashboard">Dashboard</TabsTrigger>
          <TabsTrigger value="detalhado">Visão Detalhada</TabsTrigger>
          <TabsTrigger value="projecao">Simulação Reforma Tributária</TabsTrigger>
        </TabsList>

        <TabsContent value="dashboard" className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Valor Total</CardTitle>
                <FileText className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  {formatCurrency(data.reduce((acc, curr) => acc + curr.valor, 0))}
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Total PIS</CardTitle>
                <Calculator className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  {formatCurrency(data.reduce((acc, curr) => acc + curr.pis, 0))}
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Total COFINS</CardTitle>
                <Calculator className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  {formatCurrency(data.reduce((acc, curr) => acc + curr.cofins, 0))}
                </div>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                <CardTitle className="text-sm font-medium">Total ICMS</CardTitle>
                <Calculator className="h-4 w-4 text-muted-foreground" />
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-bold">
                  {formatCurrency(data.reduce((acc, curr) => acc + curr.icms, 0))}
                </div>
              </CardContent>
            </Card>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>Evolução Mensal</CardTitle>
            </CardHeader>
            <CardContent className="pl-2">
              <ResponsiveContainer width="100%" height={350}>
                <BarChart data={chartData}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="name" />
                  <YAxis tickFormatter={(value) => `R$ ${value}`} />
                  <Tooltip formatter={(value: number) => formatCurrency(value)} />
                  <Legend />
                  <Bar dataKey="Valor" fill="#8884d8" name="Valor Total" />
                  <Bar dataKey="ICMS" fill="#82ca9d" name="ICMS" />
                  <Bar dataKey="PIS" fill="#ffc658" name="PIS" />
                  <Bar dataKey="COFINS" fill="#ff7300" name="COFINS" />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="detalhado">
          <Card>
            <CardHeader>
              <CardTitle>Detalhamento por Filial e Mês</CardTitle>
            </CardHeader>
            <CardContent>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Filial</TableHead>
                    <TableHead>Mês/Ano</TableHead>
                    <TableHead>Tipo</TableHead>
                    <TableHead className="text-right">Valor Total</TableHead>
                    <TableHead className="text-right">PIS</TableHead>
                    <TableHead className="text-right">COFINS</TableHead>
                    <TableHead className="text-right">ICMS</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {data.map((row, index) => (
                    <TableRow key={index}>
                      <TableCell>{row.filial_nome}</TableCell>
                      <TableCell>{row.mes_ano}</TableCell>
                      <TableCell>{row.tipo}</TableCell>
                      <TableCell className="text-right">{formatCurrency(row.valor)}</TableCell>
                      <TableCell className="text-right">{formatCurrency(row.pis)}</TableCell>
                      <TableCell className="text-right">{formatCurrency(row.cofins)}</TableCell>
                      <TableCell className="text-right">{formatCurrency(row.icms)}</TableCell>
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

           <Card>
            <CardHeader>
              <CardTitle>Comparativo Tributário (Projeção)</CardTitle>
            </CardHeader>
            <CardContent className="pl-2">
              <ResponsiveContainer width="100%" height={350}>
                <BarChart data={projectionData}>
                  <CartesianGrid strokeDasharray="3 3" />
                  <XAxis dataKey="name" />
                  <YAxis tickFormatter={(value) => `R$ ${value}`} />
                  <Tooltip formatter={(value: number) => formatCurrency(value)} />
                  <Legend />
                  <Bar dataKey="IBS" fill="#3b82f6" name="IBS Projetado" />
                  <Bar dataKey="CBS" fill="#a855f7" name="CBS Projetado" />
                  <Bar dataKey="ICMS_PROJ" fill="#22c55e" name="ICMS Reduzido" />
                </BarChart>
              </ResponsiveContainer>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
};

export default Comunicacoes;