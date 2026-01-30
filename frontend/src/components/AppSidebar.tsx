import {
  Settings,
  Table,
  Users,
  Building,
  Calculator,
  FileSpreadsheet,
  ShoppingCart,
  Zap,
  Truck,
  Phone,
  LayoutDashboard,
  FileText,
  Upload,
  Globe,
  Download,
  ChevronRight,
  Database
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

// Menu definition
const menuItems = [
  {
    title: "Configurações e Tabelas",
    icon: Settings,
    items: [
      { title: "Tabela de Alíquotas", url: "/config/aliquotas", icon: Table },
      { title: "Tabela CFOP", url: "/config/cfop", icon: Table },
      { title: "Gestão de Usuários", url: "/config/usuarios", icon: Users, disabled: true },
      { title: "Gestão de Ambiente", url: "/config/ambiente", icon: Building, disabled: true },
    ]
  },
  {
    title: "Simulador da RT",
    icon: Calculator,
    isActive: true, // Default open
    items: [
      { title: "Importar SPEDs", url: "/importar-efd", icon: FileSpreadsheet },
      { title: "Operações Comerciais", url: "/mercadorias", icon: ShoppingCart },
      { title: "Energia", url: "/energia", icon: Zap },
      { title: "Transporte", url: "/transporte", icon: Truck },
      { title: "Comunicação", url: "/comunicacoes", icon: Phone },
      { title: "Dashboards", url: "/dashboards", icon: LayoutDashboard, disabled: true },
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
                        {item.items.map((subItem) => (
                          <SidebarMenuSubItem key={subItem.title}>
                            <SidebarMenuSubButton 
                              asChild 
                              isActive={location.pathname === subItem.url}
                              className={subItem.disabled ? "opacity-50 pointer-events-none" : ""}
                            >
                              <Link to={subItem.url}>
                                {subItem.icon && <subItem.icon className="mr-2 h-4 w-4" />}
                                <span>{subItem.title}</span>
                                {subItem.disabled && <span className="ml-auto text-xs text-muted-foreground">(Dev)</span>}
                              </Link>
                            </SidebarMenuSubButton>
                          </SidebarMenuSubItem>
                        ))}
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
        <div className="px-4 py-2 text-xs text-muted-foreground text-center">
          v1.0.0
        </div>
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}