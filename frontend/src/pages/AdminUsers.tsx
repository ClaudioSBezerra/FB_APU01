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
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [newRole, setNewRole] = useState<string>("user");
  const [extendDays, setExtendDays] = useState<number>(0);

  const { data: users, isLoading } = useQuery<User[]>({
    queryKey: ['admin-users'],
    queryFn: async () => {
      const response = await fetch(`${import.meta.env.VITE_API_TARGET}/api/admin/users`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!response.ok) throw new Error('Failed to fetch users');
      return response.json();
    }
  });

  const promoteMutation = useMutation({
    mutationFn: async (data: { userId: string, role: string, extendDays: number }) => {
      const response = await fetch(`${import.meta.env.VITE_API_TARGET}/api/admin/users/promote?id=${data.userId}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: `Bearer ${token}`
        },
        body: JSON.stringify({ role: data.role, extend_days: data.extendDays })
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
      const response = await fetch(`${import.meta.env.VITE_API_TARGET}/api/admin/users/delete?id=${userId}`, {
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

  const handleOpenPromote = (user: User) => {
    setSelectedUser(user);
    setNewRole(user.role);
    setExtendDays(0);
    setPromoteDialogOpen(true);
  };

  const handlePromote = () => {
    if (selectedUser) {
      promoteMutation.mutate({
        userId: selectedUser.id,
        role: newRole,
        extendDays: extendDays
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
              <Label htmlFor="extend" className="text-right">
                Estender Trial
              </Label>
              <div className="col-span-3 flex items-center gap-2">
                <Input
                  id="extend"
                  type="number"
                  value={extendDays}
                  onChange={(e) => setExtendDays(parseInt(e.target.value) || 0)}
                  className="w-24"
                />
                <span className="text-sm text-muted-foreground">dias</span>
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