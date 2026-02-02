import { useAuth } from "@/contexts/AuthContext";

export function Footer() {
  const { user, environment, group, company, isAuthenticated } = useAuth();

  if (!isAuthenticated || !user) {
    return null;
  }

  return (
    <footer className="border-t bg-muted/50 py-2 px-4 text-xs text-muted-foreground mt-auto">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex items-center gap-4">
          <span>
            <strong>Usuário:</strong> {user.full_name}
          </span>
          <span className="hidden sm:inline">|</span>
          <span>
            <strong>Ambiente:</strong> {environment || "N/A"}
          </span>
          <span className="hidden sm:inline">|</span>
          <span>
            <strong>Grupo:</strong> {group || "N/A"}
          </span>
          <span className="hidden sm:inline">|</span>
          <span>
            <strong>Empresa:</strong> {company || "N/A"}
          </span>
        </div>
        <div>
          <span>
            <strong>Trial expira em:</strong> {new Date(user.trial_ends_at).toLocaleDateString()}
          </span>
        </div>
      </div>
    </footer>
  );
}
