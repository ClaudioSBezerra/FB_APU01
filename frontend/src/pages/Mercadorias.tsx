import { useState, useEffect, useCallback } from 'react';
import { useSearchParams, useLocation } from 'react-router-dom';
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { 
  BarChart, 
  Bar, 
  XAxis, 
  YAxis, 
  CartesianGrid, 
  Tooltip, 
  Legend, 
  ResponsiveContainer,
  LineChart,
  Line,
  ReferenceLine
} from 'recharts';
import { Download, RefreshCcw, ArrowDownCircle, ArrowUpCircle, Scale } from "lucide-react";
import { exportToExcel } from "@/lib/exportToExcel";
import { formatCurrency } from "@/lib/utils";

interface AggregatedData {
  filial_nome: string;
  filial_cnpj: string;
  mes_ano: string;
  valor: number;
  icms: number;
  vl_icms_projetado: number;
  vl_ibs_projetado: number;
  vl_cbs_projetado: number;
  tipo: 'ENTRADA' | 'SAIDA';
  tipo_cfop?: string;
  origem?: string;
  tipo_operacao?: string;
}

interface TaxRate {
  ano: number;
  perc_ibs_uf: number;
  perc_ibs_mun: number;
  perc_cbs: number;
  perc_reduc_icms: number;
  perc_reduc_piscofins: number;
}

const Mercadorias = () => {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  
  // Tax Reform Simulation Range: 2027-2033
  const [selectedYear, setSelectedYear] = useState<string>("2027");
  const [selectedFilial, setSelectedFilial] = useState<string>("all");
  const [selectedMonth, setSelectedMonth] = useState<string>("all");
  const [selectedOperationType, setSelectedOperationType] = useState<string>("all");
  const [data, setData] = useState<AggregatedData[]>([]);
  const [taxRates, setTaxRates] = useState<TaxRate[]>([]);
  const [loading, setLoading] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const [error, setError] = useState<string | null>(null);

  // Fetch tax rates
  useEffect(() => {
    fetch("/api/config/aliquotas")
      .then((res) => res.json())
      .then((data) => setTaxRates(data || []))
      .catch((err) => console.error("Failed to fetch tax rates", err));
  }, []);

  // Fetch data from backend
  const fetchData = useCallback(() => {
    setLoading(true);
    // Request 'todos' to get all operations (Commercial + Others)
    fetch(`/api/reports/mercadorias?target_year=${selectedYear}&tipo_operacao=todos`)
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

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  useEffect(() => {
    if (location.state?.refresh) {
      handleRefreshViews();
      window.history.replaceState({}, document.title);
    }
  }, [location.state]);

  const handleRefreshViews = async () => {
    setIsRefreshing(true);
    try {
      const token = localStorage.getItem('token');
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
  const maskCnpj = (cnpj: string) => {
    if (!cnpj) return "";
    
    // Remove characters that are not digits
    const raw = cnpj.replace(/\D/g, '');
    
    // Check valid length
    if (raw.length !== 14) return cnpj;
    
    // Format: XX.XXX.XXX/YYYY-ZZ
    // We want to mask everything before the slash: **.***.***/YYYY-ZZ
    
    // Extract the suffix (YYYY-ZZ)
    const suffix = raw.slice(8); // 000128
    const formattedSuffix = `${suffix.slice(0, 4)}-${suffix.slice(4)}`;
    
    return `**.***.***/${formattedSuffix}`;
  };

  const getCategoryLabel = (tipo: string, tipoCfop?: string, origem?: string, tipoOperacao?: string) => {
    // Priority: use tipoOperacao from backend if available
    if (tipoOperacao) {
      switch (tipoOperacao) {
        // Entradas
        case 'Entrada_Revenda': return 'R - Entrada Revenda (C100/C190)';
        case 'Entradas_Frete': return 'R - Entrada Frete (D100)';
        case 'Entradas_Consumo': return 'C - Entrada Uso e Consumo';
        case 'Entradas_Imobilizado': return 'A - Entrada Ativo Imobilizado';
        case 'Entradas_Transferencia': return 'T - Entrada Transferência';
        case 'Entradas_Outros': return 'O - Outras Entradas';
        case 'Entradas_Energia_Agua': return 'Entrada Energia/Água (C500)';
        case 'Entradas_Comunicações': return 'Entrada Comunicações (D500)';
        case 'Entradas_NaoIdent': return 'Entrada Não Identificada';
        
        // Saídas
        case 'Saidas_Revenda': return 'R - Saída Revenda';
        case 'Saidas_Consumo': return 'C - Saída Uso e Consumo';
        case 'Saidas_Transferencia': return 'T - Saída Transferência';
        case 'Saidas_Imobilizado': return 'A - Saída Ativo Imobilizado';
        case 'Saidas_Outros': return 'O - Outras Saídas';
        case 'Saidas_Energia_Agua': return 'Saída Energia/Água (C600)';
        case 'Saidas_NaoIdent': return 'Saída Não Identificada';
        
        default: return tipoOperacao.replace(/_/g, ' ');
      }
    }

    if (!tipoCfop) return tipo === 'ENTRADA' ? 'Entrada (Outros)' : 'Saída (Outros)';
    
    // R de Entrada do bloco C100/C190
    if (tipo === 'ENTRADA' && tipoCfop === 'R' && origem === 'C100') return 'R de Entrada do bloco C100/C190';
    
    // R de Entradas Frete
    if (tipo === 'ENTRADA' && tipoCfop === 'R' && origem === 'D100') return 'R de Entradas Frete';

    // C Entradas Consumo
    if (tipo === 'ENTRADA' && tipoCfop === 'C') return 'C Entradas Consumo';

    // A Entradas Ativo
    if (tipo === 'ENTRADA' && tipoCfop === 'A') return 'A Entradas Ativo';

    // R de Saidas Bloco C100/C190
    if (tipo === 'SAIDA' && tipoCfop === 'R' && origem === 'C100') return 'R de Saidas Bloco C100/C190';
    
    // Fallback for others (keep existing logic for safety)
    if (tipo === 'ENTRADA' && tipoCfop === 'R') return 'Entrada Revenda';
    if (tipo === 'SAIDA' && (tipoCfop === 'R' || tipoCfop === 'S')) return 'Saída Revenda';
    if (tipo === 'ENTRADA' && tipoCfop === 'C') return 'Entrada Uso Consumo';
    if (tipo === 'ENTRADA' && tipoCfop === 'A') return 'Entrada Imobilizado';
    
    return `${tipo === 'ENTRADA' ? 'Entrada' : 'Saída'} (${tipoCfop})`;
  };

  const formatNumber = (value: number) => {
    return new Intl.NumberFormat('pt-BR', {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    }).format(value);
  };

  const uniqueFiliais = Array.from(new Set(data.map(item => JSON.stringify({ nome: item.filial_nome, cnpj: item.filial_cnpj }))))
    .map(str => JSON.parse(str))
    .sort((a, b) => a.nome.localeCompare(b.nome));

  const uniqueMonths = Array.from(new Set(data.map(item => item.mes_ano))).sort((a, b) => {
    const [ma, ya] = a.split('/').map(Number);
    const [mb, yb] = b.split('/').map(Number);
    return ya - yb || ma - mb;
  });
  
  const uniqueOperationTypes = Array.from(new Set(data.map(item => 
    getCategoryLabel(item.tipo, item.tipo_cfop, item.origem, item.tipo_operacao)
  ))).sort();

  // Filter data
  const filteredData = data.filter(item => {
    const matchFilial = selectedFilial === "all" || item.filial_nome === selectedFilial;
    const matchMonth = selectedMonth === "all" || item.mes_ano === selectedMonth;
    const matchOperation = selectedOperationType === "all" || 
      getCategoryLabel(item.tipo, item.tipo_cfop, item.origem, item.tipo_operacao) === selectedOperationType;
    return matchFilial && matchMonth && matchOperation;
  });

  const totals = filteredData.reduce((acc, item) => {
    // Check if item is taxable for IBS/CBS (exclude T and O)
    const isTaxable = item.tipo_cfop !== 'T' && item.tipo_cfop !== 'O';

    if (item.tipo === 'SAIDA') {
      acc.saidas.valor += item.valor;
      acc.saidas.icms += item.icms;
      acc.saidas.icmsProj += item.vl_icms_projetado;
      acc.saidas.ibsProj += item.vl_ibs_projetado;
      acc.saidas.cbsProj += item.vl_cbs_projetado;
      
      if (isTaxable) {
        acc.saidas.valorTaxable += item.valor;
        acc.saidas.icmsTaxable += item.icms;
      }
    } else {
      acc.entradas.valor += item.valor;
      acc.entradas.icms += item.icms;
      acc.entradas.icmsProj += item.vl_icms_projetado;
      acc.entradas.ibsProj += item.vl_ibs_projetado;
      acc.entradas.cbsProj += item.vl_cbs_projetado;

      if (isTaxable) {
        acc.entradas.valorTaxable += item.valor;
        acc.entradas.icmsTaxable += item.icms;
      }
    }
    return acc;
  }, {
    saidas: { valor: 0, icms: 0, icmsProj: 0, ibsProj: 0, cbsProj: 0, valorTaxable: 0, icmsTaxable: 0 },
    entradas: { valor: 0, icms: 0, icmsProj: 0, ibsProj: 0, cbsProj: 0, valorTaxable: 0, icmsTaxable: 0 }
  });

  // Projection Logic for 2027-2033 (based on currently filtered totals)
  const projectionData = taxRates
    .filter(r => r.ano >= 2027 && r.ano <= 2033)
    .sort((a, b) => a.ano - b.ano)
    .map(rate => {
      const reductionFactor = (1 - (rate.perc_reduc_icms / 100.0));
      const ibsRate = (rate.perc_ibs_uf + rate.perc_ibs_mun) / 100.0;
      const cbsRate = rate.perc_cbs / 100.0;

      // Saídas
      // ICMS Proj is calculated on TOTAL ICMS (including T/O?) 
      // User said: "operações com TIPO "T" ... e "O" ... não terão incidência de IBS e CBS"
      // Assuming ICMS reduction applies to ALL ICMS (or just taxable?).
      // Usually ICMS reduction is general. But IBS/CBS only applies to taxable base.
      // So we use totals.saidas.icms for ICMS Projection (display purpose)
      const icmsProjSaida = totals.saidas.icms * reductionFactor;
      
      // For IBS/CBS Base, we use Taxable Value - Taxable ICMS Projected
      const icmsProjSaidaTaxable = totals.saidas.icmsTaxable * reductionFactor;
      const baseIbsCbsSaida = totals.saidas.valorTaxable - icmsProjSaidaTaxable;
      
      const ibsSaida = baseIbsCbsSaida * ibsRate;
      const cbsSaida = baseIbsCbsSaida * cbsRate;
      const totalDebitosAno = icmsProjSaida + ibsSaida + cbsSaida;

      // Entradas
      const icmsProjEntrada = totals.entradas.icms * reductionFactor;
      
      const icmsProjEntradaTaxable = totals.entradas.icmsTaxable * reductionFactor;
      const baseIbsCbsEntrada = totals.entradas.valorTaxable - icmsProjEntradaTaxable;

      const ibsEntrada = baseIbsCbsEntrada * ibsRate;
      const cbsEntrada = baseIbsCbsEntrada * cbsRate;
      const totalCreditosAno = icmsProjEntrada + ibsEntrada + cbsEntrada;

      return {
        name: rate.ano.toString(),
        SaldoReforma: totalDebitosAno - totalCreditosAno,
        Debitos: totalDebitosAno,
        Creditos: totalCreditosAno
      };
    });

  const totalDebitos = totals.saidas.icmsProj + totals.saidas.ibsProj + totals.saidas.cbsProj;
  const totalCreditos = totals.entradas.icmsProj + totals.entradas.ibsProj + totals.entradas.cbsProj;
  const saldoReforma = totalDebitos - totalCreditos;

  const totalDebitosAtual = totals.saidas.icms;
  const totalCreditosAtual = totals.entradas.icms;
  const saldoAtual = totalDebitosAtual - totalCreditosAtual;

  const handleExport = () => {
    const exportData = filteredData.map(item => {
      const totalAtual = (item.icms || 0);
      const baseIbsCbs = (item.valor || 0) - (item.vl_icms_projetado || 0);
      const totalReforma = (item.vl_icms_projetado || 0) + (item.vl_ibs_projetado || 0) + (item.vl_cbs_projetado || 0);
      const diferenca = totalAtual - totalReforma;

      return {
        'Filial': item.filial_nome,
        'Mês/Ano': item.mes_ano,
        'Detalhe': getCategoryLabel(item.tipo, item.tipo_cfop, item.origem, item.tipo_operacao),
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
    exportToExcel(exportData, 'relatorio_mercadorias_detalhado');
  };

  // Chart Data Preparation - Net Balance over time
  const chartData = filteredData.reduce((acc: any[], curr) => {
    const existing = acc.find(item => item.name === curr.mes_ano);
    
    // Tax Reform Values
    const taxValue = curr.vl_icms_projetado + curr.vl_ibs_projetado + curr.vl_cbs_projetado;
    
    // Current Values
    const currentTaxValue = curr.icms;

    if (existing) {
      if (curr.tipo === 'SAIDA') {
        existing.Debitos += taxValue;
        existing.DebitosAtual += currentTaxValue;
      } else {
        existing.Creditos += taxValue;
        existing.CreditosAtual += currentTaxValue;
      }
      existing.Saldo = existing.Debitos - existing.Creditos;
      existing.SaldoAtual = existing.DebitosAtual - existing.CreditosAtual;
    } else {
      const isSaida = curr.tipo === 'SAIDA';
      const debitos = isSaida ? taxValue : 0;
      const creditos = isSaida ? 0 : taxValue;
      const debitosAtual = isSaida ? currentTaxValue : 0;
      const creditosAtual = isSaida ? 0 : currentTaxValue;

      acc.push({
        name: curr.mes_ano,
        Debitos: debitos,
        Creditos: creditos,
        Saldo: debitos - creditos,
        DebitosAtual: debitosAtual,
        CreditosAtual: creditosAtual,
        SaldoAtual: debitosAtual - creditosAtual
      });
    }
    return acc;
  }, []).sort((a, b) => {
     const [ma, ya] = a.name.split('/').map(Number);
     const [mb, yb] = b.name.split('/').map(Number);
     return ya - yb || ma - mb;
  });

  return (
    <div className="container mx-auto p-6 space-y-8">
      <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4">
        <div>
          <h1 className="text-xl font-bold text-gray-900">Comparativo de impostos atuais<br/>com IBS e CBS</h1>
        </div>

        <div className="flex gap-2 items-center flex-wrap">
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
            {isRefreshing ? 'Atualizando...' : 'Atualizar'}
          </Button>
        </div>
      </div>

      {/* Cards de Totais */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* Total Saídas */}
        <Card className="border-l-4 border-l-red-500">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-base font-medium text-red-700">Total de Saídas</CardTitle>
            <ArrowUpCircle className="h-5 w-5 text-red-500" />
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-xs">
              <div className="flex justify-between">
                <span className="text-gray-500">Valor de Saídas:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.valor)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">Valor de ICMS:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.icms)}</span>
              </div>
              
              <div className="my-2 border-t border-dashed border-gray-200"></div>
              
              <div className="flex justify-between">
                <span className="text-gray-500">Valor ICMS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.icmsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">Valor IBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.ibsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">Valor CBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.saidas.cbsProj)}</span>
              </div>

              <div className="flex justify-between pt-2 border-t mt-2">
                <span className="text-red-700 font-bold">Total Débitos:</span>
                <span className="font-bold text-red-600 text-base">{formatCurrency(totalDebitos)}</span>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Total Entradas */}
        <Card className="border-l-4 border-l-green-500">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-base font-medium text-green-700">Total de Entradas</CardTitle>
            <ArrowDownCircle className="h-5 w-5 text-green-500" />
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-xs">
              <div className="flex justify-between">
                <span className="text-gray-500">Valor de Entradas:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.valor)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">Valor de ICMS:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.icms)}</span>
              </div>

              <div className="my-2 border-t border-dashed border-gray-200"></div>

              <div className="flex justify-between">
                <span className="text-gray-500">Valor ICMS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.icmsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">Valor IBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.ibsProj)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">Valor CBS Proj.:</span>
                <span className="font-medium">{formatCurrency(totals.entradas.cbsProj)}</span>
              </div>

              <div className="flex justify-between pt-2 border-t mt-2">
                <span className="text-green-700 font-bold">Total Créditos:</span>
                <span className="font-bold text-green-600 text-base">{formatCurrency(totalCreditos)}</span>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Apuração Projetada */}
        <Card className="border-l-4 border-l-blue-500 bg-blue-50/30">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-base font-medium text-blue-800">Apuração</CardTitle>
            <Scale className="h-5 w-5 text-blue-600" />
          </CardHeader>
          <CardContent>
            <div className="space-y-3 text-xs">
               <div className="flex justify-between items-center">
                <span className="text-gray-600">Total Débitos:</span>
                <span className="font-medium text-red-600">
                  {formatCurrency(totalDebitos)}
                </span>
              </div>
              <div className="flex justify-between items-center">
                <span className="text-gray-600">Total Créditos:</span>
                <span className="font-medium text-green-600">
                  - {formatCurrency(totalCreditos)}
                </span>
              </div>
              
              <div className="border-t border-blue-300 my-2"></div>

              <div className="flex justify-between items-center">
                <span className="text-blue-900 font-bold text-sm">Resultado:</span>
                <span className={`font-bold text-xl ${saldoReforma > 0 ? 'text-red-600' : 'text-green-600'}`}>
                  {formatCurrency(saldoReforma)}
                </span>
              </div>
              <div className="text-xs text-blue-500 text-right font-medium">
                {saldoReforma > 0 ? "Imposto a Pagar" : "Crédito Acumulado"}
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Gráfico de Projeção 2027-2033 */}
      <Card>
        <CardHeader>
          <CardTitle>Projeção do Saldo de Imposto (2027-2033)</CardTitle>
          <div className="text-sm text-gray-500 font-normal">
            Projeção baseada nos totais filtrados e na tabela de alíquotas de transição.
          </div>
        </CardHeader>
        <CardContent className="h-[350px]">
          {projectionData.length > 0 ? (
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={projectionData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="name" />
                <YAxis />
                <Tooltip formatter={(value) => formatCurrency(Number(value))} />
                <Legend />
                <ReferenceLine y={0} stroke="#000" />
                <Line type="monotone" dataKey="Debitos" name="Total Débitos (Projetado)" stroke="#dc2626" strokeDasharray="3 3" />
                <Line type="monotone" dataKey="Creditos" name="Total Créditos (Projetado)" stroke="#16a34a" strokeDasharray="3 3" />
                <Line type="monotone" dataKey="SaldoReforma" name="Saldo a Pagar (Projetado)" stroke="#2563eb" strokeWidth={3} dot={{ r: 6 }} />
              </LineChart>
            </ResponsiveContainer>
          ) : (
            <div className="flex items-center justify-center h-full text-gray-500">
              Não foi possível gerar a projeção. Verifique se a tabela de alíquotas está configurada.
            </div>
          )}
        </CardContent>
      </Card>

      {/* Tabela Detalhada */}
      <div className="flex gap-2 items-center flex-wrap mb-1">
        <Select value={selectedFilial} onValueChange={setSelectedFilial}>
          <SelectTrigger className="w-[300px] h-8 bg-white">
            <SelectValue placeholder="Filial: Todas" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Filial: Todas</SelectItem>
            {uniqueFiliais.map((f) => (
              <SelectItem key={f.nome} value={f.nome}>
                {maskCnpj(f.cnpj)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={selectedMonth} onValueChange={setSelectedMonth}>
          <SelectTrigger className="w-[130px] h-8 bg-white">
            <SelectValue placeholder="Mês: Todos" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Mês: Todos</SelectItem>
            {uniqueMonths.map((m) => (
              <SelectItem key={m} value={m}>{m}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={selectedOperationType} onValueChange={setSelectedOperationType}>
          <SelectTrigger className="w-[280px] h-8 bg-white">
            <SelectValue placeholder="Tipo: Todos" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">Tipo: Todos</SelectItem>
            {uniqueOperationTypes.map((t) => (
              <SelectItem key={t} value={t}>{t}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Detalhamento por Filial e Operação</CardTitle>
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
                      <TableCell className="font-medium text-[10px]" title={row.filial_nome}>{maskCnpj(row.filial_cnpj)}</TableCell>
                      <TableCell className="text-[10px]">{row.mes_ano}</TableCell>
                      <TableCell>
                        <span className={`px-2 py-1 rounded text-[10px] font-bold ${
                          row.tipo === 'SAIDA' ? 'bg-red-100 text-red-700' : 'bg-green-100 text-green-700'
                        }`}>
                          {getCategoryLabel(row.tipo, row.tipo_cfop, row.origem, row.tipo_operacao)}
                        </span>
                      </TableCell>
                      <TableCell className="text-right text-[10px]">{formatNumber(row.valor)}</TableCell>
                      <TableCell className="text-right text-[10px] text-gray-500">{formatNumber(row.icms)}</TableCell>
                      <TableCell className="text-right text-[10px] text-blue-600 bg-blue-50">{formatNumber(row.vl_icms_projetado)}</TableCell>
                      <TableCell className="text-right text-[10px] text-gray-400 bg-blue-50">{formatNumber(baseIbsCbs)}</TableCell>
                      <TableCell className="text-right text-[10px] text-blue-600 bg-blue-50">{formatNumber(row.vl_ibs_projetado)}</TableCell>
                      <TableCell className="text-right text-[10px] text-blue-600 bg-blue-50">{formatNumber(row.vl_cbs_projetado)}</TableCell>
                      
                      <TableCell className="text-right text-[10px] font-bold border-l border-r bg-gray-50">{formatNumber(totalAtual)}</TableCell>
                      <TableCell className="text-right text-[10px] font-bold bg-blue-100 text-blue-800 border-r border-blue-200">{formatNumber(totalReforma)}</TableCell>
                      
                      <TableCell className={`text-right text-[10px] font-bold ${diferenca > 0 ? 'text-green-600' : 'text-red-600'}`}>
                        {formatNumber(diferenca)}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        </CardContent>
      </Card>
    </div>
  );
};

export default Mercadorias;
