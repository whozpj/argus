"use client";

import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { useRouter, usePathname, useSearchParams } from "next/navigation";
import { LogOut, Settings as SettingsIcon, Zap, LayoutDashboard, Activity, Bell, BookOpen } from "lucide-react";

import { fetchMe, logout } from "@/lib/api";
import type { MeResponse, Project } from "@/lib/types";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from "@/components/ui/select";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

// ─── Context ───────────────────────────────────────────────────────────────────

interface ShellContextValue {
  me: MeResponse;
  selectedProject: Project | null;
}

const ShellContext = createContext<ShellContextValue | null>(null);

export function useShell(): ShellContextValue {
  const ctx = useContext(ShellContext);
  if (!ctx) throw new Error("useShell must be used inside <Shell>");
  return ctx;
}

// ─── Nav items ────────────────────────────────────────────────────────────────

interface NavItem {
  label: string;
  href: string;
  icon: ReactNode;
  // True when the current route should highlight this item.
  matches: (pathname: string, search: string) => boolean;
}

const NAV_ITEMS: NavItem[] = [
  {
    label: "Overview",
    href: "/dashboard",
    icon: <LayoutDashboard className="h-4 w-4" />,
    matches: (p, s) => p === "/dashboard" && !s.includes("tab="),
  },
  {
    label: "Models",
    href: "/dashboard?tab=models",
    icon: <Activity className="h-4 w-4" />,
    matches: (p, s) => p === "/dashboard" && s.includes("tab=models"),
  },
  {
    label: "Alerts",
    href: "/dashboard?tab=alerts",
    icon: <Bell className="h-4 w-4" />,
    matches: (p, s) => p === "/dashboard" && s.includes("tab=alerts"),
  },
  {
    label: "Settings",
    href: "/settings",
    icon: <SettingsIcon className="h-4 w-4" />,
    matches: (p) => p === "/settings",
  },
  {
    label: "Docs",
    href: "/docs/quickstart",
    icon: <BookOpen className="h-4 w-4" />,
    matches: (p) => p.startsWith("/docs"),
  },
];

// ─── Shell ─────────────────────────────────────────────────────────────────────

export default function Shell({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const params = useSearchParams();
  const search = params.toString();

  const [me, setMe] = useState<MeResponse | null>(null);
  const [authError, setAuthError] = useState(false);

  // Preserve the project param across navigation — e.g. when switching tabs.
  const currentProjectID = params.get("project") ?? "";

  useEffect(() => {
    fetchMe()
      .then((m) => {
        setMe(m);
        // Auto-select first project on dashboard if none in URL.
        if (!params.get("project") && m.projects.length > 0 && pathname === "/dashboard") {
          const url = new URL(window.location.href);
          url.searchParams.set("project", m.projects[0].id);
          router.replace(url.pathname + url.search);
        }
      })
      .catch(() => {
        setAuthError(true);
        router.replace("/login");
      });
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  if (authError) return null;
  if (!me) return <ShellSkeleton />;

  const selectedProject = me.projects.find((p) => p.id === currentProjectID) ?? null;

  const handleProjectChange = (id: string | null) => {
    if (!id) return;
    const url = new URL(window.location.href);
    url.searchParams.set("project", id);
    router.push(url.pathname + url.search);
  };

  const handleLogout = () => {
    logout();
    router.replace("/login");
  };

  return (
    <ShellContext.Provider value={{ me, selectedProject }}>
      <div className="min-h-screen bg-[#f1f3f4]">
        {/* Topbar */}
        <header
          className="fixed top-0 left-0 right-0 z-20 h-14 bg-[#202124] text-white flex items-center px-4 gap-4"
          data-testid="shell-topbar"
        >
          <div className="flex items-center gap-2 shrink-0">
            <div className="flex items-center justify-center w-7 h-7 rounded-md bg-[#1a73e8]">
              <Zap className="h-4 w-4" />
            </div>
            <span className="font-semibold text-[15px] tracking-tight">Argus</span>
          </div>

          {me.projects.length > 0 && (
            <Select value={currentProjectID} onValueChange={handleProjectChange}>
              <SelectTrigger
                data-testid="project-selector"
                className="h-8 text-sm w-48 bg-[#3c4043] border-[#5f6368] text-white"
              >
                <span className="flex-1 text-left truncate">
                  {selectedProject?.name ?? "Select project"}
                </span>
              </SelectTrigger>
              <SelectContent>
                {me.projects.map((p) => (
                  <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}

          <div className="ml-auto flex items-center gap-3">
            <DropdownMenu>
              <DropdownMenuTrigger
                data-testid="user-email"
                className="text-sm text-white/90 hover:text-white max-w-[200px] truncate"
                title={me.email}
              >
                {me.display_name ?? me.email}
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="w-44">
                <DropdownMenuItem onClick={() => router.push("/settings")} className="flex items-center gap-2 cursor-pointer">
                  <SettingsIcon className="h-3.5 w-3.5" />
                  Settings
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  data-testid="sign-out"
                  onClick={handleLogout}
                  className="flex items-center gap-2 cursor-pointer text-muted-foreground"
                >
                  <LogOut className="h-3.5 w-3.5" />
                  Sign out
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </header>

        {/* Sidebar */}
        <aside
          className="fixed top-14 left-0 bottom-0 w-40 bg-white border-r border-[#e0e0e0] overflow-y-auto"
          data-testid="shell-sidebar"
        >
          <nav className="py-2">
            {NAV_ITEMS.map((item) => {
              const active = item.matches(pathname, search);
              return (
                <SidebarLink
                  key={item.href}
                  href={preserveProject(item.href, currentProjectID)}
                  active={active}
                  icon={item.icon}
                  label={item.label}
                  onClick={router.push}
                />
              );
            })}
          </nav>
        </aside>

        {/* Content */}
        <main className="pl-40 pt-14 min-h-screen">
          <div className="p-6">{children}</div>
        </main>
      </div>
    </ShellContext.Provider>
  );
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function SidebarLink({
  href,
  active,
  icon,
  label,
  onClick,
}: {
  href: string;
  active: boolean;
  icon: ReactNode;
  label: string;
  onClick: (href: string) => void;
}) {
  return (
    <button
      onClick={() => onClick(href)}
      className={
        "w-full flex items-center gap-3 px-4 h-9 text-sm text-left transition-colors " +
        (active
          ? "bg-[#e8f0fe] text-[#1a73e8] font-medium"
          : "text-[#202124] hover:bg-[#f1f3f4]")
      }
      data-testid={`nav-${label.toLowerCase()}`}
    >
      <span className={active ? "text-[#1a73e8]" : "text-[#5f6368]"}>{icon}</span>
      {label}
    </button>
  );
}

function ShellSkeleton() {
  return (
    <div className="min-h-screen bg-[#f1f3f4] flex items-center justify-center">
      <p className="text-sm text-[#5f6368]">Loading…</p>
    </div>
  );
}

// Preserve ?project= across navigations within the shell.
function preserveProject(href: string, projectID: string): string {
  if (!projectID) return href;
  if (!href.startsWith("/dashboard") && !href.startsWith("/settings") && !href.startsWith("/docs")) {
    return href;
  }
  const [base, query] = href.split("?");
  const qp = new URLSearchParams(query ?? "");
  qp.set("project", projectID);
  return `${base}?${qp.toString()}`;
}
