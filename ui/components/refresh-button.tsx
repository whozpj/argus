"use client";

import { useRouter } from "next/navigation";
import { RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useState } from "react";

export function RefreshButton() {
  const router = useRouter();
  const [spinning, setSpinning] = useState(false);

  function handleRefresh() {
    setSpinning(true);
    router.refresh();
    setTimeout(() => setSpinning(false), 600);
  }

  return (
    <Button variant="ghost" size="sm" onClick={handleRefresh} aria-label="Refresh dashboard">
      <RefreshCw className={`h-4 w-4 ${spinning ? "animate-spin" : ""}`} />
      <span className="ml-2 text-sm">Refresh</span>
    </Button>
  );
}
