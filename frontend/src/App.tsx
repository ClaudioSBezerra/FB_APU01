import { useState, useEffect } from 'react'
import { FileUpload } from './components/FileUpload'
import { ParticipantList } from './components/ParticipantList'

function App() {
  const [status, setStatus] = useState('Conectando ao backend...')
  const [completedJobId, setCompletedJobId] = useState<string | null>(null)

  useEffect(() => {
    fetch('/api/health')
      .then(async res => {
        const contentType = res.headers.get("content-type");
        if (!res.ok) {
          throw new Error(`HTTP error! status: ${res.status}`);
        }
        if (contentType && contentType.indexOf("application/json") !== -1) {
          return res.json();
        } else {
          const text = await res.text();
          throw new Error(`Resposta não é JSON: ${text.substring(0, 50)}...`);
        }
      })
      .then(data => setStatus(`Backend Status: ${data.status} | Service: ${data.service}`))
      .catch(err => setStatus('Erro ao conectar ao backend: ' + err.message))
  }, [])

  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-gray-100 p-4">
      <h1 className="text-3xl font-bold mb-4">FB_APU01</h1>
      <p className="text-xl text-gray-700">Sistema de Apuração Assistida (VPS/Go)</p>
      
      <div className="mt-8 p-4 bg-white rounded shadow-md w-full max-w-md">
        <h2 className="text-lg font-semibold mb-2">Status do Sistema:</h2>
        <code className="bg-gray-800 text-green-400 p-2 rounded block text-sm overflow-x-auto">
          {status}
        </code>
      </div>

      <FileUpload onUploadComplete={setCompletedJobId} />
      
      {completedJobId && <ParticipantList jobId={completedJobId} />}
    </div>
  )
}

export default App