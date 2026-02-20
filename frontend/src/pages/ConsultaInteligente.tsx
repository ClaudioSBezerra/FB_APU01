import { useState, useRef } from 'react';
import { Sparkles, Send, ChevronDown, ChevronUp, Loader2, AlertCircle, Database } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Textarea } from '@/components/ui/textarea';

// ─── Tipos ───────────────────────────────────────────────────────────────────

interface QueryResult {
  pergunta: string;
  sql: string;
  columns: string[];
  rows: Record<string, unknown>[];
  row_count: number;
  model: string;
}

interface QueryError {
  error: string;
  sql?: string;
  ai_text?: string;
}

// ─── Sugestões ───────────────────────────────────────────────────────────────

const sugestoes = [
  'Qual fornecedor do Simples Nacional gerou mais prejuízo?',
  'Qual o faturamento total por filial no último período importado?',
  'Quais os 10 maiores fornecedores por valor de compra?',
  'Qual a proporção de entradas vs saídas por mês?',
  'Qual o total de IBS e CBS projetado por período?',
  'Quais filiais têm ICMS a pagar acima de R$ 100.000?',
];

// ─── Helpers ─────────────────────────────────────────────────────────────────

function formatCellValue(value: unknown): string {
  if (value === null || value === undefined) return '—';
  if (typeof value === 'number') {
    if (Math.abs(value) > 100) {
      return new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' }).format(value);
    }
    return value.toLocaleString('pt-BR', { maximumFractionDigits: 4 });
  }
  if (typeof value === 'object' && value instanceof Uint8Array) {
    return Buffer.from(value).toString('hex');
  }
  return String(value);
}

function isMoneyColumn(col: string): boolean {
  return /valor|vl_|total|prejuizo|icms|ibs|cbs|doc/i.test(col);
}

// ─── Componente ──────────────────────────────────────────────────────────────

export default function ConsultaInteligente() {
  const [pergunta, setPergunta] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<QueryResult | null>(null);
  const [error, setError] = useState<QueryError | null>(null);
  const [showSQL, setShowSQL] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const getHeaders = () => {
    const token = localStorage.getItem('token');
    const companyId = localStorage.getItem('selectedCompanyId');
    return {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${token}`,
      'X-Company-ID': companyId || '',
    };
  };

  const handleQuery = async (q?: string) => {
    const query = (q ?? pergunta).trim();
    if (!query) return;

    setLoading(true);
    setResult(null);
    setError(null);
    setShowSQL(false);
    if (q) setPergunta(q);

    try {
      const resp = await fetch('/api/ai/query', {
        method: 'POST',
        headers: getHeaders(),
        body: JSON.stringify({ pergunta: query }),
      });

      const data = await resp.json();

      if (!resp.ok) {
        setError(data as QueryError);
      } else {
        setResult(data as QueryResult);
        setShowSQL(false);
      }
    } catch {
      setError({ error: 'Erro de conexão com o servidor.' });
    } finally {
      setLoading(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleQuery();
    }
  };

  return (
    <div className="max-w-5xl mx-auto space-y-5 pb-10">

      {/* ── Cabeçalho ── */}
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-violet-500 to-indigo-600 text-white shadow">
          <Sparkles className="h-5 w-5" />
        </div>
        <div>
          <h1 className="text-2xl font-bold">Consulta Inteligente</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            Faça perguntas em português sobre os dados fiscais importados. A IA gera e executa o SQL automaticamente.
          </p>
        </div>
      </div>

      {/* ── Sugestões ── */}
      <Card>
        <CardHeader className="pb-2 pt-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">Perguntas sugeridas</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2 pb-4">
          {sugestoes.map((s) => (
            <button
              key={s}
              onClick={() => handleQuery(s)}
              disabled={loading}
              className="text-xs px-3 py-1.5 rounded-full border border-indigo-200 bg-indigo-50 text-indigo-700 hover:bg-indigo-100 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              {s}
            </button>
          ))}
        </CardContent>
      </Card>

      {/* ── Input ── */}
      <div className="flex gap-2 items-end">
        <Textarea
          ref={textareaRef}
          value={pergunta}
          onChange={(e) => setPergunta(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Ex: Qual o total de ICMS pago em 2024 por filial?"
          className="resize-none text-sm min-h-[60px] max-h-32"
          rows={2}
          disabled={loading}
        />
        <Button
          onClick={() => handleQuery()}
          disabled={loading || !pergunta.trim()}
          className="h-[60px] px-4 bg-indigo-600 hover:bg-indigo-700"
        >
          {loading
            ? <Loader2 className="h-4 w-4 animate-spin" />
            : <Send className="h-4 w-4" />
          }
        </Button>
      </div>

      {/* ── Loading ── */}
      {loading && (
        <Card className="border-indigo-100 bg-indigo-50/40">
          <CardContent className="py-6 flex items-center gap-3 text-indigo-700">
            <Loader2 className="h-5 w-5 animate-spin shrink-0" />
            <div>
              <p className="text-sm font-medium">Gerando SQL e consultando o banco...</p>
              <p className="text-xs text-indigo-500 mt-0.5">A IA está analisando sua pergunta e os dados fiscais.</p>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── Erro ── */}
      {error && (
        <Card className="border-red-200 bg-red-50">
          <CardContent className="py-4 space-y-2">
            <div className="flex items-center gap-2 text-red-700">
              <AlertCircle className="h-4 w-4 shrink-0" />
              <p className="text-sm font-medium">{error.error}</p>
            </div>
            {error.sql && (
              <pre className="mt-2 text-xs bg-red-100 text-red-800 p-3 rounded-md overflow-x-auto whitespace-pre-wrap">
                {error.sql}
              </pre>
            )}
          </CardContent>
        </Card>
      )}

      {/* ── Resultado ── */}
      {result && (
        <div className="space-y-3">

          {/* Header do resultado */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Database className="h-4 w-4 text-muted-foreground" />
              <span className="text-sm font-medium">
                {result.row_count} {result.row_count === 1 ? 'resultado' : 'resultados'}
              </span>
              <Badge variant="outline" className="text-[10px] bg-violet-50 text-violet-700 border-violet-200">
                {result.model}
              </Badge>
            </div>
            <button
              onClick={() => setShowSQL(!showSQL)}
              className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              {showSQL ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
              {showSQL ? 'Ocultar SQL' : 'Ver SQL gerado'}
            </button>
          </div>

          {/* SQL colapsável */}
          {showSQL && (
            <Card className="border-slate-200 bg-slate-50">
              <CardContent className="py-3 px-4">
                <pre className="text-xs text-slate-700 overflow-x-auto whitespace-pre-wrap leading-relaxed">
                  {result.sql}
                </pre>
              </CardContent>
            </Card>
          )}

          {/* Tabela de resultados */}
          {result.row_count === 0 ? (
            <Card>
              <CardContent className="py-10 text-center text-sm text-muted-foreground">
                Nenhum dado encontrado para essa consulta.
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="p-0">
                <div className="overflow-x-auto rounded-md">
                  <table className="min-w-full divide-y divide-gray-200 text-sm">
                    <thead className="bg-gray-50">
                      <tr>
                        {result.columns.map((col) => (
                          <th
                            key={col}
                            className={`px-4 py-2.5 text-xs font-semibold text-gray-600 ${
                              isMoneyColumn(col) ? 'text-right' : 'text-left'
                            }`}
                          >
                            {col.replace(/_/g, ' ')}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-100 bg-white">
                      {result.rows.map((row, i) => (
                        <tr key={i} className="hover:bg-gray-50 transition-colors">
                          {result.columns.map((col) => (
                            <td
                              key={col}
                              className={`px-4 py-2 text-xs ${
                                isMoneyColumn(col) ? 'text-right font-medium' : 'text-left'
                              }`}
                            >
                              {formatCellValue(row[col])}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                {result.row_count >= 100 && (
                  <p className="text-center text-xs text-muted-foreground py-2 border-t">
                    Exibindo os primeiros 100 resultados.
                  </p>
                )}
              </CardContent>
            </Card>
          )}
        </div>
      )}
    </div>
  );
}
