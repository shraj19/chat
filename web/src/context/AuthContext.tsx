import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from "react";
import { useApiMutation } from "wire-axon/hooks";
import { useScratchQuery } from "wire-axon/hooks";
import {
  type User,
  type LoginResponse,
  type SignupResponse,
  type MeResponse,
  meResponseSchema,
  loginResponseSchema,
  signupResponseSchema,
} from "@/lib/schemas";
import { queryClient } from "@/lib/api";

interface AuthContextType {
  user: User | null;
  loading: boolean;
  isAuthenticated: boolean;
  login: (username: string, password: string) => Promise<void>;
  signup: (username: string, email: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  refetchUser: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const { get } = useScratchQuery({ baseURL: "/api" });

  // Fetch current user on mount
  const fetchUser = useCallback(async () => {
    try {
      const response = await get<MeResponse>({
        url: "/me",
        apiConfig: { timeout: 5000 },
      });
      const parsed = meResponseSchema.parse(response);
      setUser(parsed);
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, [get]);

  useEffect(() => {
    fetchUser();
  }, [fetchUser]);

  // Login mutation
  const { mutate: loginMutate } = useApiMutation<LoginResponse>({
    url: "/login",
    method: "post",
    baseURL: "/api",
    mutationOptions: {
      onSuccess: (response) => {
        const data = loginResponseSchema.parse(response.data);
        setUser({
          user_id: data.user_id,
          username: data.username,
          email: data.email,
          created_at: data.created_at,
        });
      },
    },
  });

  // Signup mutation
  const { mutate: signupMutate } = useApiMutation<SignupResponse>({
    url: "/signup",
    method: "post",
    baseURL: "/api",
    mutationOptions: {
      onSuccess: (response) => {
        const data = signupResponseSchema.parse(response.data);
        setUser({
          user_id: data.user_id,
          username: data.username,
          email: data.email,
        });
      },
    },
  });

  // Logout mutation
  const { mutate: logoutMutate } = useApiMutation({
    url: "/logout",
    method: "post",
    baseURL: "/api",
    mutationOptions: {
      onSuccess: () => {
        setUser(null);
        queryClient.clear();
      },
    },
  });

  const login = useCallback(
    (username: string, password: string): Promise<void> => {
      return new Promise((resolve, reject) => {
        loginMutate(
          { username, password },
          {
            onSuccess: () => resolve(),
            onError: (error) => reject(error),
          }
        );
      });
    },
    [loginMutate]
  );

  const signup = useCallback(
    (username: string, email: string, password: string): Promise<void> => {
      return new Promise((resolve, reject) => {
        signupMutate(
          { username, email, password },
          {
            onSuccess: () => resolve(),
            onError: (error) => reject(error),
          }
        );
      });
    },
    [signupMutate]
  );

  const logout = useCallback((): Promise<void> => {
    return new Promise((resolve, reject) => {
      logoutMutate(
        {},
        {
          onSuccess: () => resolve(),
          onError: (error) => reject(error),
        }
      );
    });
  }, [logoutMutate]);

  const value: AuthContextType = {
    user,
    loading,
    isAuthenticated: !!user,
    login,
    signup,
    logout,
    refetchUser: fetchUser,
  };

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
