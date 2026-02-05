import React, { useEffect, useState } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import { Skeleton } from "@/components/ui/skeleton";
import { Trash2, Plus } from "lucide-react";

interface FornSimples {
  cnpj: string;
}

export default function TabelaFornSimples() {
  const [data, setData] = useState<FornSimples[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [newCnpj, setNewCnpj] = useState("");

  const fetchData = () => {
    setLoading(true);
    fetch("/api/config/forn-simples")
      .then((res) => res.json())
      .then((data) => {
        setData(data || []);
        setLoading(false);
      })
      .catch((err) => {
        console.error("Failed to fetch FornSimples", err);
        toast.error("Erro ao carregar tabela Fornecedores Simples");
        setLoading(false);
      });
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!e.target.files || e.target.files.length === 0) return;
    
    const file = e.target.files[0];
    const formData = new FormData();
    formData.append("file", file);

    setUploading(true);
    try {
      const res = await fetch("/api/config/forn-simples/import", {
        method: "POST",
        body: formData,
      });
      
      if (!res.ok) {
        let errorMsg = "Falha na importação";
        try {
          const text = await res.text();
          try {
            const errData = JSON.parse(text);
            errorMsg = errData.error || errData.message || errorMsg;
          } catch {
            if (text) errorMsg = text;
          }
        } catch (e) {
            console.error(e);
        }
        throw new Error(errorMsg);
      }
      
      toast.success("Importação concluída com sucesso!");
      fetchData();
    } catch (error: any) {
      console.error(error);
      toast.error(`Erro ao importar arquivo CSV: ${error.message}`);
    } finally {
        setUploading(false);
        e.target.value = "";
    }
  };

  const handleAdd = async () => {
      if (!newCnpj) return;
      try {
          const res = await fetch("/api/config/forn-simples", {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ cnpj: newCnpj }),
          });
          if (!res.ok) {
              const text = await res.text();
              throw new Error(text);
          }
          toast.success("CNPJ adicionado com sucesso!");
          setNewCnpj("");
          fetchData();
      } catch (err: any) {
          toast.error("Erro ao adicionar CNPJ: " + err.message);
      }
  };

  const handleDelete = async (cnpj: string) => {
      if (!confirm(`Deseja remover o CNPJ ${cnpj}?`)) return;
      try {
          const res = await fetch(`/api/config/forn-simples?cnpj=${cnpj}`, {
              method: "DELETE",
          });
          if (!res.ok) {
              const text = await res.text();
              throw new Error(text);
          }
          toast.success("CNPJ removido com sucesso!");
          fetchData();
      } catch (err: any) {
          toast.error("Erro ao remover CNPJ: " + err.message);
      }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Fornecedores Simples Nacional</CardTitle>
          <CardDescription>
            Gerencie os CNPJs de fornecedores do Simples Nacional. Importe via CSV (coluna única: CNPJ, delimitador ';').
          </CardDescription>
        </CardHeader>
        <CardContent>
            <div className="flex flex-col gap-6 mb-6">
                <div className="flex items-end gap-4">
                    <div className="grid w-full max-w-sm items-center gap-1.5">
                        <Label htmlFor="cnpj_input">Adicionar CNPJ Manualmente</Label>
                        <Input 
                            id="cnpj_input" 
                            placeholder="00.000.000/0000-00" 
                            value={newCnpj} 
                            onChange={(e) => setNewCnpj(e.target.value)} 
                        />
                    </div>
                    <Button onClick={handleAdd} disabled={!newCnpj}>
                        <Plus className="mr-2 h-4 w-4" /> Adicionar
                    </Button>
                </div>

                <div className="grid w-full max-w-sm items-center gap-1.5">
                    <Label htmlFor="csv_upload">Importar CSV (Lista de CNPJs)</Label>
                    <Input id="csv_upload" type="file" accept=".csv" onChange={handleFileUpload} disabled={uploading} />
                </div>
            </div>

            {loading ? (
                 <div className="space-y-2">
                    <Skeleton className="h-8 w-full" />
                    <Skeleton className="h-8 w-full" />
                    <Skeleton className="h-8 w-full" />
                 </div>
            ) : (
                <div className="rounded-md border">
                <Table>
                    <TableHeader>
                    <TableRow>
                        <TableHead>CNPJ</TableHead>
                        <TableHead className="w-[100px] text-right">Ações</TableHead>
                    </TableRow>
                    </TableHeader>
                    <TableBody>
                    {data.length === 0 ? (
                        <TableRow>
                            <TableCell colSpan={2} className="text-center">Nenhum registro encontrado.</TableCell>
                        </TableRow>
                    ) : (
                        data.map((item) => (
                            <TableRow key={item.cnpj}>
                            <TableCell className="font-medium">{item.cnpj}</TableCell>
                            <TableCell className="text-right">
                                <Button variant="ghost" size="icon" onClick={() => handleDelete(item.cnpj)}>
                                    <Trash2 className="h-4 w-4 text-red-500" />
                                </Button>
                            </TableCell>
                            </TableRow>
                        ))
                    )}
                    </TableBody>
                </Table>
                </div>
            )}
        </CardContent>
      </Card>
    </div>
  );
}
