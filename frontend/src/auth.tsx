import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import { api, ApiError, clearSession, refresh } from "./api";
import type { User } from "./types";
const Context = createContext<{
  user: User | null;
  loading: boolean;
  error: string;
  reload: () => Promise<void>;
}>(null!);
export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const reload = async () => {
    setUser(await api<User>("/users/me"));
  };
  useEffect(() => {
    const session = (e: Event) => setUser((e as CustomEvent<User>).detail);
    const signedOut = () => setUser(null);
    window.addEventListener("relations:session", session);
    window.addEventListener("relations:logout", signedOut);
    const channel =
      typeof BroadcastChannel !== "undefined"
        ? new BroadcastChannel("relations-auth")
        : null;
    if (channel) channel.onmessage = clearSession;
    refresh()
      .catch((e) => {
        if (!(e instanceof ApiError && e.status === 401)) setError(e.message);
      })
      .finally(() => setLoading(false));
    return () => {
      window.removeEventListener("relations:session", session);
      window.removeEventListener("relations:logout", signedOut);
      channel?.close();
    };
  }, []);
  return (
    <Context.Provider value={{ user, loading, error, reload }}>
      {children}
    </Context.Provider>
  );
}
export const useAuth = () => useContext(Context);
