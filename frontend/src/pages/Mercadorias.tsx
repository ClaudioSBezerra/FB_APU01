import { useState, useEffect, useCallback } from 'react';
import { useSearchParams, useLocation } from 'react-router-dom';
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
import { Download, Filter, FileText, Calculator, RefreshCcw } from "lucide-react";
import { exportToExcel } from "@/lib/exportToExcel";
import { formatCurrency } from "@/lib/utils";

interface AggregatedData {
  filial_nome: string;
  mes_ano: string;
  valor: number;
  icms: number;
  vl_icms_projetado: number;
  vl_ibs_projetado: number;
  vl_cbs_projetado: number;
  tipo: 'ENTRADA' | 'SAIDA';
  tipo_cfop?: string;
}

const Mercadorias = () => {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const initialTab = searchParams.get('tab') || "comercial";
  
  // Tax Reform Simulation Range: 2027-2033
  const currentYear = new Date().getFullYear();
  const [operationType, setOperationType] = useState(initialTab);
  const [activeTab, setActiveTab] = useState("dashboard");
  const [selectedYear, setSelectedYear] = useState<string>("2027");
  const [selectedFilial, setSelectedFilial] = useState<string>("all");
  const [selectedMonth, setSelectedMonth] = useState<string>("all");
  const [data, setData] = useState<AggregatedData[]>([]);
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const [error, setError] = useState<string | null>(null);

  // Fetch data from backend
  const fetchData = useCallback(() => {
    setLoading(true);
    fetch(`/api/reports/mercadorias?target_year=${selectedYear}&tipo_operacao=${operationType}`)
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
  }, [selectedYear, operationType]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  useEffect(() => {
    if (location.state?.refresh) {
      handleRefreshViews();
      // Clean state to avoid loops if user navigates back (though replaceState below handles it for current history entry)
      window.history.replaceState({}, document.title);
    }
  }, [location.state]);

  const handleRefreshViews = async () => {
    setIsRefreshing(true);
    try {
      const token = localStorage.getItem('token');
      // Use relative path to leverage proxy (Dev) or Nginx (Prod)
      const response = await fetch(`/api/admin/refresh-views`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`
        }
      });
      if (response.ok) {
        fetchData();
        alert('Dados atualizados com sucesso!');
      } else {
        const errText = await response.text();
        alert(`Erro ao atualizar dados: ${response.status} ${response.statusText}\n${errText}`);
      }
    } catch (e: any) {
      alert(`Erro de conexão ao atualizar dados: ${e.message}`);
    } finally {
      setIsRefreshing(false);
    }
  };

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

  // Helper to map operation types to user-friendly labels
  const getCategoryLabel = (tipo: string, tipoCfop?: string) => {
    if (!tipoCfop) return tipo === 'ENTRADA' ? 'Entrada (Outros)' : 'Saída (Outros)';
    
    // Detalha Entradas Revenda, Saida Revenda, Entrada Uso Consumo e Entrada Imobilizado
    if (tipo === 'ENTRADA' && tipoCfop === 'R') return 'Entrada Revenda';
    if (tipo === 'SAIDA' && tipoCfop === 'R') return 'Saída Revenda'; // Assuming Saída Revenda follows R
    if (tipo === 'SAIDA' && tipoCfop === 'S') return 'Saída Revenda'; // Often S is used for output in some contexts, but sticking to R based on user request. 
    // Correction: In previous context, S=Saída/Serviço. User asked for "Saida Revenda". If mapping matches, good.
    // Let's assume the backend filters (R, S) for Commercial. 
    // If it's S, maybe label as "Saída (Venda/Serviço)" or just "Saída Revenda" if that's the domain term.
    // User specifically asked: "Detalha Entradas Revenda, Saida Revenda..."
    // I will map 'S' to "Saída Venda" or similar if 'R' isn't the only one.
    if (tipo === 'SAIDA' && (tipoCfop === 'R' || tipoCfop === 'S')) return 'Saída Revenda'; 

    if (tipo === 'ENTRADA' && tipoCfop === 'C') return 'Entrada Uso Consumo';
    if (tipo === 'ENTRADA' && tipoCfop === 'A') return 'Entrada Imobilizado';
    
    // Fallback for others
    return `${tipo === 'ENTRADA' ? 'Entrada' : 'Saída'} (${tipoCfop})`;
  };

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

  const totals = filteredData.reduce((acc, item) => {
    if (item.tipo === 'SAIDA') {
      acc.saidas.valor += item.valor;
      acc.saidas.icms += item.icms;
      acc.saidas.icmsProj += item.vl_icms_projetado;
      acc.saidas.ibsProj += item.vl_ibs_projetado;
      acc.saidas.cbsProj += item.vl_cbs_projetado;
    } else {
      acc.entradas.valor += item.valor;
      acc.entradas.icms += item.icms;
      acc.entradas.icmsProj += item.vl_icms_projetado;
      acc.entradas.ibsProj += item.vl_ibs_projetado;
      acc.entradas.cbsProj += item.vl_cbs_projetado;
    }
    return acc;
  }, {
    saidas: { valor: 0, icms: 0, icmsProj: 0, ibsProj: 0, cbsProj: 0 },
    entradas: { valor: 0, icms: 0, icmsProj: 0, ibsProj: 0, cbsProj: 0 }
  });

  const totalDebitos = totals.saidas.icmsProj + totals.saidas.ibsProj + totals.saidas.cbsProj;
  const totalCreditos = totals.entradas.icmsProj + totals.entradas.ibsProj + totals.entradas.cbsProj;

  const handleExport = () => {
    const exportData = filteredData.map(item => {
      const totalAtual = (item.icms || 0);
      const baseIbsCbs = (item.valor || 0) - (item.vl_icms_projetado || 0);
      const totalReforma = (item.vl_icms_projetado || 0) + (item.vl_ibs_projetado || 0) + (item.vl_cbs_projetado || 0);
      const diferenca = totalAtual - totalReforma;

      return {
        'Filial': item.filial_nome,
        'Mês/Ano': item.mes_ano,
        'Detalhe': getCategoryLabel(item.tipo, item.tipo_cfop),
        'Valor': item.valor,
        'ICMS': item.icms,
        'ICMS Proj.': item.vl_icms_projetado,
        'Base IBS/CBS': baseIbsCbs,
        'IBS Proj.': item.vl_ibs_projetado,
        'CBS Proj.': item.vl_cbs_projetado,
        'Total Atual (ICMS)': totalAtual,
        'Total Reforma': totalReforma,
        'Diferença': diferenca
      };
    });
    exportToExcel(exportData, 'relatorio_mercadorias');
  };

  const chartData = filteredData.reduce((acc: any[], curr) => {
    const existing = acc.find(item => item.name === curr.mes_ano);
    if (existing) {
      if (curr.tipo === 'SAIDA') {
        existing.Saídas += curr.valor;
        existing.Impostos += (curr.vl_icms_projetado + curr.vl_ibs_projetado + curr.vl_cbs_projetado);
      } else {
        existing.Entradas += curr.valor;
        existing.Créditos += (curr.vl_icms_projetado + curr.vl_ibs_projetado + curr.vl_cbs_projetado);
      }
    } else {
      acc.push({
        name: curr.mes_ano,
        Saídas: curr.tipo === 'SAIDA' ? curr.valor : 0,
        Entradas: curr.tipo === 'ENTRADA' ? curr.valor : 0,
        Impostos: curr.tipo === 'SAIDA' ? (curr.vl_icms_projetado + curr.vl_ibs_projetado + curr.vl_cbs_projetado) : 0,
        Créditos: curr.tipo === 'ENTRADA' ? (curr.vl_icms_projetado + curr.vl_ibs_projetado + curr.vl_cbs_projetado) : 0,
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

        <Tabs value={operationType} onValueChange={setOperationType} className="w-[400px]">
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="comercial">Operações Comerciais</TabsTrigger>
            <TabsTrigger value="outras">Outras Operações</TabsTrigger>
          </TabsList>
        </Tabs>

        <div className="flex gap-2 items-center">
          <div className="flex items-center gap-2 bg-white p-1 rounded-md border">
            <span className="text-sm font-medium text-gray-700 ml-2">Simulação:</span>
            <Select value={selectedYear} onValueChange={setSelectedYear}>
              <SelectTrigger className="w-[100px] h-8 border-none focus:ring-0">
                <SelectValue placeholder="Ano" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="2027">2027</SelectItem>
                <SelectItem value="2028">2028</SelectItem>
                <SelectItem value="2029">2029</SelectItem>
                <SelectItem value="2030">2030</SelectItem>
                <SelectItem value="2031">2031</SelectItem>
                <SelectItem value="2032">2032</SelectItem>
                <SelectItem value="2033">2033</SelectItem>
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

          <Button 
            variant="outline" 
            size="sm" 
            onClick={handleRefreshViews} 
            disabled={isRefreshing}
            title="Recalcular Dados (Atualizar Views)"
            className={isRefreshing ? "opacity-50 cursor-not-allowed" : ""}
          >
            <RefreshCcw className={`w-4 h-4 mr-2 ${isRefreshing ? 'animate-spin' : ''}`} />
            {isRefreshing ? 'Atualizando...' : 'Atualizar Dados'}
          </Button>
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-lg font-medium">Total de Saídas</CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="space-y-1 text-sm">
              <div className="flex justify-between">
                <span className="text-gray-500">Valor:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.valor)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">ICMS:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.icms)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">ICMS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.icmsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">IBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.ibsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">CBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.cbsProj)}</span>
              </div>
              <div className="flex justify-between border-t pt-2 mt-2">
                <span className="font-bold text-gray-700">Total Débitos:</span>
                <span className="font-bold text-red-600">{formatCurrency(totalDebitos)}</span>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-lg font-medium">Total de Entradas</CardTitle>
            <FileText className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="space-y-1 text-sm">
              <div className="flex justify-between">
                <span className="text-gray-500">Valor:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.valor)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">ICMS:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.icms)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">ICMS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.icmsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">IBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.ibsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">CBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.cbsProj)}</span>
              </div>
              <div className="flex justify-between border-t pt-2 mt-2">
                <span className="font-bold text-gray-700">Total Créditos:</span>
                <span className="font-bold text-green-600">{formatCurrency(totalCreditos)}</span>
              </div>
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
              {chartData.length > 0 ? (
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
              ) : (
                <div className="flex items-center justify-center h-full text-gray-500">
                  Nenhum dado disponível para o período selecionado.
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="detalhado">
          <Card>
            <CardHeader>
              <CardTitle>Detalhamento por Filial</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="rounded-md border overflow-x-auto">
                <Table className="min-w-[1200px]">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-[100px]">Filial</TableHead>
                      <TableHead className="w-[80px]">Mês/Ano</TableHead>
                      <TableHead className="w-[150px]">Detalhe</TableHead>
                      <TableHead className="text-right">Valor</TableHead>
                      <TableHead className="text-right text-xs">ICMS</TableHead>
                      <TableHead className="text-right text-xs bg-blue-50">ICMS Proj.</TableHead>
                      <TableHead className="text-right text-xs bg-blue-50">Base IBS/CBS</TableHead>
                      <TableHead className="text-right text-xs bg-blue-50">IBS Proj.</TableHead>
                      <TableHead className="text-right text-xs bg-blue-50">CBS Proj.</TableHead>
                      <TableHead className="text-right font-bold border-l border-r bg-gray-50">Total Atual (ICMS)</TableHead>
                      <TableHead className="text-right font-bold bg-blue-100 border-r border-blue-200">Total Reforma</TableHead>
                      <TableHead className="text-right font-bold">Diferença</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {filteredData.map((row, i) => {
                      const totalAtual = (row.icms || 0);
                      const baseIbsCbs = (row.valor || 0) - (row.vl_icms_projetado || 0);
                      const totalReforma = (row.vl_icms_projetado || 0) + (row.vl_ibs_projetado || 0) + (row.vl_cbs_projetado || 0);
                      const diferenca = totalAtual - totalReforma;

                      return (
                        <TableRow key={i} className="hover:bg-gray-50">
                          <TableCell className="font-medium text-xs">{row.filial_nome}</TableCell>
                          <TableCell className="text-xs">{row.mes_ano}</TableCell>
                          <TableCell>
                            <span className={`px-2 py-1 rounded text-[11px] font-bold ${
                              row.tipo === 'SAIDA' ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'
                            }`}>
                              {getCategoryLabel(row.tipo, row.tipo_cfop)}
                            </span>
                          </TableCell>
                          <TableCell className="text-right text-xs">{formatCurrency(row.valor)}</TableCell>
                          <TableCell className="text-right text-xs text-gray-500">{formatCurrency(row.icms)}</TableCell>
                          <TableCell className="text-right text-xs text-blue-600 bg-blue-50">{formatCurrency(row.vl_icms_projetado)}</TableCell>
                          <TableCell className="text-right text-xs text-gray-400 bg-blue-50">{formatCurrency(baseIbsCbs)}</TableCell>
                          <TableCell className="text-right text-xs text-blue-600 bg-blue-50">{formatCurrency(row.vl_ibs_projetado)}</TableCell>
                          <TableCell className="text-right text-xs text-blue-600 bg-blue-50">{formatCurrency(row.vl_cbs_projetado)}</TableCell>
                          
                          <TableCell className="text-right text-xs font-bold border-l border-r bg-gray-50">{formatCurrency(totalAtual)}</TableCell>
                          <TableCell className="text-right text-xs font-bold bg-blue-100 text-blue-800 border-r border-blue-200">{formatCurrency(totalReforma)}</TableCell>
                          
                          <TableCell className={`text-right text-xs font-bold ${diferenca > 0 ? 'text-green-600' : 'text-red-600'}`}>
                            {formatCurrency(diferenca)}
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </div>
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