import { useState, useEffect } from 'react';
import { useAuth } from '@/contexts/AuthContext';
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Loader2, RefreshCw, Sparkles, FileText, TrendingDown, TrendingUp, Minus } from 'lucide-react';
import { Badge } from '@/components/ui/badge';

interface SummaryData {
  narrativa: string;
  dados: {
    company_name: string;
    cnpj: string;
    periodo: string;
    faturamento_bruto: number;
    total_entradas: number;
    icms_entrada: number;
    icms_saida: number;
    icms_a_pagar: number;
    ibs_projetado: number;
    cbs_projetado: number;
    faturamento_anterior: number;
    icms_a_pagar_anterior: number;
    periodo_anterior: string;
    total_jobs: number;
    aliquota_efetiva_icms: number;
    aliquota_efetiva_ibs: number;
    aliquota_efetiva_cbs: number;
    aliquota_efetiva_total_reforma: number;
    aliquota_efetiva_icms_anterior: number;
  } | null;
  periodo: string;
  model?: string;
  cached: boolean;
}

const formatMoney = (value: number) => {
  return new Intl.NumberFormat('pt-BR', {
    style: 'currency',
    currency: 'BRL',
  }).format(value);
};

const formatPct = (value: number) =>
  `${value.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}%`;

// Simple Markdown renderer for narratives
function RenderMarkdown({ text }: { text: string }) {
  const lines = text.split('\n');
  const elements: JSX.Element[] = [];

  lines.forEach((line, i) => {
    const trimmed = line.trim();
    if (trimmed.startsWith('## ')) {
      elements.push(<h2 key={i} className="text-lg font-bold mt-4 mb-2">{trimmed.slice(3)}</h2>);
    } else if (trimmed.startsWith('### ')) {
      elements.push(<h3 key={i} className="text-base font-semibold mt-3 mb-1">{trimmed.slice(4)}</h3>);
    } else if (trimmed.startsWith('- ') || trimmed.startsWith('* ')) {
      const content = trimmed.slice(2);
      elements.push(
        <li key={i} className="ml-4 text-sm text-muted-foreground list-disc">
          <BoldText text={content} />
        </li>
      );
    } else if (trimmed.match(/^\d+\. /)) {
      const content = trimmed.replace(/^\d+\. /, '');
      elements.push(
        <li key={i} className="ml-4 text-sm text-muted-foreground list-decimal">
          <BoldText text={content} />
        </li>
      );
    } else if (trimmed === '') {
      elements.push(<div key={i} className="h-2" />);
    } else {
      elements.push(
        <p key={i} className="text-sm text-muted-foreground leading-relaxed">
          <BoldText text={trimmed} />
        </p>
      );
    }
  });

  return <div className="space-y-0.5">{elements}</div>;
}

function BoldText({ text }: { text: string }) {
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  return (
    <>
      {parts.map((part, i) => {
        if (part.startsWith('**') && part.endsWith('**')) {
          return <strong key={i} className="font-semibold text-foreground">{part.slice(2, -2)}</strong>;
        }
        // Handle italic
        const italicParts = part.split(/(\*[^*]+\*)/g);
        return italicParts.map((ip, j) => {
          if (ip.startsWith('*') && ip.endsWith('*') && !ip.startsWith('**')) {
            return <em key={`${i}-${j}`} className="italic">{ip.slice(1, -1)}</em>;
          }
          return <span key={`${i}-${j}`}>{ip}</span>;
        });
      })}
    </>
  );
}

// Generate month options for the selector
function getMonthOptions() {
  const options = [];
  const now = new Date();
  for (let i = 0; i < 12; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 1);
    const value = `${String(d.getMonth() + 1).padStart(2, '0')}/${d.getFullYear()}`;
    const label = d.toLocaleDateString('pt-BR', { month: 'long', year: 'numeric' });
    options.push({ value, label: label.charAt(0).toUpperCase() + label.slice(1) });
  }
  return options;
}

// Widget dedicado: Carga Tributária Efetiva
function CargaTributariaEfetiva({ dados }: { dados: NonNullable<SummaryData['dados']> }) {
  const { aliquota_efetiva_icms, aliquota_efetiva_ibs, aliquota_efetiva_cbs,
          aliquota_efetiva_total_reforma, aliquota_efetiva_icms_anterior } = dados;

  if (!aliquota_efetiva_icms && !aliquota_efetiva_total_reforma) return null;

  const diff = aliquota_efetiva_total_reforma - aliquota_efetiva_icms;
  const diffPrev = aliquota_efetiva_icms_anterior
    ? aliquota_efetiva_icms - aliquota_efetiva_icms_anterior
    : null;

  const maxRate = Math.max(aliquota_efetiva_icms, aliquota_efetiva_total_reforma, 1);

  return (
    <Card>
      <CardHeader className="pb-2 pt-4 px-4">
        <div className="flex items-center justify-between">
          <CardTitle className="text-sm font-semibold">Carga Tributária Efetiva</CardTitle>
          <Badge variant="outline" className="text-[10px]">% sobre faturamento</Badge>
        </div>
        <CardDescription className="text-[11px]">
          Quanto do faturamento bruto é consumido por cada imposto após créditos
        </CardDescription>
      </CardHeader>
      <CardContent className="px-4 pb-4 space-y-4">
        {/* Barras comparativas */}
        <div className="space-y-3">
          {/* ICMS atual */}
          <div className="space-y-1">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground font-medium">ICMS (Regime Atual)</span>
              <div className="flex items-center gap-2">
                {diffPrev !== null && (
                  <span className={`text-[10px] ${diffPrev > 0 ? 'text-red-500' : diffPrev < 0 ? 'text-emerald-600' : 'text-muted-foreground'}`}>
                    {diffPrev > 0 ? '+' : ''}{diffPrev.toFixed(2)} p.p. vs anterior
                  </span>
                )}
                <span className="font-bold text-orange-600">{formatPct(aliquota_efetiva_icms)}</span>
              </div>
            </div>
            <div className="h-2.5 w-full bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-orange-500 rounded-full transition-all"
                style={{ width: `${(aliquota_efetiva_icms / maxRate) * 100}%` }}
              />
            </div>
          </div>

          {/* IBS projetado */}
          <div className="space-y-1">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground font-medium">IBS (Reforma 2033)</span>
              <span className="font-bold text-blue-600">{formatPct(aliquota_efetiva_ibs)}</span>
            </div>
            <div className="h-2.5 w-full bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-blue-500 rounded-full transition-all"
                style={{ width: `${(aliquota_efetiva_ibs / maxRate) * 100}%` }}
              />
            </div>
          </div>

          {/* CBS projetado */}
          <div className="space-y-1">
            <div className="flex items-center justify-between text-xs">
              <span className="text-muted-foreground font-medium">CBS (Reforma 2033)</span>
              <span className="font-bold text-purple-600">{formatPct(aliquota_efetiva_cbs)}</span>
            </div>
            <div className="h-2.5 w-full bg-muted rounded-full overflow-hidden">
              <div
                className="h-full bg-purple-500 rounded-full transition-all"
                style={{ width: `${(aliquota_efetiva_cbs / maxRate) * 100}%` }}
              />
            </div>
          </div>
        </div>

        {/* Resumo do impacto */}
        <div className="border-t pt-3 flex items-center justify-between">
          <div className="text-xs text-muted-foreground">
            <span className="font-medium">Total IBS + CBS (2033):</span>{' '}
            <span className="font-bold text-foreground">{formatPct(aliquota_efetiva_total_reforma)}</span>
          </div>
          <div className={`flex items-center gap-1 text-xs font-semibold ${
            diff < -0.01 ? 'text-emerald-600' : diff > 0.01 ? 'text-red-500' : 'text-muted-foreground'
          }`}>
            {diff < -0.01 ? <TrendingDown className="h-3.5 w-3.5" /> :
             diff > 0.01 ? <TrendingUp className="h-3.5 w-3.5" /> :
             <Minus className="h-3.5 w-3.5" />}
            {diff > 0 ? '+' : ''}{diff.toFixed(2)} p.p. vs ICMS
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

export default function ExecutiveSummary() {
  const { token, companyId } = useAuth();
  const [data, setData] = useState<SummaryData | null>(null);
  const [loading, setLoading] = useState(false);
  const [selectedPeriod, setSelectedPeriod] = useState('');
  const [availablePeriods, setAvailablePeriods] = useState<string[]>([]);

  const monthOptions = getMonthOptions();

  // Fetch available periods and default to the most recent one with data
  useEffect(() => {
    const fetchPeriods = async () => {
      try {
        const headers: Record<string, string> = {
          Authorization: `Bearer ${token || localStorage.getItem('token')}`,
        };
        if (companyId) {
          headers['X-Company-ID'] = companyId;
        }
        const response = await fetch('/api/reports/available-periods', { headers });
        if (response.ok) {
          const result = await response.json();
          setAvailablePeriods(result.periods || []);
          if (result.latest) {
            setSelectedPeriod(result.latest);
            return;
          }
        }
      } catch (error) {
        console.error('Error fetching available periods:', error);
      }
      // Fallback to current month if no periods available
      const now = new Date();
      setSelectedPeriod(`${String(now.getMonth() + 1).padStart(2, '0')}/${now.getFullYear()}`);
    };
    fetchPeriods();
  }, [token, companyId]);

  const fetchSummary = async (periodo: string, force = false) => {
    setLoading(true);
    try {
      const headers: Record<string, string> = {
        Authorization: `Bearer ${token || localStorage.getItem('token')}`,
      };
      if (companyId) {
        headers['X-Company-ID'] = companyId;
      }
      const forceParam = force ? '&force=true' : '';
      const response = await fetch(`/api/reports/executive-summary?periodo=${periodo}${forceParam}`, { headers });
      if (response.ok) {
        const result = await response.json();
        setData(result);
      } else {
        console.error('Failed to fetch executive summary');
      }
    } catch (error) {
      console.error('Error fetching executive summary:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (selectedPeriod) {
      fetchSummary(selectedPeriod);
    }
  }, [selectedPeriod, companyId]);

  const handlePeriodChange = (value: string) => {
    setSelectedPeriod(value);
  };

  const dados = data?.dados;

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-2">
          <h1 className="text-lg md:text-xl lg:text-2xl font-bold tracking-tight">Resumo Executivo</h1>
          <Badge variant="secondary" className="gap-1 text-[10px]">
            <Sparkles className="h-3 w-3" />
            IA
          </Badge>
        </div>
        <p className="text-[10px] md:text-sm text-muted-foreground">
          Analise inteligente da apuracao fiscal gerada por inteligencia artificial.
        </p>
      </div>

      <div className="flex items-center gap-4">
        <Select onValueChange={handlePeriodChange} value={selectedPeriod}>
          <SelectTrigger className="w-[200px]">
            <SelectValue placeholder="Selecione o periodo" />
          </SelectTrigger>
          <SelectContent>
            {monthOptions.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}{availablePeriods.includes(opt.value) ? ' *' : ''}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Button
          variant="outline"
          size="icon"
          onClick={() => fetchSummary(selectedPeriod)}
          disabled={loading}
          title="Atualizar"
        >
          <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
        </Button>

        {data?.narrativa && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => fetchSummary(selectedPeriod, true)}
            disabled={loading}
            className="text-xs gap-1"
          >
            <Sparkles className="h-3 w-3" />
            Regenerar
          </Button>
        )}

        {data?.model && (
          <span className="text-[10px] text-muted-foreground">
            Modelo: {data.model}
          </span>
        )}
      </div>

      {/* Summary Cards */}
      {dados && dados.faturamento_bruto > 0 && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          <Card>
            <CardHeader className="pb-1 pt-3 px-3">
              <CardTitle className="text-[10px] md:text-xs font-medium text-muted-foreground">Faturamento</CardTitle>
            </CardHeader>
            <CardContent className="px-3 pb-3">
              <div className="text-sm md:text-lg font-bold">{formatMoney(dados.faturamento_bruto)}</div>
              {dados.faturamento_anterior > 0 && (
                <VariationBadge current={dados.faturamento_bruto} previous={dados.faturamento_anterior} />
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1 pt-3 px-3">
              <CardTitle className="text-[10px] md:text-xs font-medium text-muted-foreground">ICMS a Recolher</CardTitle>
            </CardHeader>
            <CardContent className="px-3 pb-3">
              <div className="text-sm md:text-lg font-bold text-red-600">{formatMoney(dados.icms_a_pagar)}</div>
              {dados.icms_a_pagar_anterior > 0 && (
                <VariationBadge current={dados.icms_a_pagar} previous={dados.icms_a_pagar_anterior} inverted />
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1 pt-3 px-3">
              <CardTitle className="text-[10px] md:text-xs font-medium text-muted-foreground">Creditos (Entrada)</CardTitle>
            </CardHeader>
            <CardContent className="px-3 pb-3">
              <div className="text-sm md:text-lg font-bold text-emerald-600">{formatMoney(dados.icms_entrada)}</div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-1 pt-3 px-3">
              <CardTitle className="text-[10px] md:text-xs font-medium text-muted-foreground">Debitos (Saida)</CardTitle>
            </CardHeader>
            <CardContent className="px-3 pb-3">
              <div className="text-sm md:text-lg font-bold">{formatMoney(dados.icms_saida)}</div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Carga Tributária Efetiva */}
      {dados && <CargaTributariaEfetiva dados={dados} />}

      {/* AI Narrative */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <FileText className="h-5 w-5 text-muted-foreground" />
            <CardTitle className="text-base">Analise com Inteligencia Artificial</CardTitle>
          </div>
          <CardDescription>
            {dados?.company_name && `${dados.company_name} — `}Periodo: {selectedPeriod}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {loading ? (
            <div className="flex items-center justify-center py-12">
              <div className="flex flex-col items-center gap-3">
                <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
                <span className="text-sm text-muted-foreground">Gerando analise com IA...</span>
              </div>
            </div>
          ) : data?.narrativa ? (
            <RenderMarkdown text={data.narrativa} />
          ) : (
            <p className="text-sm text-muted-foreground">
              Nenhum dado disponivel para gerar o resumo. Importe arquivos SPED primeiro.
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function VariationBadge({
  current,
  previous,
  inverted = false,
}: {
  current: number;
  previous: number;
  inverted?: boolean;
}) {
  if (previous === 0) return null;
  const pct = ((current - previous) / previous) * 100;
  const isUp = pct > 0;
  // For costs (inverted), up is bad; for revenue, up is good
  const isGood = inverted ? !isUp : isUp;

  return (
    <span
      className={`text-[10px] font-medium ${
        isGood ? 'text-emerald-600' : 'text-red-500'
      }`}
    >
      {isUp ? '+' : ''}{pct.toFixed(1)}% vs anterior
    </span>
  );
}
