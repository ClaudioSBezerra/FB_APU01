import {
  Settings,
  Table,
  Users,
  Building,
  Calculator,
  FileSpreadsheet,
  ShoppingCart,
  LayoutDashboard,
  FileText,
  Upload,
  Globe,
  Download,
  ChevronRight,
  Database,
  LogOut,
  Store,
  Sparkles,
  CreditCard,
  Wallet,
  Truck,
  CheckCircle,
  BarChart3,
  Tag
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
  SidebarMenuSub,
  SidebarMenuSubItem,
  SidebarMenuSubButton,
  SidebarRail,
  SidebarHeader,
  SidebarFooter
} from "@/components/ui/sidebar"
import { Link, useLocation } from "react-router-dom"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/contexts/AuthContext"
import { CompanySwitcher } from "@/components/CompanySwitcher"

// ---------------------------------------------------------------------------
// Tipos
// ---------------------------------------------------------------------------
interface SubItem {
  title: string;
  url: string;
  icon?: React.ElementType;
  disabled?: boolean;
  adminOnly?: boolean;
  className?: string;
}

interface SubGroup {
  title: string;
  icon?: React.ElementType;
  isActive?: boolean;
  items: SubItem[];
}

interface MenuItem {
  title: string;
  icon: React.ElementType;
  isActive?: boolean;
  items?: SubItem[];
  subGroups?: SubGroup[];
}

// ---------------------------------------------------------------------------
// Menu definition
// ---------------------------------------------------------------------------
const menuItems: MenuItem[] = [
  {
    title: "Configurações e Tabelas",
    icon: Settings,
    items: [
      { title: "Tabela de Alíquotas",   url: "/config/aliquotas",        icon: Table },
      { title: "Tabela CFOP",            url: "/config/cfop",              icon: Table },
      { title: "Simples Nacional",       url: "/config/forn-simples",      icon: Users },
      { title: "Apelidos de Filiais",    url: "/config/apelidos-filiais",  icon: Tag },
      { title: "Gestão de Usuários",     url: "/config/usuarios",          icon: Users, adminOnly: true },
      { title: "Gestores de Relatórios", url: "/config/gestores",          icon: Users },
      { title: "Gestão de Ambiente",     url: "/config/ambiente",          icon: Building },
    ]
  },
  {
    title: "Simulador da RT",
    icon: Calculator,
    isActive: true,
    items: [
      { title: "Importar SPEDs",             url: "/importar-efd",                    icon: FileSpreadsheet },
      { title: "Operações Comerciais",        url: "/mercadorias?tab=comercial",       icon: ShoppingCart },
      { title: "Operações Simples Nacional",  url: "/operacoes/simples",               icon: Store },
      { title: "Dashboard Reforma",           url: "/dashboards",                      icon: LayoutDashboard },
      { title: "Resumo Executivo IA",         url: "/relatorios/resumo-executivo",     icon: Sparkles },
      { title: "Consulta Inteligente",        url: "/relatorios/consulta-inteligente", icon: Sparkles },
    ]
  },
  {
    title: "Apuração Assistida",
    icon: FileText,
    isActive: true,
    subGroups: [
      {
        title: "Importações de XMLs",
        icon: Upload,
        isActive: true,
        items: [
          { title: "Entradas Mod. 55",   url: "/apuracao/entrada", icon: Upload },
          { title: "Saídas Mod. 55/65",  url: "/apuracao/saida",   icon: Upload },
          { title: "Entrada Serviços",   url: "#", icon: Upload, disabled: true },
          { title: "Saídas Serviços",    url: "#", icon: Upload, disabled: true },
          { title: "Entradas CT-e",      url: "#", icon: Upload, disabled: true },
        ]
      },
      {
        title: "Consultas",
        icon: FileText,
        isActive: true,
        items: [
          { title: "Entradas Mod. 55",   url: "/apuracao/entrada/notas", icon: FileText },
          { title: "Saídas Mod. 55/65",  url: "/apuracao/saida/notas",   icon: FileText },
          { title: "Entrada Serviços",   url: "#", icon: FileText, disabled: true },
          { title: "Saídas Serviços",    url: "#", icon: FileText, disabled: true },
          { title: "Entradas CT-e",      url: "#", icon: FileText, disabled: true },
        ]
      },
    ]
  },
  {
    title: "Apuração da Receita Federal",
    icon: Globe,
    items: [
      { title: "Gestão Créditos IBS/CBS",       url: "/rfb/gestao-creditos",         icon: BarChart3 },
      { title: "Credenciais API",                url: "/rfb/credenciais",             icon: Globe },
      { title: "Débitos CBS mês corrente",       url: "/rfb/apuracao",                icon: Download },
      { title: "Créditos CBS mês corrente",      url: "/rfb/creditos-cbs",            icon: CreditCard,  disabled: true },
      { title: "Pagamentos CBS mês corrente",    url: "/rfb/pagamentos-cbs",          icon: Wallet,      disabled: true },
      { title: "Pagamentos CBS a Fornecedores",  url: "/rfb/pagamentos-fornecedores", icon: Truck,       disabled: true },
      { title: "Concluir apuração mês anterior", url: "/rfb/concluir-apuracao",       icon: CheckCircle, disabled: true },
    ]
  }
]

// ---------------------------------------------------------------------------
// Leaf item
// ---------------------------------------------------------------------------
function SubItemLink({ subItem, isActive }: { subItem: SubItem; isActive: boolean }) {
  return (
    <SidebarMenuSubButton
      asChild
      isActive={isActive}
      className={`h-auto py-1 whitespace-normal leading-tight ${
        subItem.disabled ? "opacity-50 pointer-events-none" : ""
      } ${subItem.className || ""}`}
    >
      <Link to={subItem.url}>
        {subItem.icon && <subItem.icon className="mr-1.5 h-3 w-3 shrink-0" />}
        <span className="text-[11px]">{subItem.title}</span>
        {subItem.disabled && (
          <span className="ml-auto text-[8px] text-muted-foreground">(Dev)</span>
        )}
      </Link>
    </SidebarMenuSubButton>
  );
}

// ---------------------------------------------------------------------------
// AppSidebar
// ---------------------------------------------------------------------------
export function AppSidebar() {
  const location = useLocation();
  const { user, company, logout } = useAuth();
  const isAdmin = user?.role === 'admin';

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <div className="flex items-center gap-1 px-2 py-1.5">
          <div className="flex aspect-square size-6 md:size-7 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Database className="size-3" />
          </div>
          <div className="grid flex-1 text-left">
            <span className="truncate font-semibold text-[11px]">FB_APU01</span>
            <span className="truncate text-[8px] text-muted-foreground">Tax Reform System</span>
          </div>
        </div>
        <div className="px-2 pb-1">
          {isAdmin && <CompanySwitcher />}
        </div>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel className="text-[10px]">Módulos</SidebarGroupLabel>
          <SidebarGroupContent>
            <SidebarMenu>
              {menuItems.map((item) => (
                <Collapsible
                  key={item.title}
                  asChild
                  defaultOpen={item.isActive}
                  className="group/collapsible"
                >
                  <SidebarMenuItem>
                    <CollapsibleTrigger asChild>
                      <SidebarMenuButton
                        tooltip={item.title}
                        className="h-auto py-1 px-2 whitespace-normal md:px-3"
                      >
                        {item.icon && <item.icon className="h-3.5 w-3.5 shrink-0" />}
                        <span className="leading-tight text-[11px]">{item.title}</span>
                        <ChevronRight className="ml-auto h-3 w-3 transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                      </SidebarMenuButton>
                    </CollapsibleTrigger>

                    <CollapsibleContent>
                      <SidebarMenuSub>

                        {/* ── Sub-grupos expansíveis (3º nível) ── */}
                        {item.subGroups?.map((group) => (
                          <Collapsible
                            key={group.title}
                            asChild
                            defaultOpen={group.isActive}
                            className="group/subgroup"
                          >
                            <SidebarMenuSubItem>
                              <CollapsibleTrigger asChild>
                                <SidebarMenuSubButton className="h-auto py-1 whitespace-normal leading-tight font-semibold text-muted-foreground hover:text-foreground">
                                  {group.icon && <group.icon className="mr-1.5 h-3 w-3 shrink-0" />}
                                  <span className="text-[11px]">{group.title}</span>
                                  <ChevronRight className="ml-auto h-3 w-3 transition-transform duration-200 group-data-[state=open]/subgroup:rotate-90" />
                                </SidebarMenuSubButton>
                              </CollapsibleTrigger>
                              <CollapsibleContent>
                                <SidebarMenuSub className="border-l border-sidebar-border ml-2 pl-1 gap-0">
                                  {group.items.map((subItem) => (
                                    <SidebarMenuSubItem key={subItem.title}>
                                      <SubItemLink
                                        subItem={subItem}
                                        isActive={location.pathname === subItem.url}
                                      />
                                    </SidebarMenuSubItem>
                                  ))}
                                </SidebarMenuSub>
                              </CollapsibleContent>
                            </SidebarMenuSubItem>
                          </Collapsible>
                        ))}

                        {/* ── Itens planos (2º nível) ── */}
                        {item.items?.map((subItem) => {
                          if (subItem.adminOnly && !isAdmin) return null;
                          return (
                            <SidebarMenuSubItem key={subItem.title}>
                              <SubItemLink
                                subItem={subItem}
                                isActive={location.pathname === subItem.url}
                              />
                            </SidebarMenuSubItem>
                          );
                        })}

                      </SidebarMenuSub>
                    </CollapsibleContent>
                  </SidebarMenuItem>
                </Collapsible>
              ))}
            </SidebarMenu>
          </SidebarGroupContent>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <div className="p-1 border-t mt-auto">
          {user && (
            <div className="flex flex-col gap-0.5 p-1 bg-sidebar-accent/50 rounded-md">
              <div className="text-[9px] italic truncate text-muted-foreground">
                {company || "Empresa não identificada"}
              </div>
              <div className="text-[10px] truncate">{user.full_name}</div>
              <div className="bg-yellow-100 text-yellow-700 px-1 py-0.5 rounded text-[8px] font-medium border border-yellow-200 self-start">
                Vencimento: {new Date(user.trial_ends_at).toLocaleDateString()}
              </div>
              <Button
                variant="ghost"
                size="sm"
                className="w-full justify-start h-5 px-0 text-muted-foreground hover:text-foreground mt-0.5"
                onClick={logout}
              >
                <LogOut className="mr-1.5 h-3 w-3" />
                <span className="text-[10px]">Sair</span>
              </Button>
            </div>
          )}
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
