# Roadmap do Projeto - Rumo à Produção (Hostinger VPS + Coolify)

Este documento detalha os passos macro (Épicos) para levar o **FB_APU01** do ambiente local para o servidor de produção na Hostinger VPS, utilizando o **Coolify** para gerenciamento.

---

## 🏁 Épico 1: Finalização e Estabilização do MVP (Local)
**Objetivo:** Garantir que o fluxo "Upload -> Processamento -> Visualização" funcione perfeitamente na máquina local.

- [x] Correção de Bugs Críticos (Build Backend).
- [x] Teste de Carga SPED (Leitura e Importação).
- [x] Visualização de Participantes.

## ☁️ Épico 2: Preparação da Infraestrutura (Hostinger VPS)
**Objetivo:** Configurar o servidor VPS (Recomendado: KVM 2 ou superior, Ubuntu 22.04/24.04).

- [x] **Contratação Hostinger**: VPS adquirida.
- [x] **Configuração Inicial**: Resetar senha root ou configurar chave SSH.
- [x] **Acesso SSH**: Validar conexão.
- [x] **Instalação do Coolify**: O painel de controle da nossa infraestrutura.

## 🚀 Épico 3: Pipeline de Deploy Contínuo (CD)
**Objetivo:** Automatizar a atualização do sistema via Git.

- [x] **Conexão GitHub -> Coolify**: Adicionar repositório.
- [x] **Configuração de Serviços**:
  - [x] Banco de Dados (Postgres).
  - [x] Redis.
  - [x] Aplicação (Docker Compose).
- [x] **Variáveis de Ambiente**: Configurar segredos de produção.
- [x] **Deploy em Produção**: Acessível em `http://fbtax.cloud`.

## 📊 Épico 4: Monitoramento e Observabilidade
**Objetivo:** Manter a saúde do sistema.

- [x] **Painel Coolify**: Monitoramento de recursos ativo.
- [x] **Health Checks**: Endpoint `/api/health` validado.
- [ ] **Backups**: Configurar rotina automática no Coolify.

---

## 🔮 Épico 5: Migração do Sistema Completo (Lovable -> Go/React)
**Objetivo:** Migrar as funcionalidades avançadas desenvolvidas no Lovable para nossa infraestrutura proprietária.

- [ ] **Análise do Código Lovable**: Mapear componentes e fluxos.
- [ ] **Migração do Frontend**:
  - [ ] Dashboards analíticos.
  - [ ] Telas de cadastro complexas.
  - [ ] Relatórios fiscais.
- [ ] **Expansão do Backend (Go)**:
  - [ ] Novos endpoints para suportar features do Lovable.
  - [ ] Otimização de queries para grandes volumes de dados.
- [ ] **Integração**: Conectar novo Frontend ao Backend Go existente.