"use client";

import { createContext, useContext, useEffect, useState, ReactNode } from "react";
import { useRouter, usePathname, useSearchParams } from "next/navigation";
import { LogOut, Settings as SettingsIcon, Zap, LayoutDashboard, Activity, Bell, BookOpen, Plus } from "lucide-react";

import { fetchMe, logout, createProject } from "@/lib/api";
import { DOCS_PAGES } from "@/lib/docs";
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
  const [showNewProject, setShowNewProject] = useState(false);
  const [newProjectName, setNewProjectName] = useState("");
  const [creating, setCreating] = useState(false);
  const [createError, setCreateError] = useState<string | null>(null);

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

  // Default to first project on dashboard when URL has no ?project= yet
  // (avoids a two-render race where DashboardInner mounts before router.replace fires)
  const selectedProject =
    me.projects.find((p) => p.id === currentProjectID) ??
    (pathname === "/dashboard" && me.projects.length > 0 ? me.projects[0] : null);

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

  const handleCreateProject = async () => {
    const name = newProjectName.trim();
    if (!name) return;
    setCreating(true);
    setCreateError(null);
    try {
      const proj = await createProject(name);
      const updated = await fetchMe();
      setMe(updated);
      setShowNewProject(false);
      setNewProjectName("");
      const url = new URL(window.location.href);
      url.searchParams.set("project", proj.id);
      router.push("/dashboard?" + url.searchParams.toString());
    } catch (e: unknown) {
      setCreateError(e instanceof Error ? e.message : "Failed to create project");
    } finally {
      setCreating(false);
    }
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

          <button
            data-testid="new-project-btn"
            onClick={() => { setShowNewProject(true); setCreateError(null); setNewProjectName(""); }}
            className="flex items-center gap-1.5 h-8 px-3 text-sm border border-[#5f6368] text-[#8ab4f8] rounded hover:bg-[#3c4043] transition-colors"
          >
            <Plus className="h-3.5 w-3.5" />
            New project
          </button>

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
              const isDocs = item.label === "Docs";
              return (
                <div key={item.href}>
                  <SidebarLink
                    href={preserveProject(item.href, currentProjectID)}
                    active={active}
                    icon={item.icon}
                    label={item.label}
                    onClick={router.push}
                  />
                  {isDocs && pathname.startsWith("/docs") && (
                    <div className="pb-2">
                      {DOCS_PAGES.map((p) => {
                        const slugActive = pathname === `/docs/${p.slug}`;
                        return (
                          <button
                            key={p.slug}
                            onClick={() => router.push(`/docs/${p.slug}`)}
                            data-testid={`docs-subnav-${p.slug}`}
                            className={
                              "w-full flex items-center pl-10 pr-4 h-7 text-[12px] text-left transition-colors " +
                              (slugActive
                                ? "text-[#1a73e8] font-medium bg-[#e8f0fe]"
                                : "text-[#5f6368] hover:bg-[#f1f3f4]")
                            }
                          >
                            {p.title}
                          </button>
                        );
                      })}
                    </div>
                  )}
                </div>
              );
            })}
          </nav>
        </aside>

        {/* Content */}
        <main className="pl-40 pt-14 min-h-screen">
          <div className="p-6">{children}</div>
        </main>
      </div>

      {/* New project modal */}
      {showNewProject && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
          onClick={(e) => { if (e.target === e.currentTarget) setShowNewProject(false); }}
        >
          <div className="bg-white rounded-lg shadow-xl w-full max-w-sm p-6">
            <h2 className="text-base font-semibold text-[#202124] mb-4">New project</h2>
            <input
              autoFocus
              type="text"
              placeholder="Project name"
              value={newProjectName}
              onChange={(e) => setNewProjectName(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") handleCreateProject(); if (e.key === "Escape") setShowNewProject(false); }}
              className="w-full border border-[#dadce0] rounded px-3 py-2 text-sm text-[#202124] focus:outline-none focus:border-[#1a73e8] mb-3"
            />
            {createError && (
              <p className="text-xs text-red-600 mb-3">{createError}</p>
            )}
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setShowNewProject(false)}
                className="px-4 py-2 text-sm text-[#5f6368] hover:bg-[#f1f3f4] rounded transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleCreateProject}
                disabled={!newProjectName.trim() || creating}
                className="px-4 py-2 text-sm bg-[#1a73e8] text-white rounded hover:bg-[#1557b0] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
              >
                {creating ? "Creating…" : "Create"}
              </button>
            </div>
          </div>
        </div>
      )}
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
