import { useState, useRef, useEffect } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { AlertCircle, CheckCircle, Clock, FileText, Loader2, RefreshCw, Upload, XCircle } from 'lucide-react';
import { toast } from 'sonner';
import { UploadProgressDisplay, UploadProgressType } from '@/components/UploadProgress';

interface ImportJob {
  id: string;
  filename: string;
  status: 'pending' | 'processing' | 'completed' | 'error';
  created_at: string;
  updated_at: string;
  message?: string;
}

export default function ImportarEFD() {
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [isDragOver, setIsDragOver] = useState(false);
  const [uploadProgress, setUploadProgress] = useState<UploadProgressType>({
    status: 'idle',
    percentage: 0,
    bytesUploaded: 0,
    bytesTotal: 0,
    speed: 0,
    remainingTime: 0
  });
  const [jobs, setJobs] = useState<ImportJob[]>([]);
  const [scanStats, setScanStats] = useState({ scanned: 0, relevant: 0, phase: 'idle' });
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Poll jobs list
  useEffect(() => {
    const fetchJobs = async () => {
      try {
        const res = await fetch('/api/jobs'); 
        if (res.ok) {
          const data = await res.json();
          setJobs(data);
        }
      } catch (error) {
        console.error('Error fetching jobs:', error);
      }
    };

    fetchJobs();
    const interval = setInterval(fetchJobs, 2000);
    return () => clearInterval(interval);
  }, []);

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files[0]) {
      setSelectedFile(e.target.files[0]);
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
    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      setSelectedFile(e.dataTransfer.files[0]);
    }
  };

  const handleUpload = async () => {
    if (!selectedFile) return;

    // --- CLIENT-SIDE PARSING & FILTERING (PHASE 2.1 - FULL SCAN) ---
    // Reads file locally, filters only relevant lines, continues until EOF
    // Creates a smaller Payload for upload.
    
    setUploadProgress({
      status: 'uploading',
      percentage: 0,
      bytesUploaded: 0,
      bytesTotal: selectedFile.size, // Initial size estimation
      speed: 0,
      remainingTime: 0
    });

    try {
      // 1. Filter Logic
      const CHUNK_SIZE = 5 * 1024 * 1024; // 5MB Read Buffer
      let offset = 0;
      let buffer = '';
      const relevantPrefixes = ['|0000|', '|0150|', '|C100|', '|C190|', '|C500|', '|C600|', '|D100|', '|D500|', '|D590|', '|D990|'];
      const filteredParts: string[] = [];
      let finishedReading = false;
      let processedBytes = 0;
      let foundD990 = false;
      const startTimeRead = Date.now();

      // Read Loop
      console.log(`[DEBUG] Starting Client-Side Filter for: ${selectedFile.name}`);
      let totalLinesScanned = 0;
      let relevantLinesFound = 0;

      while (offset < selectedFile.size) { // Force read until end of file
        const chunkBlob = selectedFile.slice(offset, offset + CHUNK_SIZE);
        const chunkText = await chunkBlob.text();
        
        // Prepend previous buffer residue
        const fullText = buffer + chunkText;
        const lines = fullText.split('\n');
        
        // Save the last incomplete line for next iteration (unless EOF)
        if (offset + CHUNK_SIZE < selectedFile.size) {
            buffer = lines.pop() || '';
        } else {
            buffer = ''; // End of file
        }

        // Process Lines
        for (const line of lines) {
           totalLinesScanned++;
           // Fast Check: Must start with pipe
           if (!line.startsWith('|')) continue;

           // STOP SCANNING IF |9999| IS FOUND
           if (line.startsWith('|9999|')) {
             console.log(`[DEBUG] Found |9999| at line ${totalLinesScanned}. Stopping scan.`);
             finishedReading = true;
             break;
           }

           // Check against whitelist
           let isRelevant = false;
           for (const p of relevantPrefixes) {
             if (line.startsWith(p)) {
               isRelevant = true;
               
               // DEBUG SAMPLING: Log first 5 occurrences of each register type or every 1000th relevant line
               if (relevantLinesFound < 20 || relevantLinesFound % 5000 === 0) {
                 console.log(`[DEBUG] Keeping Line #${totalLinesScanned}: ${line.substring(0, 50)}...`);
               }

               if (p === '|D990|') {
                 foundD990 = true;
                 console.log(`[DEBUG] FOUND |D990| at Line #${totalLinesScanned}. Keeping it and continuing scan to confirm no duplicates.`);
               }
               break;
             }
           }

           if (isRelevant) {
             filteredParts.push(line + '\n');
             relevantLinesFound++;
           }
        }

        offset += CHUNK_SIZE;
        processedBytes += chunkBlob.size;
        
        // Update UI (Filtering Phase)
        if (totalLinesScanned % 5000 === 0 || offset >= selectedFile.size || finishedReading) {
            setScanStats({ scanned: totalLinesScanned, relevant: relevantLinesFound, phase: 'scanning' });
            // Yield to UI thread to prevent freeze
            await new Promise(r => setTimeout(r, 0));
        }

        if (finishedReading) break;

        setUploadProgress(prev => ({
           ...prev,
           percentage: Math.min(Math.round((processedBytes / selectedFile.size) * 50), 50), // 0-50% is filtering
           speed: (processedBytes / ((Date.now() - startTimeRead)/1000))
        }));
      }

      // Add Artificial Trailer for Backend Compatibility
      console.log(`[DEBUG] Filter Complete. Scanned: ${totalLinesScanned}, Kept: ${filteredParts.length}. D990 Found: ${foundD990}`);
      
      if (foundD990) {
        filteredParts.push('|9999|' + filteredParts.length + '|\n');
      } else {
        // Warning: File didn't reach D990, but we send what we have
        console.warn("File ended before |D990|");
        filteredParts.push('|9999|' + filteredParts.length + '|\n');
      }

      // 2. Create Filtered Blob
      const filteredBlob = new Blob(filteredParts, { type: 'text/plain' });
      const filteredFile = new File([filteredBlob], selectedFile.name, { type: 'text/plain' });
      
      console.log(`Original Size: ${selectedFile.size}, Filtered Size: ${filteredFile.size}`);

      // 3. Upload Filtered File (CHUNKED UPLOAD STRATEGY)
      const startTimeUpload = Date.now();
      const UPLOAD_CHUNK_SIZE = 5 * 1024 * 1024; // 5MB Upload Chunks
      const totalChunks = Math.ceil(filteredFile.size / UPLOAD_CHUNK_SIZE);
      const uploadID = `${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
      
      console.log(`[DEBUG] Starting Chunked Upload. Total Chunks: ${totalChunks}, ID: ${uploadID}`);

      for (let chunkIndex = 0; chunkIndex < totalChunks; chunkIndex++) {
        const start = chunkIndex * UPLOAD_CHUNK_SIZE;
        const end = Math.min(start + UPLOAD_CHUNK_SIZE, filteredFile.size);
        const chunk = filteredFile.slice(start, end);

        const formData = new FormData();
        // IMPORTANT: Pass filename as 3rd argument so backend detects extension (.txt)
        formData.append('file', chunk, selectedFile.name);
        formData.append('is_chunked', 'true');
        formData.append('upload_id', uploadID);
        formData.append('chunk_index', chunkIndex.toString());
        formData.append('total_chunks', totalChunks.toString());
        formData.append('filename', selectedFile.name); // Original name for reference
        
        // Only send metadata on last chunk to trigger validation
        if (chunkIndex === totalChunks - 1) {
             formData.append('expected_lines', filteredParts.length.toString());
             formData.append('expected_size', filteredFile.size.toString());
        }

        // Upload Chunk with Retry Logic
        let responseJson: any = null;
        await new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();
            xhr.open('POST', '/api/upload', true);

            xhr.onload = () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    try {
                        responseJson = JSON.parse(xhr.response);
                    } catch (e) {
                        // ignore if not json
                    }
                    resolve(xhr.response);
                } else {
                    reject(new Error(`Upload failed at chunk ${chunkIndex}: ${xhr.statusText}`));
                }
            };
            xhr.onerror = () => reject(new Error(`Network error at chunk ${chunkIndex}`));
            xhr.send(formData);
        });

        // Last Chunk Verification
        if (chunkIndex === totalChunks - 1 && responseJson && responseJson.detected_lines) {
           const sentLines = filteredParts.length.toString();
           const receivedLines = responseJson.detected_lines;
           
           if (receivedLines !== 'not_found' && sentLines !== receivedLines) {
             toast.warning(`Atenção: Backend reportou ${receivedLines} linhas, mas enviamos ${sentLines}. Verifique a integridade.`);
           } else if (sentLines === receivedLines) {
             toast.success(`Integridade Verificada: ${receivedLines} registros confirmados no servidor.`);
           }
        }

        // Update Progress
        const percentUpload = ((chunkIndex + 1) / totalChunks) * 50; 
        const totalPercent = 50 + percentUpload;
        
        // Calculate speed based on total bytes uploaded so far
        const bytesUploadedSoFar = end;
        const elapsedTime = (Date.now() - startTimeUpload) / 1000;
        const speed = bytesUploadedSoFar / elapsedTime;
        const remainingBytes = filteredFile.size - bytesUploadedSoFar;
        const remainingTime = speed > 0 ? remainingBytes / speed : 0;

        setUploadProgress({
            status: 'uploading',
            percentage: Math.round(totalPercent),
            bytesUploaded: bytesUploadedSoFar,
            bytesTotal: filteredFile.size,
            speed,
            remainingTime
        });
      }

      setUploadProgress(prev => ({ ...prev, status: 'completed', percentage: 100 }));
      toast.success(`Arquivo enviado! (${filteredParts.length} registros).`);
      setSelectedFile(null);
      
      // Trigger job refresh
      const res = await fetch('/api/jobs');
      if (res.ok) {
        const data = await res.json();
        setJobs(data);
      }

    } catch (error) {
      console.error('Upload error:', error);
      setUploadProgress(prev => ({ ...prev, status: 'error', errorMessage: String(error) }));
      toast.error('Erro no processamento/upload. Tente novamente.');
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

  return (
    <div className="container mx-auto p-6 space-y-6 animate-fade-in">
      <div className="flex flex-col gap-2">
        <h1 className="text-3xl font-bold tracking-tight text-primary">Importar EFD <span className="text-sm font-normal text-muted-foreground">(v4.0 Chunked Upload)</span></h1>
        <p className="text-muted-foreground">
          Envie seus arquivos SPED EFD Contribuições para processamento.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        {/* Upload Card */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Upload className="h-5 w-5" />
              Novo Arquivo
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
              onClick={() => fileInputRef.current?.click()}
            >
              <input
                type="file"
                ref={fileInputRef}
                className="hidden"
                accept=".txt"
                onChange={handleFileChange}
              />
              
              <div className="flex flex-col items-center gap-2">
                <div className="p-4 bg-muted rounded-full">
                  <FileText className="h-8 w-8 text-muted-foreground" />
                </div>
                <div className="space-y-1">
                  <p className="text-sm font-medium">
                    {selectedFile ? selectedFile.name : 'Clique para selecionar ou arraste aqui'}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Suporta arquivos .txt (SPED)
                  </p>
                </div>
              </div>
            </div>

            {selectedFile && (
              <div className="flex items-center justify-between p-3 bg-muted rounded-md">
                <div className="flex items-center gap-2 overflow-hidden">
                  <FileText className="h-4 w-4 flex-shrink-0" />
                  <span className="text-sm truncate">{selectedFile.name}</span>
                </div>
                <Button 
                  size="sm" 
                  onClick={handleUpload}
                  disabled={uploadProgress.status === 'uploading'}
                >
                  Enviar
                </Button>
              </div>
            )}

            {scanStats.phase === 'scanning' && (
              <div className="text-xs text-muted-foreground mt-2 text-center animate-pulse">
                🔍 Analisando e Filtrando Arquivo... <br/>
                {scanStats.scanned.toLocaleString()} linhas lidas | {scanStats.relevant.toLocaleString()} registros mantidos
              </div>
            )}

            <UploadProgressDisplay 
              progress={uploadProgress} 
              fileName={selectedFile?.name || ''}
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
              <div className="space-y-2">
                {jobs.map((job) => (
                  <div key={job.id} className="flex items-center justify-between p-3 border rounded-lg">
                    <div className="flex items-center gap-3">
                      {job.status === 'completed' && <CheckCircle className="h-4 w-4 text-green-500" />}
                      {job.status === 'processing' && <Loader2 className="h-4 w-4 text-blue-500 animate-spin" />}
                      {job.status === 'error' && <XCircle className="h-4 w-4 text-red-500" />}
                      {job.status === 'pending' && <Clock className="h-4 w-4 text-gray-500" />}
                      
                      <div className="flex flex-col flex-1 min-w-0">
                        <div className="flex justify-between items-center mb-1">
                          <span className="text-sm font-medium truncate">{job.filename}</span>
                          <div className="flex items-center gap-2">
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