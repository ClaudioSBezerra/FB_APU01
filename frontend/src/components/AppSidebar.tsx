import {
  Table,
  Users,
  Building,
  FileSpreadsheet,
  ShoppingCart,
  LayoutDashboard,
  FileText,
  Upload,
  Globe,
  Download,
  LogOut,
  Store,
  Sparkles,
  CreditCard,
  Wallet,
  Truck,
  CheckCircle,
  BarChart3,
  Tag,
  ShieldAlert,
  Search,
} from "lucide-react"
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  SidebarHeader,
  SidebarFooter,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { Link, useLocation } from "react-router-dom"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/contexts/AuthContext"
import { CompanySwitcher } from "@/components/CompanySwitcher"
import { cn } from "@/lib/utils"

// ---------------------------------------------------------------------------
// Tipos
// ---------------------------------------------------------------------------
interface NavItem {
  title: string;
  url: string;
  icon: React.ElementType;
  disabled?: boolean;
  adminOnly?: boolean;
  danger?: boolean;
}

interface NavSection {
  id: string;
  title: string;
  dot: string;   // Tailwind bg-* color for the dot indicator
  items: NavItem[];
}

// ---------------------------------------------------------------------------
// Definição das seções (estrutura flat — sem subgrupos aninhados)
// ---------------------------------------------------------------------------
const sections: NavSection[] = [
  {
    id: "config",
    title: "Configurações e Tabelas",
    dot: "bg-slate-400",
    items: [
      { title: "Tabela de Alíquotas",    url: "/config/aliquotas",        icon: Table },
      { title: "Tabela CFOP",             url: "/config/cfop",              icon: Table },
      { title: "Simples Nacional",        url: "/config/forn-simples",      icon: Store },
      { title: "Apelidos de Filiais",     url: "/config/apelidos-filiais",  icon: Tag },
      { title: "Gestores de Relatórios",  url: "/config/gestores",          icon: Users },
      { title: "Gestão de Ambiente",      url: "/config/ambiente",          icon: Building },
      { title: "Gestão de Usuários",      url: "/config/usuarios",          icon: Users, adminOnly: true },
    ],
  },
  {
    id: "simulador",
    title: "Simulador da Reforma Tributária",
    dot: "bg-emerald-400",
    items: [
      { title: "Importar SPEDs",              url: "/importar-efd",                    icon: FileSpreadsheet },
      { title: "Operações Comerciais",         url: "/mercadorias",                     icon: ShoppingCart },
      { title: "Operações Simples Nacional",   url: "/operacoes/simples",               icon: Store },
      { title: "Dashboard Reforma",            url: "/dashboards",                      icon: LayoutDashboard },
      { title: "Resumo Executivo IA",          url: "/relatorios/resumo-executivo",     icon: Sparkles },
      { title: "Consulta Inteligente",         url: "/relatorios/consulta-inteligente", icon: Search },
    ],
  },
  {
    id: "importar",
    title: "Apuração Assistida — Importar",
    dot: "bg-violet-400",
    items: [
      { title: "Entradas Mod. 55",        url: "/apuracao/entrada",  icon: Upload },
      { title: "Saídas Mod. 55/65",       url: "/apuracao/saida",    icon: Upload },
      { title: "Serviços — Entradas",     url: "#",                       icon: Upload, disabled: true },
      { title: "Serviços — Saídas",       url: "#",                       icon: Upload, disabled: true },
      { title: "CT-e — Entradas",         url: "/apuracao/cte-entrada",   icon: Upload },
    ],
  },
  {
    id: "consultar",
    title: "Apuração Assistida — Consultar",
    dot: "bg-violet-400",
    items: [
      { title: "Entradas Mod. 55",        url: "/apuracao/entrada/notas",     icon: FileText },
      { title: "Saídas Mod. 55/65",       url: "/apuracao/saida/notas",       icon: FileText },
      { title: "Serviços — Entradas",     url: "#",                                   icon: FileText, disabled: true },
      { title: "Serviços — Saídas",       url: "#",                                   icon: FileText, disabled: true },
      { title: "CT-e — Entradas",         url: "/apuracao/cte-entrada/notas",         icon: FileText },
      { title: "Créditos em Risco",       url: "/apuracao/creditos-perdidos",  icon: ShieldAlert, danger: true },
    ],
  },
  {
    id: "rfb",
    title: "Receita Federal",
    dot: "bg-orange-400",
    items: [
      { title: "Gestão Créditos IBS/CBS",       url: "/rfb/gestao-creditos",         icon: BarChart3 },
      { title: "Credenciais API RFB",            url: "/rfb/credenciais",             icon: Globe },
      { title: "Débitos CBS — mês corrente",     url: "/rfb/apuracao",                icon: Download },
      { title: "Créditos CBS — mês corrente",    url: "/rfb/creditos-cbs",            icon: CreditCard,  disabled: true },
      { title: "Pagamentos CBS — mês corrente",  url: "/rfb/pagamentos-cbs",          icon: Wallet,      disabled: true },
      { title: "Pgtos CBS a Fornecedores",       url: "/rfb/pagamentos-fornecedores", icon: Truck,       disabled: true },
      { title: "Concluir apuração mês ant.",     url: "/rfb/concluir-apuracao",       icon: CheckCircle, disabled: true },
    ],
  },
]

// ---------------------------------------------------------------------------
// AppSidebar
// ---------------------------------------------------------------------------
export function AppSidebar() {
  const location = useLocation()
  const { user, company, logout } = useAuth()
  const isAdmin = user?.role === "admin"

  function isActive(url: string) {
    if (url === "#") return false
    return location.pathname === url.split("?")[0]
  }

  return (
    <Sidebar collapsible="icon">
      {/* ── Header ── */}
      <SidebarHeader className="border-b pb-2">
        <div className="flex items-center gap-2.5 px-3 py-2">
          <img
            src="/favicon-fc.png"
            alt="Ferreira Costa"
            className="size-8 rounded-lg shrink-0 object-cover"
          />
          <div className="grid flex-1 text-left leading-tight">
            <span className="font-bold text-sm truncate">FBTax Cloud</span>
            <span className="text-[10px] text-muted-foreground truncate">Tax Reform System</span>
          </div>
          <SidebarTrigger className="ml-auto h-6 w-6 shrink-0" />
        </div>
        {isAdmin && (
          <div className="px-3 pb-1">
            <CompanySwitcher />
          </div>
        )}
      </SidebarHeader>

      {/* ── Conteúdo ── */}
      <SidebarContent>
        {sections.map((section, idx) => {
          const visibleItems = section.items.filter(
            (item) => !item.adminOnly || isAdmin
          )
          if (visibleItems.length === 0) return null

          return (
            <SidebarGroup key={section.id} className={idx > 0 ? "pt-0" : ""}>
              {/* Separador entre seções */}
              {idx > 0 && <div className="border-t mx-2 mb-2 mt-1" />}

              {/* Label da seção */}
              <SidebarGroupLabel className="flex items-center gap-1.5 px-3 py-1 text-[10px] uppercase tracking-wider font-semibold text-muted-foreground/80">
                <span className={cn("h-1.5 w-1.5 rounded-full shrink-0", section.dot)} />
                {section.title}
              </SidebarGroupLabel>

              <SidebarGroupContent>
                <SidebarMenu>
                  {visibleItems.map((item) => (
                    <SidebarMenuItem key={item.title}>
                      {item.disabled ? (
                        /* Item desabilitado (em desenvolvimento) */
                        <SidebarMenuButton
                          className="h-8 px-3 opacity-45 pointer-events-none"
                          tooltip={`${item.title} (em desenvolvimento)`}
                        >
                          <item.icon className="h-4 w-4 shrink-0" />
                          <span className="text-xs">{item.title}</span>
                          <span className="ml-auto text-[9px] bg-muted text-muted-foreground px-1 py-0.5 rounded font-normal">
                            dev
                          </span>
                        </SidebarMenuButton>
                      ) : (
                        /* Item ativo */
                        <SidebarMenuButton
                          asChild
                          isActive={isActive(item.url)}
                          tooltip={item.title}
                          className={cn(
                            "h-8 px-3",
                            item.danger && [
                              "text-red-600 hover:text-red-700 hover:bg-red-50",
                              "dark:hover:bg-red-950/20 font-semibold",
                            ],
                            isActive(item.url) && item.danger && [
                              "bg-red-50 dark:bg-red-950/20 text-red-700",
                            ],
                          )}
                        >
                          <Link to={item.url}>
                            <item.icon className="h-4 w-4 shrink-0" />
                            <span className="text-xs">{item.title}</span>
                          </Link>
                        </SidebarMenuButton>
                      )}
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          )
        })}
      </SidebarContent>

      {/* ── Footer ── */}
      <SidebarFooter className="border-t">
        {user && (
          <div className="p-2">
            <div className="flex flex-col gap-1 px-2 py-2 bg-sidebar-accent/60 rounded-lg">
              <p className="text-[10px] italic truncate text-muted-foreground leading-tight">
                {company || "Empresa não identificada"}
              </p>
              <p className="text-xs font-medium truncate leading-tight">{user.full_name}</p>
              <span className="self-start bg-yellow-100 text-yellow-700 border border-yellow-200 px-1.5 py-0.5 rounded text-[9px] font-medium">
                Vence: {new Date(user.trial_ends_at).toLocaleDateString("pt-BR")}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="w-full justify-start h-7 px-1 text-muted-foreground hover:text-foreground mt-0.5"
                onClick={logout}
              >
                <LogOut className="mr-2 h-3.5 w-3.5" />
                <span className="text-xs">Sair</span>
              </Button>
            </div>
          </div>
        )}
      </SidebarFooter>

      <SidebarRail />
    </Sidebar>
  )
}
