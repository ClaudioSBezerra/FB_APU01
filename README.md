# FB_APU01 - Motor de Apuração Assistida (Reforma Tributária)

## Visão Geral
Este é o novo ambiente de desenvolvimento para o sistema de Apuração Assistida, migrado para arquitetura VPS (Hostinger) com Backend em Go e Frontend em React.

## Estrutura do Projeto
- **/backend**: API e Motor Fiscal desenvolvido em Go (Golang).
- **/frontend**: Interface do usuário em React (Vite + Tailwind).
- **/docs**: Documentação técnica e de arquitetura.
- **/config**: Arquivos de configuração de ambiente e deploy.
- **/tests**: Testes de integração e scripts de validação.

## Arquitetura
- **Infraestrutura**: VPS Hostinger gerenciado via Coolify.
- **Backend**: Go 1.22+ (Alta performance para XMLs).
- **Frontend**: React (SPA).
- **Banco de Dados**: PostgreSQL.
- **Filas**: Redis.

## Como Iniciar
1. Configure o arquivo `.env` baseado em `.env.FB_APU01`.
2. Execute `docker compose --env-file .env.FB_APU01 up -d` na raiz para subir todo o ambiente.