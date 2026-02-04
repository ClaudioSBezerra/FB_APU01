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
  LogOut
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
      { title: "Gestão de Usuários", url: "/config/usuarios", icon: Users },
      { title: "Gestão de Ambiente", url: "/config/ambiente", icon: Building },
    ]
  },
  {
    title: "Simulador da RT",
    icon: Calculator,
    isActive: true, // Default open
    items: [
      { title: "Importar SPEDs", url: "/importar-efd", icon: FileSpreadsheet },
      { title: "Operações Comerciais", url: "/mercadorias?tab=comercial", icon: ShoppingCart, className: "whitespace-normal h-auto py-2 leading-tight text-xs" },
      { title: "Dashboard Reforma", url: "/dashboards", icon: LayoutDashboard },
    ]
  },
  {
    title: "Apuração Assistida",
    icon: FileText,
    items: [
      { title: "Importar XMLs Entrada", url: "/apuracao/entrada", icon: Upload, disabled: true },
      { title: "Importar XMLs Saída", url: "/apuracao/saida", icon: Upload, disabled: true },
      { title: "Importar XMLs NFS-e", url: "/apuracao/nfse", icon: FileText, disabled: true },
    ]
  },
  {
    title: "Conectar Receita Federal",
    icon: Globe,
    items: [
      { title: "Importar Apuração RFB", url: "/rfb/importar", icon: Download, disabled: true },
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
        <div className="flex items-center gap-2 px-4 py-2">
          <div className="flex aspect-square size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground">
            <Database className="size-4" />
          </div>
          <div className="grid flex-1 text-left text-sm leading-tight">
            <span className="truncate font-semibold">FB_APU01</span>
            <span className="truncate text-xs">Tax Reform System</span>
          </div>
        </div>
        <div className="px-4 pb-2">
           <CompanySwitcher />
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
                      <SidebarMenuButton tooltip={item.title}>
                        {item.icon && <item.icon />}
                        <span>{item.title}</span>
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
                              className={`${subItem.disabled ? "opacity-50 pointer-events-none" : ""} ${(subItem as any).className || ""}`}
                            >
                              <Link to={subItem.url}>
                                {subItem.icon && <subItem.icon className="mr-2 h-4 w-4 shrink-0" />}
                                <span>{subItem.title}</span>
                                {subItem.disabled && <span className="ml-auto text-xs text-muted-foreground">(Dev)</span>}
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
        <div className="p-2 border-t mt-auto">
          {user && (
            <div className="flex flex-col gap-2 p-2 bg-sidebar-accent/50 rounded-md">
              <div className="text-xs italic truncate">
                {company || "Empresa não identificada"}
              </div>
              <div className="flex flex-col gap-1">
                <div className="text-xs truncate">{user.full_name}</div>
                <div className="bg-yellow-100 text-yellow-700 px-1.5 py-0.5 rounded text-[10px] font-medium border border-yellow-200 self-start">
                   Vencimento: {new Date(user.trial_ends_at).toLocaleDateString()}
                </div>
              </div>
              <Button variant="ghost" size="sm" className="w-full justify-start h-7 px-0 text-muted-foreground hover:text-foreground" onClick={logout}>
                <LogOut className="mr-2 h-3 w-3" />
                <span className="text-xs">Sair</span>
              </Button>
            </div>
          )}
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}