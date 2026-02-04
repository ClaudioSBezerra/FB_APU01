import { useState, useRef, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { CheckCircle, Clock, FileText, Loader2, Upload, XCircle, Trash2, FolderOpen } from 'lucide-react';
import { toast } from 'sonner';
import { UploadProgressDisplay, UploadProgressType } from '@/components/UploadProgress';
import { useAuth } from '@/contexts/AuthContext';

interface ImportJob {
  id: string;
  filename: string;
  status: 'pending' | 'processing' | 'completed' | 'error';
  created_at: string;
  updated_at: string;
  message?: string;
}

export default function ImportarEFD() {
  const navigate = useNavigate();
  const { token, user, cnpj, company, companyId } = useAuth();
  const [selectedFiles, setSelectedFiles] = useState<File[]>([]);
  const [isDragOver, setIsDragOver] = useState(false);
  
  // Progress state for the current file being uploaded
  const [uploadProgress, setUploadProgress] = useState<UploadProgressType>({
    status: 'idle',
    percentage: 0,
    bytesUploaded: 0,
    bytesTotal: 0,
    speed: 0,
    remainingTime: 0
  });

  // Global queue progress
  const [currentFileIndex, setCurrentFileIndex] = useState<number>(-1);
  const [isUploading, setIsUploading] = useState(false);

  const [jobs, setJobs] = useState<ImportJob[]>([]);
  const [scanStats, setScanStats] = useState({ scanned: 0, relevant: 0, phase: 'idle' });
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);

  // Poll jobs list
  useEffect(() => {
    let pollInterval: NodeJS.Timeout;

    const fetchJobs = async () => {
      if (!token) return;
      try {
        const res = await fetch('/api/jobs', {
          headers: {
            'Authorization': `Bearer ${token}`,
            'Cache-Control': 'no-cache',
            'Pragma': 'no-cache'
          }
        }); 
        if (res.ok) {
          const data = await res.json();
          setJobs(data);
        } else if (res.status === 401) {
            console.error("Unauthorized polling jobs - redirecting");
            clearInterval(pollInterval);
            navigate('/login');
            return; // Stop loop
        }
      } catch (error) {
        console.error("Error polling jobs:", error);
      }
    };

    // Initial fetch
    fetchJobs();

    // Set interval
    pollInterval = setInterval(fetchJobs, 2000);

    return () => clearInterval(pollInterval);
  }, [token, navigate]);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      const allFiles = Array.from(e.target.files);
      // Filter only .txt files
      const txtFiles = allFiles.filter(file => file.name.toLowerCase().endsWith('.txt'));
      
      if (txtFiles.length < allFiles.length) {
          toast.info(`${allFiles.length - txtFiles.length} arquivos ignorados (apenas .txt são permitidos).`);
      }

      if (txtFiles.length > 0) {
        setSelectedFiles(prev => [...prev, ...txtFiles]);
      } else if (allFiles.length > 0) {
        toast.warning("Nenhum arquivo .txt encontrado na seleção.");
      }
    }
    // Reset input value to allow selecting same files again if needed
    if (fileInputRef.current) {
        fileInputRef.current.value = '';
    }
    if (folderInputRef.current) {
        folderInputRef.current.value = '';
    }
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      const allFiles = Array.from(e.dataTransfer.files);
      const txtFiles = allFiles.filter(file => file.name.toLowerCase().endsWith('.txt'));

      if (txtFiles.length < allFiles.length) {
          toast.info(`${allFiles.length - txtFiles.length} arquivos ignorados (apenas .txt são permitidos).`);
      }

      if (txtFiles.length > 0) {
        setSelectedFiles(prev => [...prev, ...txtFiles]);
      } else if (allFiles.length > 0) {
        toast.warning("Nenhum arquivo .txt válido encontrado. (Para pastas, use o botão 'Selecionar Pasta')");
      }
    }
  };

  const removeFile = (index: number) => {
      if (isUploading) return;
      setSelectedFiles(prev => prev.filter((_, i) => i !== index));
  };

  const processSingleFile = async (file: File): Promise<string | null> => {
    // --- DUPLICITY CHECK ---
    try {
        const firstChunk = file.slice(0, 4096);
        const text = await firstChunk.text();
        const lines = text.split('\n');
        const reg0000 = lines.find(l => l.startsWith('|0000|'));
        
        if (reg0000) {
            const fields = reg0000.split('|');
            // Standard SPED: |0000|COD_VER|COD_FIN|DT_INI|DT_FIN|NOME|CNPJ|...
            // Split results: ["", "0000", "VER", "FIN", "DT_INI", "DT_FIN", "NOME", "CNPJ", ...]
            if (fields.length >= 8) {
                const dtIni = fields[4];
                const cnpj = fields[7];
                
                if (dtIni && cnpj) {
                     const authToken = token || localStorage.getItem('token');
                     const res = await fetch(`/api/check-duplicity?cnpj=${cnpj}&dt_ini=${dtIni}`, {
                        headers: {
                            'Authorization': `Bearer ${authToken}`
                        }
                     });
                     
                     if (res.ok) {
                         const data = await res.json();
                         if (data.exists) {
                             const confirmMsg = `${data.message}\n\nDeseja continuar a importação mesmo assim? (Isso pode gerar duplicidade de dados)`;
                             if (!window.confirm(confirmMsg)) {
                                 toast.info(`Importação de ${file.name} cancelada.`);
                                 return null;
                             }
                         }
                     }
                }
            }
        }
    } catch (err) {
        console.warn("Could not check duplicity:", err);
    }

    // --- CLIENT-SIDE PARSING & FILTERING (PHASE 2.1 - FULL SCAN) ---
    // Reads file locally, filters only relevant lines, continues until EOF
    // Creates a smaller Payload for upload.
    
    let createdJobId: string | null = null;

    setUploadProgress({
      status: 'uploading',
      percentage: 0,
      bytesUploaded: 0,
      bytesTotal: file.size, // Initial size estimation
      speed: 0,
      remainingTime: 0
    });
    setScanStats({ scanned: 0, relevant: 0, phase: 'scanning' });

    try {
      // 1. Filter Logic
      const CHUNK_SIZE = 5 * 1024 * 1024; // 5MB Read Buffer
      let offset = 0;
      let buffer = '';
      const filteredParts: string[] = []; // Stores filtered chunks (strings)
      let totalRelevantLines = 0;
      let totalLinesScanned = 0;

      // Relevant Registers
      const RELEVANT_REGISTERS = new Set([
        '0000', '0140', '0150', 
        'C010', 'C100', 'C170', 'C190', 'C500', 'C600', 
        'D010', 'D100', 'D500', 'D590',
        '9999' // Trailer
      ]);

      // Phase 1: Scan & Filter
      while (offset < file.size) {
        const chunk = file.slice(offset, offset + CHUNK_SIZE);
        const text = await chunk.text();
        const rawPart = buffer + text;
        
        let lastNewlineIndex = rawPart.lastIndexOf('\n');
        // Handle EOF case (last chunk might not end with newline)
        if (offset + CHUNK_SIZE >= file.size && lastNewlineIndex === -1) {
             lastNewlineIndex = rawPart.length;
        }

        const processable = rawPart.substring(0, lastNewlineIndex);
        buffer = rawPart.substring(lastNewlineIndex + 1);

        const lines = processable.split('\n');
        let chunkFiltered = '';
        
        for (const line of lines) {
            const trimmed = line.trim();
            if (!trimmed) continue;
            totalLinesScanned++;

            // SPED format: |REG|...
            if (trimmed.startsWith('|')) {
                const reg = trimmed.split('|')[1];
                if (RELEVANT_REGISTERS.has(reg)) {
                    chunkFiltered += trimmed + '\n';
                    totalRelevantLines++;
                }
            }
        }
        
        if (chunkFiltered.length > 0) {
            filteredParts.push(chunkFiltered);
        }

        offset += CHUNK_SIZE;
        
        // Update UI for Scanning Phase
        setScanStats({ scanned: totalLinesScanned, relevant: totalRelevantLines, phase: 'scanning' });
      }

      setScanStats(prev => ({ ...prev, phase: 'uploading' }));

      // 2. Create Optimized File
      // Combine filtered parts into a single Blob
      const filteredBlob = new Blob(filteredParts, { type: 'text/plain' });
      const filteredFile = new File([filteredBlob], file.name, { type: 'text/plain' });

      console.log(`Optimization: Original ${file.size} bytes -> Filtered ${filteredFile.size} bytes`);
      
      // 3. Upload Optimized File (Chunked)
      const UPLOAD_CHUNK_SIZE = 2 * 1024 * 1024; // 2MB Upload Chunks
      const totalChunks = Math.ceil(filteredFile.size / UPLOAD_CHUNK_SIZE);
      const uploadId = `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
      
      let startTimeUpload = Date.now();
      
      for (let chunkIndex = 0; chunkIndex < totalChunks; chunkIndex++) {
        const start = chunkIndex * UPLOAD_CHUNK_SIZE;
        const end = Math.min(start + UPLOAD_CHUNK_SIZE, filteredFile.size);
        const chunk = filteredFile.slice(start, end);

        const formData = new FormData();
        formData.append('file', chunk, filteredFile.name);
        formData.append('is_chunked', 'true');
        formData.append('upload_id', uploadId);
        formData.append('chunk_index', chunkIndex.toString());
        formData.append('total_chunks', totalChunks.toString());
        // Integrity Metadata
        formData.append('expected_lines', totalRelevantLines.toString());
        formData.append('expected_size', filteredFile.size.toString());

        // Fix: Send company_id to backend to ensure data is linked to the correct company
        if (companyId) {
            formData.append('company_id', companyId);
        }

        // Wait for chunk upload
        const responseJson = await new Promise<any>((resolve, reject) => {
            const xhr = new XMLHttpRequest();
            xhr.open('POST', '/api/upload', true);
            
            // Inject Auth Token
            if (token) {
                xhr.setRequestHeader('Authorization', `Bearer ${token}`);
            }

            xhr.onload = () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    try {
                        resolve(JSON.parse(xhr.responseText));
                    } catch (e) {
                        resolve(null);
                    }
                } else {
                    reject(new Error(`Upload failed: ${xhr.statusText}`));
                }
            };
            xhr.onerror = () => reject(new Error(`Network error at chunk ${chunkIndex}`));
            xhr.send(formData);
        });

        // Last Chunk Verification
        if (chunkIndex === totalChunks - 1 && responseJson) {
           if (responseJson.detected_lines) {
               const receivedLines = responseJson.detected_lines;
               console.log(`Verification: Sent ~${totalRelevantLines} lines, Backend received ${receivedLines}`);
           }
           if (responseJson.job_id) {
               createdJobId = responseJson.job_id;
           }
        }

        // Update Progress
        const percentUpload = ((chunkIndex + 1) / totalChunks) * 100; 
        
        // Calculate speed based on total bytes uploaded so far
        const bytesUploadedSoFar = end;
        const elapsedTime = (Date.now() - startTimeUpload) / 1000;
        const speed = bytesUploadedSoFar / elapsedTime;
        const remainingBytes = filteredFile.size - bytesUploadedSoFar;
        const remainingTime = speed > 0 ? remainingBytes / speed : 0;

        setUploadProgress({
            status: 'uploading',
            percentage: Math.round(percentUpload),
            bytesUploaded: bytesUploadedSoFar,
            bytesTotal: filteredFile.size,
            speed,
            remainingTime
        });
      }

      setUploadProgress(prev => ({ ...prev, status: 'completed', percentage: 100 }));
      toast.success(`Arquivo ${file.name} enviado!`);
      return createdJobId;
      
    } catch (error) {
      console.error(`Upload error for ${file.name}:`, error);
      setUploadProgress(prev => ({ ...prev, status: 'error', errorMessage: String(error) }));
      toast.error(`Erro ao enviar ${file.name}.`);
      throw error; // Re-throw to stop sequence or handle in parent
    }
  };

  const handleUploadAll = async () => {
    if (selectedFiles.length === 0) return;

    setIsUploading(true);
    
    const batchJobIds: string[] = [];

    // Process files sequentially
    for (let i = 0; i < selectedFiles.length; i++) {
        setCurrentFileIndex(i);
        try {
            const jobId = await processSingleFile(selectedFiles[i]);
            if (jobId) batchJobIds.push(jobId);
        } catch (err) {
            // Option: Continue to next file or stop?
            // Let's continue but log error
            console.error(`Failed to process file ${i}`, err);
        }
    }

    if (batchJobIds.length === 0) {
        setIsUploading(false);
        setCurrentFileIndex(-1);
        return;
    }

    // Trigger job refresh and Wait for Processing Completion
    let allCompleted = false;
    const processingToastId = toast.loading(`Processando ${batchJobIds.length} arquivos no servidor...`);
    
    // Wait for all jobs to finish (pending/processing -> completed/error)
    while (!allCompleted) {
        await new Promise(r => setTimeout(r, 2000)); // Poll every 2s
        
        try {
            const authToken = token || localStorage.getItem('token');
            const res = await fetch('/api/jobs', {
              headers: {
                'Authorization': `Bearer ${authToken}`,
                'Cache-Control': 'no-cache',
                'Pragma': 'no-cache'
              }
            }); 
            if (res.ok) {
                const data: ImportJob[] = await res.json();
                setJobs(data);
                
                // Check if any RELEVANT job is still active
                const relevantJobs = data.filter(j => batchJobIds.includes(j.id));
                const pendingCount = relevantJobs.filter(j => j.status === 'pending' || j.status === 'processing').length;
                
                // Ensure we found the jobs and none are pending
                const foundIds = relevantJobs.map(j => j.id);
                // We only care about jobs we just created. If they are not in the list yet, we wait.
                // Note: If ListJobs has a limit (50), and we uploaded, they should be at the top.
                const allFound = batchJobIds.every(id => foundIds.includes(id));

                if (allFound && pendingCount === 0) {
                    allCompleted = true;
                }
            } else if (res.status === 401) {
                 console.error("Unauthorized polling jobs - redirecting");
                 navigate('/login');
                 return; // Stop loop
            }
        } catch (e) {
            console.error("Error polling jobs", e);
            // Continue polling on error
        }
    }
    toast.dismiss(processingToastId);
    
    // Notify user about View Refresh phase (Generate Once)
    const toastId = toast.loading("Consolidando dados e gerando relatórios... Aguarde.");

    try {
        const refreshRes = await fetch('/api/admin/refresh-views', {
            method: 'POST',
            headers: {
                'Authorization': `Bearer ${token}`
            }
        });
        
        toast.dismiss(toastId);

        if (!refreshRes.ok) {
            console.error("Erro ao atualizar views");
            toast.warning("Importação concluída, mas houve um atraso na atualização dos relatórios.");
        } else {
            toast.success("Importação Total Concluída! Redirecionando...");
            // Redirect to Commercial Dashboard as requested
            setTimeout(() => {
                navigate('/mercadorias?tab=comercial');
            }, 1500);
        }
    } catch (e) {
        toast.dismiss(toastId);
        console.error("Erro de conexão ao atualizar views", e);
        toast.error("Erro ao conectar para atualização de relatórios.");
    } finally {
        setIsUploading(false);
        setCurrentFileIndex(-1);
        setSelectedFiles([]); // Clear selection after all done
        setScanStats({ scanned: 0, relevant: 0, phase: 'idle' });
    }
  };

  const handleCancelJob = async (id: string) => {
    try {
      const res = await fetch(`/api/jobs/${id}/cancel`, { method: 'POST' });
      if (res.ok) {
        toast.info('Cancelamento solicitado. O processo será interrompido em breve.');
      } else {
        toast.error('Erro ao solicitar cancelamento.');
      }
    } catch (error) {
      console.error('Error cancelling job:', error);
      toast.error('Erro ao conectar com o servidor.');
    }
  };

  const handleResetDatabase = async () => {
    if (!window.confirm('ATENÇÃO: Tem certeza que deseja APAGAR TODOS os dados importados? Essa ação não pode ser desfeita.')) {
        return;
    }

    try {
        const authToken = token || localStorage.getItem('token');
        const res = await fetch('/api/admin/reset-db', { 
            method: 'DELETE',
            headers: {
                'Authorization': `Bearer ${authToken}`
            }
        });
        if (res.ok) {
            toast.success('Base de dados limpa com sucesso!');
            setJobs([]); // Clear list immediately
        } else {
            toast.error('Erro ao limpar base de dados.');
        }
    } catch (error) {
        console.error('Error resetting database:', error);
        toast.error('Erro de conexão.');
    }
  };

  const handleResetCompanyData = async () => {
    if (!companyId) {
        toast.error("Erro: Identificador da empresa não encontrado. Tente logar novamente.");
        return;
    }
    
    const displayInfo = cnpj ? `(CNPJ: ${cnpj})` : `(ID: ${companyId.substring(0,8)}...)`;

    if (!window.confirm(`ATENÇÃO: Deseja APAGAR TODOS os dados da empresa ${company || 'selecionada'} ${displayInfo}? Essa ação não pode ser desfeita.`)) {
        return;
    }

    try {
        const authToken = token || localStorage.getItem('token');
        const res = await fetch('/api/company/reset-data', { 
            method: 'DELETE',
            headers: {
                'Authorization': `Bearer ${authToken}`,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ company_id: companyId })
        });

        if (res.ok) {
            toast.success(`Dados da empresa ${company} limpos com sucesso!`);
            const authToken = token || localStorage.getItem('token');
            const jobsRes = await fetch('/api/jobs', {
                headers: { 'Authorization': `Bearer ${authToken}` }
            });
            if (jobsRes.ok) {
                const data = await jobsRes.json();
                setJobs(data);
            }
        } else {
            const err = await res.text();
            toast.error(`Erro ao limpar dados: ${err}`);
        }
    } catch (error) {
        console.error('Error resetting company data:', error);
        toast.error('Erro de conexão.');
    }
  };

  return (
    <div className="container mx-auto p-6 space-y-6 animate-fade-in">
      <div className="flex justify-between items-start">
        <div className="flex flex-col gap-2">
            <h1 className="text-3xl font-bold tracking-tight text-primary">Importar EFD <span className="text-sm font-normal text-muted-foreground">(Multi-File Upload)</span></h1>
            <p className="text-muted-foreground">
            Envie seus arquivos SPED EFD Contribuições para processamento. Selecione múltiplos arquivos de uma vez.
            </p>
        </div>
        
        <div className="flex gap-2">
            {user?.role === 'admin' && (
                <Button variant="destructive" size="sm" onClick={handleResetDatabase} className="gap-2">
                    <Trash2 className="h-4 w-4" />
                    Zerar Tudo (Admin)
                </Button>
            )}

            {companyId && (
                <Button variant="outline" size="sm" onClick={handleResetCompanyData} className="gap-2 border-red-200 hover:bg-red-50 text-red-600">
                    <Trash2 className="h-4 w-4" />
                    Limpar {company}
                </Button>
            )}
        </div>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Upload Card */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Upload className="h-5 w-5" />
              Novos Arquivos
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div
              className={`
                border-2 border-dashed rounded-lg p-8 text-center cursor-pointer transition-colors
                ${isDragOver ? 'border-primary bg-primary/5' : 'border-muted-foreground/25 hover:border-primary/50'}
              `}
              onDragOver={handleDragOver}
              onDragLeave={handleDragLeave}
              onDrop={handleDrop}
            >
              <input
                type="file"
                ref={fileInputRef}
                className="hidden"
                accept=".txt"
                multiple
                onChange={handleFileChange}
              />
              <input
                type="file"
                ref={folderInputRef}
                className="hidden"
                {...({ webkitdirectory: "", directory: "" } as any)}
                onChange={handleFileChange}
              />
              
              <div className="flex flex-col items-center gap-2">
                <div className="p-4 bg-muted rounded-full">
                  <FolderOpen className="h-8 w-8 text-muted-foreground" />
                </div>
                <div className="space-y-1">
                  <p className="text-sm font-medium">
                    Arraste arquivos aqui
                  </p>
                  <div className="flex gap-2 justify-center mt-2">
                    <Button 
                        variant="secondary" 
                        size="sm"
                        onClick={(e) => {
                            e.stopPropagation();
                            fileInputRef.current?.click();
                        }}
                    >
                        Selecionar Arquivos
                    </Button>
                    <Button 
                        variant="secondary" 
                        size="sm"
                        onClick={(e) => {
                            e.stopPropagation();
                            folderInputRef.current?.click();
                        }}
                    >
                        Selecionar Pasta
                    </Button>
                  </div>
                  <p className="text-xs text-muted-foreground mt-2">
                    Suporta arquivos .txt (SPED)
                  </p>
                </div>
              </div>
            </div>

            {selectedFiles.length > 0 && (
                <div className="space-y-2 max-h-60 overflow-y-auto pr-2">
                    <div className="flex justify-between items-center pb-2 border-b">
                        <span className="text-sm font-semibold">{selectedFiles.length} arquivos selecionados</span>
                        <Button 
                            size="sm" 
                            onClick={handleUploadAll}
                            disabled={isUploading}
                        >
                            {isUploading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Upload className="h-4 w-4 mr-2" />}
                            {isUploading ? 'Enviando...' : 'Enviar Todos'}
                        </Button>
                    </div>
                    
                    {selectedFiles.map((file, idx) => (
                        <div key={idx} className={`flex items-center justify-between p-2 rounded-md text-sm ${idx === currentFileIndex ? 'bg-primary/10 border border-primary/20' : 'bg-muted/50'}`}>
                             <div className="flex items-center gap-2 overflow-hidden flex-1 min-w-0">
                                <FileText className="h-3 w-3 flex-shrink-0 text-muted-foreground" />
                                <span className="truncate flex-1 min-w-0" title={file.name}>{file.name}</span>
                                <span className="text-xs text-muted-foreground flex-shrink-0">({(file.size / 1024 / 1024).toFixed(2)} MB)</span>
                             </div>
                             
                             {idx === currentFileIndex && isUploading && (
                                 <Badge variant="secondary" className="text-xs">Processando...</Badge>
                             )}
                             
                             {!isUploading && (
                                 <Button variant="ghost" size="icon" className="h-6 w-6" onClick={() => removeFile(idx)}>
                                     <XCircle className="h-4 w-4 text-muted-foreground hover:text-red-500" />
                                 </Button>
                             )}
                        </div>
                    ))}
                </div>
            )}

            {scanStats.phase === 'scanning' && (
              <div className="text-xs text-muted-foreground mt-2 text-center animate-pulse">
                🔍 Analisando e Filtrando Arquivo Atual... <br/>
                {scanStats.scanned.toLocaleString()} linhas lidas | {scanStats.relevant.toLocaleString()} registros mantidos
              </div>
            )}

            <UploadProgressDisplay 
              progress={uploadProgress} 
              fileName={currentFileIndex >= 0 ? selectedFiles[currentFileIndex]?.name : ''}
            />
          </CardContent>
        </Card>

        {/* Recent Jobs Card */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-5 w-5" />
              Processamentos Recentes
            </CardTitle>
          </CardHeader>
          <CardContent>
            {jobs.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">
                <p>Nenhum processamento recente.</p>
              </div>
            ) : (
              <div className="space-y-2 max-h-[500px] overflow-y-auto pr-2">
                {jobs.map((job) => (
                  <div key={job.id} className="flex items-center justify-between p-3 border rounded-lg gap-3">
                    <div className="flex items-center gap-3 flex-1 min-w-0">
                      {job.status === 'completed' && <CheckCircle className="h-4 w-4 text-green-500 flex-shrink-0" />}
                      {job.status === 'processing' && <Loader2 className="h-4 w-4 text-blue-500 animate-spin flex-shrink-0" />}
                      {job.status === 'error' && <XCircle className="h-4 w-4 text-red-500 flex-shrink-0" />}
                      {job.status === 'pending' && <Clock className="h-4 w-4 text-gray-500 flex-shrink-0" />}
                      
                      <div className="flex flex-col flex-1 min-w-0">
                        <div className="flex justify-between items-center mb-1 gap-2">
                          <span className="text-sm font-medium truncate flex-1 min-w-0" title={job.filename}>{job.filename}</span>
                          <div className="flex items-center gap-2 flex-shrink-0">
                            <span className="text-xs text-muted-foreground whitespace-nowrap">
                              {new Date(job.created_at).toLocaleString()}
                            </span>
                            {job.status === 'processing' && (
                              <Button
                                variant="ghost"
                                size="icon"
                                className="h-6 w-6 text-muted-foreground hover:text-destructive"
                                onClick={() => handleCancelJob(job.id)}
                                title="Cancelar Importação"
                              >
                                <XCircle className="h-4 w-4" />
                              </Button>
                            )}
                          </div>
                        </div>
                        
                        {job.status === 'processing' && job.message && (
                          <div className="space-y-1">
                            <div className="flex justify-between text-xs text-muted-foreground">
                              <span>{job.message}</span>
                            </div>
                            {(() => {
                              const match = job.message.match(/\(([\d.]+)%\)/);
                              const percent = match ? parseFloat(match[1]) : 0;
                              return (
                                <div className="h-1.5 w-full bg-secondary rounded-full overflow-hidden">
                                  <div 
                                    className="h-full bg-primary transition-all duration-500 ease-in-out" 
                                    style={{ width: `${percent}%` }}
                                  />
                                </div>
                              );
                            })()}
                          </div>
                        )}
                        
                        {job.status !== 'processing' && (
                           <span className="text-xs text-muted-foreground truncate" title={job.message}>
                             {job.message || 'Aguardando...'}
                           </span>
                        )}
                      </div>
                    </div>
                    <Badge variant={
                      job.status === 'completed' ? 'default' : 
                      job.status === 'error' ? 'destructive' : 'secondary'
                    }>
                      {job.status}
                    </Badge>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}