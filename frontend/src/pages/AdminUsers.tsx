import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { toast } from "sonner";
import { Check, X, Trash2, Shield, Calendar, UserCheck } from "lucide-react";
import { useAuth } from "@/contexts/AuthContext";

interface User {
  id: string;
  email: string;
  full_name: string;
  is_verified: boolean;
  trial_ends_at: string;
  role: string;
  created_at: string;
}

export default function AdminUsers() {
  const { token } = useAuth();
  const queryClient = useQueryClient();
  const [promoteDialogOpen, setPromoteDialogOpen] = useState(false);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  
  // State for Promote/Edit
  const [newRole, setNewRole] = useState<string>("user");
  const [extendDays, setExtendDays] = useState<number>(0);
  const [isOfficial, setIsOfficial] = useState<boolean>(false);

  // State for Create
  const [newUser, setNewUser] = useState({ fullName: "", email: "", password: "", role: "user" });

  const { data: users, isLoading } = useQuery<User[]>({
    queryKey: ['admin-users'],
    queryFn: async () => {
      const response = await fetch(`/api/admin/users`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!response.ok) {
        const text = await response.text();
        try {
          const json = JSON.parse(text);
          throw new Error(json.message || `Erro: ${response.status} ${response.statusText}`);
        } catch {
          // Se não for JSON (ex: HTML do Nginx 502/404), lança erro genérico com status
          throw new Error(`Erro de Servidor (${response.status}): A API não respondeu corretamente.`);
        }
      }
      return response.json();
    },
    enabled: !!token
  });

  const createMutation = useMutation({
    mutationFn: async (data: typeof newUser) => {
      const response = await fetch(`/api/admin/users/create`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify({ 
          full_name: data.fullName,
          email: data.email,
          password: data.password,
          role: data.role
        })
      });
      if (!response.ok) {
        const text = await response.text();
        try {
          const json = JSON.parse(text);
          throw new Error(json.message || 'Falha ao criar usuário');
        } catch {
          throw new Error(`Erro de Servidor (${response.status})`);
        }
      }
      return response.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      toast.success("Usuário criado com sucesso");
      setCreateDialogOpen(false);
      setNewUser({ fullName: "", email: "", password: "", role: "user" });
    },
    onError: (error: Error) => toast.error(error.message || "Erro ao criar usuário")
  });

  const promoteMutation = useMutation({
    mutationFn: async (data: { userId: string, role: string, extendDays: number, isOfficial: boolean }) => {
      const response = await fetch(`/api/admin/users/promote?id=${data.userId}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify({ role: data.role, extend_days: data.extendDays, is_official: data.isOfficial })
      });
      if (!response.ok) throw new Error('Failed to update user');
      return response.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      toast.success("Usuário atualizado com sucesso");
      setPromoteDialogOpen(false);
    },
    onError: () => toast.error("Erro ao atualizar usuário")
  });

  const deleteMutation = useMutation({
    mutationFn: async (userId: string) => {
      const response = await fetch(`/api/admin/users/delete?id=${userId}`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!response.ok) throw new Error('Failed to delete user');
      return response.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['admin-users'] });
      toast.success("Usuário removido com sucesso");
    },
    onError: () => toast.error("Erro ao remover usuário")
  });

  const handleCreate = () => {
    if (!newUser.fullName || !newUser.email || !newUser.password) {
      toast.error("Preencha todos os campos obrigatórios");
      return;
    }
    createMutation.mutate(newUser);
  };

  const handleOpenPromote = (user: User) => {
    setSelectedUser(user);
    setNewRole(user.role);
    setExtendDays(0);
    setIsOfficial(false);
    setPromoteDialogOpen(true);
  };

  const handlePromote = () => {
    if (selectedUser) {
      promoteMutation.mutate({
        userId: selectedUser.id,
        role: newRole,
        extendDays: extendDays,
        isOfficial: isOfficial
      });
    }
  };

  const handleDelete = (userId: string) => {
    if (confirm("Tem certeza que deseja excluir este usuário? Esta ação não pode ser desfeita.")) {
      deleteMutation.mutate(userId);
    }
  };

  if (isLoading) return <div>Carregando usuários...</div>;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h2 className="text-2xl font-bold tracking-tight">Gestão de Usuários</h2>
        <Button onClick={() => setCreateDialogOpen(true)}>
          <Check className="mr-2 h-4 w-4" /> Novo Usuário
        </Button>
      </div>

      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Nome</TableHead>
              <TableHead>Email</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Trial Vence Em</TableHead>
              <TableHead>Criado Em</TableHead>
              <TableHead className="text-right">Ações</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users?.map((user) => (
              <TableRow key={user.id}>
                <TableCell className="font-medium">{user.full_name}</TableCell>
                <TableCell>{user.email}</TableCell>
                <TableCell>
                  {user.is_verified ? (
                    <Badge variant="outline" className="bg-green-50 text-green-700 border-green-200">Verificado</Badge>
                  ) : (
                    <Badge variant="outline" className="bg-yellow-50 text-yellow-700 border-yellow-200">Pendente</Badge>
                  )}
                </TableCell>
                <TableCell>
                  <Badge variant={user.role === 'admin' ? "default" : "secondary"}>
                    {user.role}
                  </Badge>
                </TableCell>
                <TableCell>
                  {new Date(user.trial_ends_at).toLocaleDateString()}
                  {new Date(user.trial_ends_at) < new Date() && (
                    <span className="ml-2 text-xs text-red-500 font-medium">(Expirado)</span>
                  )}
                </TableCell>
                <TableCell>{new Date(user.created_at).toLocaleDateString()}</TableCell>
                <TableCell className="text-right space-x-2">
                  <Button variant="ghost" size="icon" onClick={() => handleOpenPromote(user)}>
                    <UserCheck className="h-4 w-4" />
                  </Button>
                  <Button variant="ghost" size="icon" className="text-red-500 hover:text-red-600" onClick={() => handleDelete(user.id)}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>

      <Dialog open={createDialogOpen} onOpenChange={setCreateDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Novo Usuário</DialogTitle>
            <DialogDescription>
              Criar um novo usuário manualmente.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="newName" className="text-right">Nome</Label>
              <Input
                id="newName"
                value={newUser.fullName}
                onChange={(e) => setNewUser({...newUser, fullName: e.target.value})}
                className="col-span-3"
              />
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="newEmail" className="text-right">Email</Label>
              <Input
                id="newEmail"
                type="email"
                value={newUser.email}
                onChange={(e) => setNewUser({...newUser, email: e.target.value})}
                className="col-span-3"
              />
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="newPassword" className="text-right">Senha</Label>
              <Input
                id="newPassword"
                type="password"
                value={newUser.password}
                onChange={(e) => setNewUser({...newUser, password: e.target.value})}
                className="col-span-3"
              />
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="newRole" className="text-right">Role</Label>
              <Select value={newUser.role} onValueChange={(val) => setNewUser({...newUser, role: val})}>
                <SelectTrigger className="col-span-3">
                  <SelectValue placeholder="Selecione..." />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="user">User</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateDialogOpen(false)}>Cancelar</Button>
            <Button onClick={handleCreate} disabled={createMutation.isPending}>
              {createMutation.isPending ? "Criando..." : "Criar Usuário"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={promoteDialogOpen} onOpenChange={setPromoteDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Editar Usuário</DialogTitle>
            <DialogDescription>
              Alterar permissões ou estender período de trial para {selectedUser?.full_name}.
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-4">
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="role" className="text-right">
                Role
              </Label>
              <Select value={newRole} onValueChange={setNewRole}>
                <SelectTrigger className="col-span-3">
                  <SelectValue placeholder="Selecione..." />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="user">User</SelectItem>
                  <SelectItem value="admin">Admin</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="extendDays" className="text-right">
                Estender (dias)
              </Label>
              <Input
                id="extendDays"
                type="number"
                value={extendDays}
                onChange={(e) => setExtendDays(Number(e.target.value))}
                className="col-span-3"
                disabled={isOfficial}
              />
            </div>
            <div className="grid grid-cols-4 items-center gap-4">
              <Label htmlFor="isOfficial" className="text-right">
                Cliente Oficial
              </Label>
              <div className="col-span-3 flex items-center space-x-2">
                <Checkbox 
                  id="isOfficial" 
                  checked={isOfficial}
                  onCheckedChange={(checked) => setIsOfficial(checked as boolean)}
                />
                <label
                  htmlFor="isOfficial"
                  className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70"
                >
                  Definir como cliente permanente (Até 2099)
                </label>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPromoteDialogOpen(false)}>Cancelar</Button>
            <Button onClick={handlePromote} disabled={promoteMutation.isPending}>
              {promoteMutation.isPending ? "Salvando..." : "Salvar Alterações"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}