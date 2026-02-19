import { useState, useEffect, useCallback } from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Globe, Send, RefreshCw, ChevronLeft, AlertTriangle, Download } from 'lucide-react';

interface RFBRequest {
  id: string;
  cnpj_base: string;
  tiquete: string;
  status: string;
  ambiente: string;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

interface RFBResumo {
  data_apuracao: string;
  total_debitos: number;
  valor_cbs_total: number;
  valor_cbs_extinto: number;
  valor_cbs_nao_extinto: number;
  total_corrente: number;
  total_ajuste: number;
  total_extemporaneo: number;
}

interface RFBDebito {
  id: string;
  tipo_apuracao: string;
  modelo_dfe: string;
  numero_dfe: string;
  chave_dfe: string;
  data_dfe_emissao?: string;
  data_apuracao: string;
  ni_emitente: string;
  ni_adquirente: string;
  valor_cbs_total: number;
  valor_cbs_extinto: number;
  valor_cbs_nao_extinto: number;
  situacao_debito: string;
}

const statusConfig: Record<string, { label: string; color: string }> = {
  pending: { label: 'Pendente', color: 'bg-gray-100 text-gray-700' },
  requested: { label: 'Solicitado', color: 'bg-yellow-100 text-yellow-700' },
  webhook_received: { label: 'Processando', color: 'bg-blue-100 text-blue-700' },
  downloading: { label: 'Baixando', color: 'bg-blue-100 text-blue-700' },
  completed: { label: 'Concluído', color: 'bg-green-100 text-green-700' },
  error: { label: 'Erro', color: 'bg-red-100 text-red-700' },
};

function formatCurrency(value: number): string {
  return new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' }).format(value);
}

function formatCNPJBase(cnpj: string): string {
  if (cnpj.length === 8) return `${cnpj.slice(0, 2)}.${cnpj.slice(2, 5)}.${cnpj.slice(5)}`;
  return cnpj;
}

function formatPeriodo(p: string): string {
  if (p.length === 6) return `${p.slice(4, 6)}/${p.slice(0, 4)}`;
  return p;
}

export default function RFBApuracao() {
  const [requests, setRequests] = useState<RFBRequest[]>([]);
  const [loading, setLoading] = useState(true);
  const [soliciting, setSoliciting] = useState(false);
  const [selectedRequest, setSelectedRequest] = useState<string | null>(null);
  const [detail, setDetail] = useState<{ request: RFBRequest; resumo: RFBResumo | null; debitos: RFBDebito[] } | null>(null);
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const getHeaders = () => {
    const token = localStorage.getItem('token');
    const companyId = localStorage.getItem('selectedCompanyId');
    return {
      'Authorization': `Bearer ${token}`,
      'X-Company-ID': companyId || '',
    };
  };

  const fetchRequests = useCallback(async () => {
    try {
      const response = await fetch('/api/rfb/apuracao/status', { headers: getHeaders() });
      if (response.ok) {
        const data = await response.json();
        setRequests(data.requests || []);
      }
    } catch {
      // silent
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRequests();
  }, [fetchRequests]);

  // Poll for status updates when there are pending requests
  useEffect(() => {
    const hasPending = requests.some(r => ['pending', 'requested', 'webhook_received', 'downloading'].includes(r.status));
    if (!hasPending) return;

    const interval = setInterval(fetchRequests, 10000);
    return () => clearInterval(interval);
  }, [requests, fetchRequests]);

  const handleSolicitar = async () => {
    if (!confirm('Solicitar apuração de débitos CBS à Receita Federal?\n\nLimite: 2 solicitações por dia.')) return;
    setMessage(null);
    setSoliciting(true);

    try {
      const response = await fetch('/api/rfb/apuracao/solicitar', {
        method: 'POST',
        headers: { ...getHeaders(), 'Content-Type': 'application/json' },
      });

      if (response.ok) {
        const data = await response.json();
        setMessage({ type: 'success', text: data.message || 'Solicitação enviada!' });
        fetchRequests();
      } else {
        const text = await response.text();
        setMessage({ type: 'error', text: text || 'Erro ao solicitar apuração' });
      }
    } catch {
      setMessage({ type: 'error', text: 'Erro de conexão' });
    } finally {
      setSoliciting(false);
    }
  };

  const fetchDetail = async (requestId: string) => {
    setSelectedRequest(requestId);
    try {
      const response = await fetch(`/api/rfb/apuracao/${requestId}`, { headers: getHeaders() });
      if (response.ok) {
        const data = await response.json();
        setDetail(data);
      }
    } catch {
      setMessage({ type: 'error', text: 'Erro ao carregar detalhes' });
    }
  };

  const handleDownloadManual = async (requestId: string) => {
    setMessage(null);
    try {
      const response = await fetch('/api/rfb/apuracao/download', {
        method: 'POST',
        headers: { ...getHeaders(), 'Content-Type': 'application/json' },
        body: JSON.stringify({ request_id: requestId }),
      });
      if (response.ok) {
        setMessage({ type: 'success', text: 'Download iniciado! Acompanhe o status.' });
        fetchRequests();
      } else {
        const text = await response.text();
        setMessage({ type: 'error', text: text || 'Erro ao iniciar download' });
      }
    } catch {
      setMessage({ type: 'error', text: 'Erro de conexão' });
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  // Detail view
  if (selectedRequest && detail) {
    const { request: req, resumo, debitos } = detail;
    const sc = statusConfig[req.status] || statusConfig.pending;

    return (
      <div className="max-w-7xl mx-auto px-4 py-8">
        <Button variant="ghost" className="mb-4" onClick={() => { setSelectedRequest(null); setDetail(null); }}>
          <ChevronLeft className="mr-1 h-4 w-4" /> Voltar
        </Button>

        <div className="mb-6">
          <h2 className="text-2xl font-bold flex items-center gap-2">
            Detalhes da Apuração CBS
            <Badge className={sc.color}>{sc.label}</Badge>
          </h2>
          <p className="text-sm text-muted-foreground mt-1">
            CNPJ Base: {formatCNPJBase(req.cnpj_base)} | Solicitado em: {new Date(req.created_at).toLocaleString('pt-BR')}
          </p>
        </div>

        {req.error_message && (
          <div className="mb-4 rounded-md p-4 bg-red-50 text-red-800">
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4" />
              <span className="text-sm font-medium">{req.error_code}: {req.error_message}</span>
            </div>
          </div>
        )}

        {resumo && (
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
            <Card>
              <CardContent className="pt-4">
                <p className="text-xs text-muted-foreground">Período</p>
                <p className="text-xl font-bold">{formatPeriodo(resumo.data_apuracao)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4">
                <p className="text-xs text-muted-foreground">CBS Total</p>
                <p className="text-xl font-bold text-red-600">{formatCurrency(resumo.valor_cbs_total)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4">
                <p className="text-xs text-muted-foreground">CBS Extinto</p>
                <p className="text-xl font-bold text-green-600">{formatCurrency(resumo.valor_cbs_extinto)}</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4">
                <p className="text-xs text-muted-foreground">CBS Não Extinto</p>
                <p className="text-xl font-bold text-orange-600">{formatCurrency(resumo.valor_cbs_nao_extinto)}</p>
              </CardContent>
            </Card>
          </div>
        )}

        {resumo && (
          <div className="grid grid-cols-3 gap-4 mb-6">
            <Card>
              <CardContent className="pt-4 text-center">
                <p className="text-xs text-muted-foreground">Corrente</p>
                <p className="text-lg font-bold">{resumo.total_corrente} débitos</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4 text-center">
                <p className="text-xs text-muted-foreground">Ajuste</p>
                <p className="text-lg font-bold">{resumo.total_ajuste} débitos</p>
              </CardContent>
            </Card>
            <Card>
              <CardContent className="pt-4 text-center">
                <p className="text-xs text-muted-foreground">Extemporâneo</p>
                <p className="text-lg font-bold">{resumo.total_extemporaneo} débitos</p>
              </CardContent>
            </Card>
          </div>
        )}

        {debitos && debitos.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Débitos CBS ({debitos.length})</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="overflow-x-auto">
                <table className="min-w-full divide-y divide-gray-300 text-sm">
                  <thead className="bg-gray-50">
                    <tr>
                      <th className="px-3 py-2 text-left font-semibold">Tipo</th>
                      <th className="px-3 py-2 text-left font-semibold">NF-e</th>
                      <th className="px-3 py-2 text-left font-semibold">Emitente</th>
                      <th className="px-3 py-2 text-right font-semibold">CBS Total</th>
                      <th className="px-3 py-2 text-right font-semibold">Extinto</th>
                      <th className="px-3 py-2 text-right font-semibold">Não Extinto</th>
                      <th className="px-3 py-2 text-left font-semibold">Situação</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-200">
                    {debitos.map((d) => (
                      <tr key={d.id} className="hover:bg-gray-50">
                        <td className="px-3 py-2">
                          <Badge variant="outline" className="text-xs">
                            {d.tipo_apuracao === 'corrente' ? 'Corrente' : d.tipo_apuracao === 'ajuste' ? 'Ajuste' : 'Extemp.'}
                          </Badge>
                        </td>
                        <td className="px-3 py-2 font-mono text-xs">{d.numero_dfe || '-'}</td>
                        <td className="px-3 py-2 font-mono text-xs">{d.ni_emitente}</td>
                        <td className="px-3 py-2 text-right">{formatCurrency(d.valor_cbs_total)}</td>
                        <td className="px-3 py-2 text-right text-green-600">{formatCurrency(d.valor_cbs_extinto)}</td>
                        <td className="px-3 py-2 text-right text-orange-600">{formatCurrency(d.valor_cbs_nao_extinto)}</td>
                        <td className="px-3 py-2 text-xs">{d.situacao_debito}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </CardContent>
          </Card>
        )}

        {req.status === 'completed' && (!debitos || debitos.length === 0) && (
          <Card>
            <CardContent className="py-8 text-center text-muted-foreground">
              Nenhum débito CBS encontrado para este período.
            </CardContent>
          </Card>
        )}
      </div>
    );
  }

  // List view
  return (
    <div className="max-w-5xl mx-auto px-4 py-8">
      <div className="md:flex md:items-center md:justify-between mb-6">
        <div>
          <h2 className="text-2xl font-bold flex items-center gap-2">
            <Globe className="h-6 w-6" />
            Apuração CBS - Receita Federal
          </h2>
          <p className="mt-1 text-sm text-gray-600">
            Solicite e acompanhe a apuração de débitos CBS diretamente da Receita Federal.
          </p>
        </div>
        <div className="mt-4 md:mt-0 flex gap-2">
          <Button variant="outline" onClick={fetchRequests}>
            <RefreshCw className="mr-2 h-4 w-4" /> Atualizar
          </Button>
          <Button onClick={handleSolicitar} disabled={soliciting}>
            <Send className="mr-2 h-4 w-4" />
            {soliciting ? 'Solicitando...' : 'Solicitar Apuração CBS'}
          </Button>
        </div>
      </div>

      {message && (
        <div className={`mb-4 rounded-md p-4 ${
          message.type === 'success' ? 'bg-green-50 text-green-800' : 'bg-red-50 text-red-800'
        }`}>
          <p className="text-sm font-medium">{message.text}</p>
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle className="text-lg">Solicitações</CardTitle>
          <CardDescription>Histórico de solicitações de apuração CBS (limite: 2 por dia)</CardDescription>
        </CardHeader>
        <CardContent>
          {requests.length === 0 ? (
            <div className="py-8 text-center text-muted-foreground">
              <Globe className="mx-auto h-12 w-12 mb-3 opacity-30" />
              <p>Nenhuma solicitação realizada.</p>
              <p className="text-xs mt-1">Clique em "Solicitar Apuração CBS" para começar.</p>
            </div>
          ) : (
            <div className="space-y-3">
              {requests.map((req) => {
                const sc = statusConfig[req.status] || statusConfig.pending;
                const isPending = ['pending', 'requested', 'webhook_received', 'downloading'].includes(req.status);

                return (
                  <div
                    key={req.id}
                    className={`flex items-center justify-between p-4 rounded-lg border ${
                      req.status === 'completed' ? 'cursor-pointer hover:bg-gray-50' : ''
                    }`}
                    onClick={() => req.status === 'completed' && fetchDetail(req.id)}
                  >
                    <div className="flex items-center gap-4">
                      {isPending && (
                        <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-blue-600"></div>
                      )}
                      <div>
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-sm">CNPJ: {formatCNPJBase(req.cnpj_base)}</span>
                          <Badge className={sc.color}>{sc.label}</Badge>
                        </div>
                        <p className="text-xs text-muted-foreground mt-0.5">
                          {new Date(req.created_at).toLocaleString('pt-BR')}
                          {req.error_message && (
                            <span className="text-red-600 ml-2">{req.error_message}</span>
                          )}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {req.status === 'requested' && (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={(e) => { e.stopPropagation(); handleDownloadManual(req.id); }}
                        >
                          <Download className="mr-1 h-3 w-3" /> Download Manual
                        </Button>
                      )}
                      {req.status === 'completed' && (
                        <span className="text-xs text-muted-foreground">Clique para ver detalhes</span>
                      )}
                      {req.status === 'error' && (
                        <Badge variant="destructive" className="text-xs">{req.error_code}</Badge>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
