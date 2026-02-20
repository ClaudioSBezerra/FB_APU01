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
  Clock,
  BarChart3
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

// Menu definition
const menuItems = [
  {
    title: "Configurações e Tabelas",
    icon: Settings,
    items: [
      { title: "Tabela de Alíquotas", url: "/config/aliquotas", icon: Table },
      { title: "Tabela CFOP", url: "/config/cfop", icon: Table },
      { title: "Simples Nacional", url: "/config/forn-simples", icon: Users },
      { title: "Gestão de Usuários", url: "/config/usuarios", icon: Users, adminOnly: true },
      { title: "Gestores de Relatórios", url: "/config/gestores", icon: Users },
      { title: "Gestão de Ambiente", url: "/config/ambiente", icon: Building },
    ]
  },
  {
    title: "Simulador da RT",
    icon: Calculator,
    isActive: true, // Default open
    items: [
      { title: "Importar SPEDs", url: "/importar-efd", icon: FileSpreadsheet },
      { title: "Operações Comerciais", url: "/mercadorias?tab=comercial", icon: ShoppingCart },
      { title: "Operações Simples Nacional", url: "/operacoes/simples", icon: Store },
      { title: "Dashboard Reforma", url: "/dashboards", icon: LayoutDashboard },
      { title: "Resumo Executivo IA", url: "/relatorios/resumo-executivo", icon: Sparkles },
    ]
  },
  {
    title: "Apuração Assistida",
    icon: FileText,
    isActive: true,
    items: [
      { title: "Gestão Créditos IBS/CBS", url: "/rfb/gestao-creditos", icon: BarChart3 },
      { title: "Importar XMLs Entrada", url: "/apuracao/entrada", icon: Upload, disabled: true },
      { title: "Importar XMLs Saída", url: "/apuracao/saida", icon: Upload, disabled: true },
      { title: "Importar XMLs NFS-e", url: "/apuracao/nfse", icon: FileText, disabled: true },
    ]
  },
  {
    title: "Conectar Receita Federal",
    icon: Globe,
    items: [
      { title: "Credenciais API", url: "/rfb/credenciais", icon: Globe },
      { title: "Débitos CBS mês corrente", url: "/rfb/apuracao", icon: Download },
      { title: "Créditos CBS mês corrente", url: "/rfb/creditos-cbs", icon: CreditCard, disabled: true },
      { title: "Pagamentos CBS mês corrente", url: "/rfb/pagamentos-cbs", icon: Wallet, disabled: true },
      { title: "Pagamentos CBS a Fornecedores", url: "/rfb/pagamentos-fornecedores", icon: Truck, disabled: true },
      { title: "Concluir apuração mês anterior", url: "/rfb/concluir-apuracao", icon: CheckCircle, disabled: true },
    ]
  }
]

export function AppSidebar() {
  const location = useLocation();
  const { user, company, logout } = useAuth();
  const isAdmin = user?.role === 'admin';

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader>
        <div className="flex items-center gap-1 px-2 py-2">
          <div className="flex aspect-square size-6 md:size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Database className="size-3 md:size-4" />
          </div>
          <div className="grid flex-1 text-left">
            <span className="truncate font-semibold text-xs md:text-sm">FB_APU01</span>
            <span className="truncate text-[8px] md:text-xs">Tax Reform System</span>
          </div>
        </div>
        <div className="px-2 pb-1">
           {isAdmin && <CompanySwitcher />}
        </div>
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>Módulos</SidebarGroupLabel>
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
                      <SidebarMenuButton tooltip={item.title} className="h-auto py-1.5 px-2 whitespace-normal text-[10px] md:text-xs md:px-4">
                        {item.icon && <item.icon />}
                        <span className="leading-tight text-xs md:text-xs">{item.title}</span>
                        <ChevronRight className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                      </SidebarMenuButton>
                    </CollapsibleTrigger>
                    <CollapsibleContent>
                      <SidebarMenuSub>
                        {item.items.map((subItem) => {
                           // Check for adminOnly
                           // Note: TypeScript might complain if adminOnly is not in the type definition of subItem, 
                           // but since menuItems is inferred, it should be fine if at least one item has it.
                           // However, to be safe and cleaner:
                           if ((subItem as any).adminOnly && !isAdmin) return null;
                           
                           return (
                          <SidebarMenuSubItem key={subItem.title}>
                            <SidebarMenuSubButton 
                              asChild 
                              isActive={location.pathname === subItem.url}
                              className={`h-auto py-1.5 whitespace-normal leading-tight text-xs ${subItem.disabled ? "opacity-50 pointer-events-none" : ""} ${(subItem as any).className || ""}`}
                            >
                              <Link to={subItem.url}>
                                {subItem.icon && <subItem.icon className="mr-2 h-3 w-3 shrink-0" />}
                                <span className="text-xs">{subItem.title}</span>
                                {subItem.disabled && <span className="ml-auto text-[8px] text-muted-foreground">(Dev)</span>}
                              </Link>
                            </SidebarMenuSubButton>
                          </SidebarMenuSubItem>
                        )})}
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
            <div className="flex flex-col gap-1 p-1 bg-sidebar-accent/50 rounded-md">
              <div className="text-[10px] italic truncate">
                {company || "Empresa não identificada"}
              </div>
              <div className="flex flex-col gap-1">
                <div className="text-[10px] truncate">{user.full_name}</div>
                <div className="bg-yellow-100 text-yellow-700 px-1 py-0.5 rounded text-[8px] font-medium border border-yellow-200 self-start">
                   Vencimento: {new Date(user.trial_ends_at).toLocaleDateString()}
                </div>
              </div>
              <Button variant="ghost" size="sm" className="w-full justify-start h-5 px-0 text-muted-foreground hover:text-foreground" onClick={logout}>
                <LogOut className="mr-2 h-3 w-3" />
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