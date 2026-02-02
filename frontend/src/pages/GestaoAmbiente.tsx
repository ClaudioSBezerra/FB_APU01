import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Plus, Trash2, Building, Layers } from "lucide-react";
import { toast } from "sonner";

interface Environment {
  id: string;
  name: string;
  description: string;
  created_at: string;
}

interface EnterpriseGroup {
  id: string;
  environment_id: string;
  name: string;
  description: string;
  created_at: string;
}

export default function GestaoAmbiente() {
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [selectedEnv, setSelectedEnv] = useState<Environment | null>(null);
  const [groups, setGroups] = useState<EnterpriseGroup[]>([]);
  
  // Modal states
  const [isEnvModalOpen, setIsEnvModalOpen] = useState(false);
  const [isGroupModalOpen, setIsGroupModalOpen] = useState(false);
  
  // Form states
  const [newEnvName, setNewEnvName] = useState("");
  const [newEnvDesc, setNewEnvDesc] = useState("");
  const [newGroupName, setNewGroupName] = useState("");
  const [newGroupDesc, setNewGroupDesc] = useState("");

  const [loading, setLoading] = useState(false);

  // Initial Load
  useEffect(() => {
    fetchEnvironments();
  }, []);

  // Load Groups when Env selected
  useEffect(() => {
    if (selectedEnv) {
      fetchGroups(selectedEnv.id);
    } else {
      setGroups([]);
    }
  }, [selectedEnv]);

  const fetchEnvironments = async () => {
    try {
      setLoading(true);
      const res = await fetch("/api/config/environments");
      if (!res.ok) throw new Error("Failed to fetch environments");
      const data = await res.json();
      setEnvironments(data);
      // Select first one by default if none selected and data exists
      if (!selectedEnv && data.length > 0) {
        setSelectedEnv(data[0]);
      }
    } catch (error) {
      console.error(error);
      toast.error("Erro ao carregar ambientes");
    } finally {
      setLoading(false);
    }
  };

  const fetchGroups = async (envId: string) => {
    try {
      const res = await fetch(`/api/config/groups?environment_id=${envId}`);
      if (!res.ok) throw new Error("Failed to fetch groups");
      const data = await res.json();
      setGroups(data);
    } catch (error) {
      console.error(error);
      toast.error("Erro ao carregar grupos de empresas");
    }
  };

  const handleCreateEnvironment = async () => {
    if (!newEnvName) {
      toast.error("Nome do ambiente é obrigatório");
      return;
    }

    try {
      const res = await fetch("/api/config/environments", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: newEnvName, description: newEnvDesc }),
      });

      if (!res.ok) throw new Error("Failed to create");
      
      toast.success("Ambiente criado com sucesso!");
      setIsEnvModalOpen(false);
      setNewEnvName("");
      setNewEnvDesc("");
      fetchEnvironments();
    } catch (error) {
      toast.error("Erro ao criar ambiente");
    }
  };

  const handleCreateGroup = async () => {
    if (!selectedEnv) return;
    if (!newGroupName) {
      toast.error("Nome do grupo é obrigatório");
      return;
    }

    try {
      const res = await fetch("/api/config/groups", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          environment_id: selectedEnv.id,
          name: newGroupName,
          description: newGroupDesc
        }),
      });

      if (!res.ok) throw new Error("Failed to create");
      
      toast.success("Grupo criado com sucesso!");
      setIsGroupModalOpen(false);
      setNewGroupName("");
      setNewGroupDesc("");
      fetchGroups(selectedEnv.id);
    } catch (error) {
      toast.error("Erro ao criar grupo");
    }
  };

  const handleDeleteEnvironment = async (id: string) => {
    if (!confirm("Tem certeza? Isso apagará TODOS os grupos vinculados.")) return;
    
    try {
      const res = await fetch(`/api/config/environments?id=${id}`, { method: "DELETE" });
      if (!res.ok) throw new Error("Failed to delete");
      toast.success("Ambiente removido");
      if (selectedEnv?.id === id) setSelectedEnv(null);
      fetchEnvironments();
    } catch (error) {
      toast.error("Erro ao remover ambiente");
    }
  };

  const handleDeleteGroup = async (id: string) => {
    if (!confirm("Tem certeza?")) return;
    
    try {
      const res = await fetch(`/api/config/groups?id=${id}`, { method: "DELETE" });
      if (!res.ok) throw new Error("Failed to delete");
      toast.success("Grupo removido");
      if (selectedEnv) fetchGroups(selectedEnv.id);
    } catch (error) {
      toast.error("Erro ao remover grupo");
    }
  };

  return (
    <div className="container mx-auto p-6 space-y-8">
      <div>
        <h1 className="text-3xl font-bold text-gray-900">Gestão de Ambientes</h1>
        <p className="text-gray-500 mt-1">
          Gerencie ambientes e grupos de empresas para multi-tenancy.
        </p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* Left Column: Environments List */}
        <div className="md:col-span-1 space-y-4">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-lg font-medium">Ambientes</CardTitle>
              <Dialog open={isEnvModalOpen} onOpenChange={setIsEnvModalOpen}>
                <DialogTrigger asChild>
                  <Button size="sm" variant="outline"><Plus className="w-4 h-4" /></Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Novo Ambiente</DialogTitle>
                    <DialogDescription>Crie um novo ambiente isolado (Ex: Produção, Homologação).</DialogDescription>
                  </DialogHeader>
                  <div className="space-y-4 py-4">
                    <div className="space-y-2">
                      <Label>Nome do Ambiente</Label>
                      <Input value={newEnvName} onChange={(e) => setNewEnvName(e.target.value)} placeholder="Ex: Ambiente Produção" />
                    </div>
                    <div className="space-y-2">
                      <Label>Descrição</Label>
                      <Input value={newEnvDesc} onChange={(e) => setNewEnvDesc(e.target.value)} placeholder="Descrição opcional" />
                    </div>
                  </div>
                  <DialogFooter>
                    <Button onClick={handleCreateEnvironment}>Criar Ambiente</Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                {loading && <p className="text-sm text-muted-foreground">Carregando...</p>}
                {!loading && environments.length === 0 && (
                  <p className="text-sm text-muted-foreground">Nenhum ambiente criado.</p>
                )}
                {environments.map((env) => (
                  <div
                    key={env.id}
                    className={`flex items-center justify-between p-3 rounded-lg border cursor-pointer transition-colors ${
                      selectedEnv?.id === env.id
                        ? "bg-primary/10 border-primary"
                        : "hover:bg-gray-50 border-gray-200"
                    }`}
                    onClick={() => setSelectedEnv(env)}
                  >
                    <div className="flex items-center gap-3">
                      <Layers className="w-4 h-4 text-gray-500" />
                      <div>
                        <p className="font-medium text-sm">{env.name}</p>
                        {env.description && <p className="text-xs text-gray-500">{env.description}</p>}
                      </div>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="h-8 w-8 text-red-500 hover:text-red-700 hover:bg-red-50"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleDeleteEnvironment(env.id);
                      }}
                    >
                      <Trash2 className="w-4 h-4" />
                    </Button>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Right Column: Groups List (Detail) */}
        <div className="md:col-span-2 space-y-4">
          <Card className="h-full">
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <div>
                <CardTitle className="text-lg font-medium">Grupos de Empresas</CardTitle>
                <CardDescription>
                  {selectedEnv 
                    ? `Vinculados ao ambiente: ${selectedEnv.name}` 
                    : "Selecione um ambiente para ver os grupos"}
                </CardDescription>
              </div>
              <Dialog open={isGroupModalOpen} onOpenChange={setIsGroupModalOpen}>
                <DialogTrigger asChild>
                  <Button size="sm" disabled={!selectedEnv}><Plus className="w-4 h-4 mr-2" /> Novo Grupo</Button>
                </DialogTrigger>
                <DialogContent>
                  <DialogHeader>
                    <DialogTitle>Novo Grupo de Empresas</DialogTitle>
                    <DialogDescription>Crie um grupo para consolidar empresas.</DialogDescription>
                  </DialogHeader>
                  <div className="space-y-4 py-4">
                    <div className="space-y-2">
                      <Label>Nome do Grupo</Label>
                      <Input value={newGroupName} onChange={(e) => setNewGroupName(e.target.value)} placeholder="Ex: Grupo Varejo X" />
                    </div>
                    <div className="space-y-2">
                      <Label>Descrição</Label>
                      <Input value={newGroupDesc} onChange={(e) => setNewGroupDesc(e.target.value)} placeholder="Descrição opcional" />
                    </div>
                  </div>
                  <DialogFooter>
                    <Button onClick={handleCreateGroup}>Criar Grupo</Button>
                  </DialogFooter>
                </DialogContent>
              </Dialog>
            </CardHeader>
            <CardContent>
              {!selectedEnv ? (
                <div className="flex flex-col items-center justify-center py-10 text-gray-400">
                  <Layers className="w-12 h-12 mb-2 opacity-20" />
                  <p>Selecione um ambiente à esquerda</p>
                </div>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Nome do Grupo</TableHead>
                      <TableHead>Descrição</TableHead>
                      <TableHead>Criado em</TableHead>
                      <TableHead className="text-right">Ações</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {groups.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-8 text-gray-500">
                          Nenhum grupo cadastrado neste ambiente.
                        </TableCell>
                      </TableRow>
                    ) : (
                      groups.map((group) => (
                        <TableRow key={group.id}>
                          <TableCell className="font-medium flex items-center gap-2">
                            <Building className="w-4 h-4 text-gray-500" />
                            {group.name}
                          </TableCell>
                          <TableCell>{group.description}</TableCell>
                          <TableCell>{new Date(group.created_at).toLocaleDateString()}</TableCell>
                          <TableCell className="text-right">
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-8 w-8 text-red-500 hover:text-red-700 hover:bg-red-50"
                              onClick={() => handleDeleteGroup(group.id)}
                            >
                              <Trash2 className="w-4 h-4" />
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
