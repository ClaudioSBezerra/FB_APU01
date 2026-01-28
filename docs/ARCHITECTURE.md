# Arquitetura do Sistema - FB_APU01

## Diagrama de Fluxo de Dados (Data Flow)

```mermaid
sequenceDiagram
    participant U as Usuário
    participant FE as Frontend (React)
    participant NG as Nginx (Proxy)
    participant API as Backend (Go)
    participant DB as PostgreSQL
    participant FS as File System
    participant W as Worker (Go Routine)

    U->>FE: Seleciona Arquivo SPED
    FE->>NG: POST /api/upload (Multipart)
    NG->>API: Proxy Pass
    API->>FS: Salva Arquivo em Disco
    API->>DB: INSERT INTO import_jobs (pending)
    API-->>FE: Retorna JobID
    
    loop Polling de Status
        FE->>API: GET /api/jobs/{id}
        API->>DB: SELECT status FROM import_jobs
        DB-->>API: status
        API-->>FE: status (processing/completed)
    end

    par Background Processing
        W->>DB: SELECT * FROM import_jobs WHERE status='pending'
        W->>DB: UPDATE status='processing'
        W->>FS: Lê Arquivo (Stream)
        W->>W: Parse & Convert (Latin1->UTF8)
        W->>DB: Transaction Begin
        W->>DB: BATCH INSERT participants
        W->>DB: Transaction Commit
        W->>DB: UPDATE status='completed'
    end
```

## Estrutura de Banco de Dados (ERD Simplificado)

```mermaid
erDiagram
    IMPORT_JOBS {
        uuid id PK
        string filename
        string status
        string message
        timestamp created_at
        timestamp updated_at
    }
    PARTICIPANTS {
        uuid id PK
        uuid job_id FK
        string cod_part
        string nome
        string cnpj
        string cpf
        string ie
        string cod_mun
        string suframa
        string endereco
        string numero
        string complemento
        string bairro
    }
    IMPORT_JOBS ||--|{ PARTICIPANTS : "contém"
```

## Componentes Principais

### 1. API Gateway / Reverse Proxy (Nginx)
Atua como ponto de entrada único, servindo o frontend estático e encaminhando requisições `/api` para o backend. Configurado para aceitar grandes payloads (necessário para arquivos SPED de 100MB+).

### 2. Fiscal Engine (Go)
O coração do sistema. Diferente de soluções em Node.js ou Python, o motor em Go utiliza tipagem estática e compilação nativa para processar gigabytes de texto em segundos.
- **Worker Pool**: Gerencia a carga de processamento sem bloquear a API principal.
- **Streaming Decoder**: Processa arquivos maiores que a memória RAM disponível.

### 3. Camada de Persistência (PostgreSQL)
Banco de dados relacional robusto. Utiliza chaves estrangeiras (`ON DELETE CASCADE`) para garantir integridade referencial (se um Job é deletado, seus dados importados também são).