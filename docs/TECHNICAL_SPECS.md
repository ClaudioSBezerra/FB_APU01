# Especificações Técnicas - FB_APU01 (Fiscal Engine)

## 1. Visão Geral do Sistema
O **FB_APU01** é um sistema de alta performance projetado para processamento assíncrono de arquivos fiscais (SPED EFD Contribuições) e apuração tributária no contexto da Reforma Tributária. A arquitetura prioriza escalabilidade, integridade de dados e feedback em tempo real.

## 2. Stack Tecnológico

### 2.1 Backend (Fiscal Engine)
- **Linguagem**: Go (Golang) 1.22+
- **Framework**: Standard Library (`net/http`) para máxima performance e baixo overhead.
- **Drivers & Libs**:
  - `github.com/lib/pq` (v1.10.9): Driver PostgreSQL puro em Go.
  - `golang.org/x/text` (v0.14.0): Tratamento avançado de encoding (ISO-8859-1/Latin1 -> UTF-8).
- **Arquitetura**:
  - **Handlers**: Camada RESTful para gestão de uploads e consultas.
  - **Workers**: Processamento em background via Goroutines (concorrência nativa).
  - **Dependency Injection**: Injeção de dependências para conexões de banco de dados (`*sql.DB`).

### 2.2 Frontend (Client Interface)
- **Framework**: React 18.3.1
- **Build Tool**: Vite 5.2.0 (Build otimizado com ESBuild).
- **Linguagem**: TypeScript 5.2.2 (Tipagem estática estrita).
- **Estilização**: Tailwind CSS 3.4.3 (Utility-first).
- **Componentes**: `lucide-react` (Ícones), `clsx`, `tailwind-merge`.
- **Server**: Nginx (Reverse Proxy & Static Server) com configuração otimizada para grandes payloads (`client_max_body_size 500M`).

### 2.3 Banco de Dados & Armazenamento
- **RDBMS**: PostgreSQL 15 (Alpine Linux).
- **Cache/Queue Support**: Redis (Alpine) para suporte futuro a filas distribuídas e cache de sessão.
- **Persistência**: Volumes Docker (`postgres_data`, `api_uploads`) garantindo durabilidade dos dados.

### 2.4 Infraestrutura & DevOps
- **Containerização**: Docker & Docker Compose V2.
- **Orquestração**: Suporte a deploy em VPS (Hostinger) via Coolify ou Docker Swarm.
- **Rede**: Bridge Network isolada (`fb_net`) para comunicação segura entre containers.

## 3. Especificações de Processamento (SPED)

### 3.1 Fluxo de Ingestão
1. **Upload**: Arquivo recebido via Multipart Form Data (`/api/upload`).
2. **Validação Prévia**: Checagem de extensão (`.txt`, `.xml`) e tamanho.
3. **Fila de Processamento**: Registro na tabela `import_jobs` com status `pending`.
4. **Armazenamento**: Arquivo salvo em disco (volume persistente) com hash/timestamp para evitar colisão.

### 3.2 Lógica de Parsing (Worker)
- **Polling**: Worker monitora a fila `import_jobs` (FIFO).
- **Stream Processing**: Leitura do arquivo linha-a-linha via `bufio.Scanner` (consumo de memória constante O(1), independente do tamanho do arquivo).
- **Encoding**: Conversão on-the-fly de `ISO-8859-1` para `UTF-8`.
- **Extração de Blocos**:
  - `0000`: Identificação da Entidade e Período.
  - `0150`: Cadastro de Participantes (Clientes/Fornecedores).
  - `C100/C170`: Documentos Fiscais (Futuro).
- **Transacionalidade**: Uso de `db.Begin()` e `tx.Commit()` para garantir atomicidade na inserção de milhares de registros.

## 4. Segurança e Performance
- **CGO Disabled**: Build estático do binário Go (`CGO_ENABLED=0`) para portabilidade total e segurança (scratch/alpine).
- **Optimized Builds**: Frontend com Code Splitting e Tree Shaking.
- **Rate Limiting**: (Planejado) Implementação via Nginx ou Middleware Go.
- **Sanitização**: Prepared Statements SQL para prevenção total de SQL Injection.

## 5. Requisitos de Ambiente
- **Hardware Mínimo**: 1 vCPU, 512MB RAM (Graças à eficiência do Go).
- **Hardware Recomendado**: 2 vCPUs, 2GB RAM (Para processamento paralelo de múltiplos arquivos SPED).
- **OS**: Linux (Debian/Alpine) ou Windows (via WSL2/Docker Desktop).