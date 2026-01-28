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

- [ ] **Conexão GitHub -> Coolify**: Adicionar repositório.
- [ ] **Configuração de Serviços**:
  - [ ] Banco de Dados (Postgres).
  - [ ] Redis.
  - [ ] Aplicação (Docker Compose).
- [ ] **Variáveis de Ambiente**: Configurar segredos de produção.

## 📊 Épico 4: Monitoramento e Observabilidade
**Objetivo:** Manter a saúde do sistema.

- [ ] **Painel Coolify**: Monitoramento de recursos.
- [ ] **Health Checks**: Alertas de disponibilidade.
- [ ] **Backups**: Rotina de segurança dos dados.