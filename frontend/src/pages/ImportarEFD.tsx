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

    // Extract Metadata for Preview
    let expectedLines = "unknown";
    let headerPreview = "";
    let footerPreview = "";
    
    try {
      // Read Header (First 100 bytes)
      const headerBlob = selectedFile.slice(0, 100);
      headerPreview = await headerBlob.text();
      
      // Read Footer (Last 16KB to skip digital signatures)
      const footerBlob = selectedFile.slice(Math.max(selectedFile.size - 16384, 0));
      footerPreview = await footerBlob.text();
      
      // Look for |9999|count| anywhere in the footer block
      // We use lastIndexOf to find the actual SPED trailer, ignoring appended signatures
      const match = footerPreview.match(/\|9999\|(\d+)\|/g);
      if (match && match.length > 0) {
        // Get the last valid 9999 record found
        const lastMatch = match[match.length - 1];
        const countMatch = lastMatch.match(/\|9999\|(\d+)\|/);
        if (countMatch) {
            expectedLines = countMatch[1];
        }
      }
    } catch (err) {
      console.warn("Could not read file preview:", err);
    }

    if (expectedLines === "unknown") {
       const confirmUpload = window.confirm(
         "AVISO: Não foi possível detectar o registro final '|9999|' neste arquivo.\n\n" +
         "Isso indica que o arquivo pode estar corrompido, incompleto ou em formato inválido.\n" +
         "Preview do Final: " + footerPreview + "\n\n" +
         "Deseja continuar mesmo assim?"
       );
       if (!confirmUpload) return;
    }

    setUploadProgress({
      status: 'uploading',
      percentage: 0,
      bytesUploaded: 0,
      bytesTotal: selectedFile.size,
      speed: 0,
      remainingTime: 0
    });

    // --- SIMPLE STREAMING UPLOAD (XHR) ---
    const startTime = Date.now();
    const formData = new FormData();
    formData.append('file', selectedFile);
    formData.append('filename', selectedFile.name);
    formData.append('expected_lines', expectedLines);
    formData.append('expected_size', selectedFile.size.toString());

    try {
        await new Promise((resolve, reject) => {
            const xhr = new XMLHttpRequest();
            xhr.open('POST', '/api/upload', true);

            // Progress Event
            xhr.upload.onprogress = (e) => {
                if (e.lengthComputable) {
                    const percentage = Math.round((e.loaded / e.total) * 100);
                    const elapsedTime = (Date.now() - startTime) / 1000;
                    const speed = e.loaded / elapsedTime; // bytes per second
                    const remainingBytes = e.total - e.loaded;
                    const remainingTime = speed > 0 ? remainingBytes / speed : 0;

                    setUploadProgress({
                        status: 'uploading',
                        percentage,
                        bytesUploaded: e.loaded,
                        bytesTotal: e.total,
                        speed,
                        remainingTime
                    });
                }
            };

            // Load/Error Events
            xhr.onload = () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                    resolve(xhr.response);
                } else {
                    reject(new Error(xhr.statusText || 'Upload failed'));
                }
            };
            xhr.onerror = () => reject(new Error('Network error during upload'));
            xhr.send(formData);
        });

      setUploadProgress(prev => ({ ...prev, status: 'completed', percentage: 100 }));
      toast.success('Arquivo enviado com sucesso!');
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
      toast.error('Erro no upload. Tente novamente.');
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
        <h1 className="text-3xl font-bold tracking-tight text-primary">Importar EFD <span className="text-sm font-normal text-muted-foreground">(v3.0 Streaming)</span></h1>
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