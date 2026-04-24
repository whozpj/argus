"use client";

import { useEffect, useRef, useState } from "react";
import { Zap, AlertTriangle, Hash, RotateCcw } from "lucide-react";

export default function DashboardDemo() {
  const rootRef = useRef<HTMLDivElement>(null);
  const [runKey, setRunKey] = useState(0);
  const [playing, setPlaying] = useState(false);

  // Trigger on scroll into view, once.
  useEffect(() => {
    const el = rootRef.current;
    if (!el) return;
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) {
            setPlaying(true);
            io.disconnect();
            break;
          }
        }
      },
      { threshold: 0.3 },
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  const replay = () => {
    setPlaying(false);
    // Next tick: flip back on to re-run keyframes.
    requestAnimationFrame(() => {
      setRunKey((k) => k + 1);
      setPlaying(true);
    });
  };

  return (
    <div>
      <div
        ref={rootRef}
        key={runKey}
        className={"demo-root rounded-lg border border-[#e0e0e0] bg-white shadow-xl overflow-hidden " + (playing ? "playing" : "")}
      >
        {/* Mock topbar */}
        <div className="h-10 bg-[#202124] text-white flex items-center px-4 gap-3">
          <div className="flex items-center gap-1.5">
            <div className="w-5 h-5 rounded-sm bg-[#1a73e8] flex items-center justify-center"><Zap className="h-3 w-3" /></div>
            <span className="text-sm font-semibold">Argus</span>
          </div>
          <div className="text-xs text-white/60 ml-2">production</div>
        </div>
        <div className="flex">
          {/* Mock sidebar */}
          <div className="w-28 bg-white border-r border-[#e0e0e0] py-2 shrink-0">
            {["Overview", "Models", "Alerts", "Settings", "Docs"].map((l, i) => (
              <div
                key={l}
                className={
                  "px-3 h-7 flex items-center text-xs " +
                  (i === 0 ? "bg-[#e8f0fe] text-[#1a73e8] font-medium" : "text-[#5f6368]")
                }
              >
                {l}
              </div>
            ))}
          </div>
          {/* Mock content */}
          <div className="flex-1 bg-[#f1f3f4] p-4 space-y-3">
            <div className="grid grid-cols-4 gap-2">
              <Stat label="Events" value="99,412" />
              <Stat label="Models" value="3" />
              <Stat label="Alerts" value="1" amber />
              <Stat label="Checked" value="42s" />
            </div>
            <div className="rounded-md border border-[#e0e0e0] bg-white p-3">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-medium text-[#202124]">claude-sonnet-4-6 · output_tokens</span>
                <span className="demo-badge text-[10px] font-medium text-[#92400e] bg-[#fef3c7] border border-[#f59e0b] rounded-full px-2 py-0.5">
                  output_tokens +42% above baseline
                </span>
              </div>
              <ChartSvg />
            </div>
          </div>
        </div>
        {/* Slack toast */}
        <div className="demo-toast absolute right-6 bottom-16 w-[320px] rounded-md border border-[#e0e0e0] bg-white shadow-xl p-3 text-xs">
          <div className="flex items-center gap-2 text-[#5f6368] mb-1">
            <Hash className="h-3.5 w-3.5" />
            <span className="font-semibold text-[#202124]">#alerts</span>
            <span>·</span>
            <span>Argus</span>
            <span>·</span>
            <span>just now</span>
          </div>
          <p className="text-[#202124] leading-5 flex gap-1.5">
            <AlertTriangle className="h-3.5 w-3.5 text-[#f59e0b] shrink-0 mt-0.5" />
            <span>Drift detected on <span className="font-mono">claude-sonnet-4-6</span>, <strong>output_tokens +42%</strong>, score: 0.84</span>
          </p>
        </div>
      </div>

      <div className="flex justify-center mt-4">
        <button
          onClick={replay}
          className="inline-flex items-center gap-1.5 text-xs text-[#5f6368] hover:text-[#202124]"
        >
          <RotateCcw className="h-3 w-3" /> Replay
        </button>
      </div>

      {/* Animation CSS — timing matches the spec timeline */}
      <style>{`
        .demo-root { position: relative; }

        /* Baseline band */
        .demo-root .demo-band {
          opacity: 0;
          transition: none;
        }
        .demo-root.playing .demo-band {
          animation: fade 0.4s ease-out 0.3s forwards;
        }

        /* Blue polyline draw */
        .demo-root .demo-line-blue {
          stroke-dasharray: 600;
          stroke-dashoffset: 600;
        }
        .demo-root.playing .demo-line-blue {
          animation: draw 1.4s ease-out 0.3s forwards;
        }

        /* Vertical drift marker */
        .demo-root .demo-marker {
          opacity: 0;
        }
        .demo-root.playing .demo-marker {
          animation: fade 0.4s ease-out 1.9s forwards;
        }

        /* Amber polyline */
        .demo-root .demo-line-amber {
          stroke-dasharray: 300;
          stroke-dashoffset: 300;
        }
        .demo-root.playing .demo-line-amber {
          animation: draw-amber 0.9s ease-out 1.9s forwards;
        }

        /* Drift label */
        .demo-root .demo-label {
          opacity: 0;
        }
        .demo-root.playing .demo-label {
          animation: fade 0.4s ease-out 2.1s forwards;
        }

        /* Drift badge (scale pop) */
        .demo-badge {
          opacity: 0;
          transform: scale(0.6);
        }
        .demo-root.playing .demo-badge {
          animation: pop 0.35s cubic-bezier(0.2, 1.4, 0.4, 1) 2.4s forwards;
        }

        /* Slack toast */
        .demo-toast {
          opacity: 0;
          transform: translateX(60px);
        }
        .demo-root.playing .demo-toast {
          animation: slide-in 0.45s ease-out 2.8s forwards;
        }

        @keyframes fade {
          to { opacity: 1; }
        }
        @keyframes draw {
          to { stroke-dashoffset: 0; }
        }
        @keyframes draw-amber {
          to { stroke-dashoffset: 0; }
        }
        @keyframes pop {
          to { opacity: 1; transform: scale(1); }
        }
        @keyframes slide-in {
          to { opacity: 1; transform: translateX(0); }
        }
      `}</style>
    </div>
  );
}

function Stat({ label, value, amber = false }: { label: string; value: string; amber?: boolean }) {
  return (
    <div className={"rounded-md bg-white border p-2 " + (amber ? "border-[#f59e0b]" : "border-[#e0e0e0]")}>
      <div className="text-[10px] uppercase text-[#5f6368] tracking-wider">{label}</div>
      <div className={"text-base font-semibold " + (amber ? "text-[#f59e0b]" : "text-[#202124]")}>{value}</div>
    </div>
  );
}

function ChartSvg() {
  // Width 600 for easy stroke-dasharray sums.
  return (
    <svg viewBox="0 0 600 180" width="100%" height="180" className="overflow-visible">
      {/* Baseline band */}
      <rect x="0" y="70" width="600" height="40" fill="#e8f0fe" className="demo-band" />
      {/* Grid */}
      <line x1="0" y1="90" x2="600" y2="90" stroke="#e0e0e0" strokeDasharray="3 3" />
      {/* Blue baseline polyline */}
      <polyline
        points="0,100 50,95 100,90 150,92 200,88 250,90 300,92 350,88 400,90 420,90"
        fill="none"
        stroke="#1a73e8"
        strokeWidth="2"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="demo-line-blue"
      />
      {/* Drift marker */}
      <line x1="420" y1="10" x2="420" y2="170" stroke="#f59e0b" strokeDasharray="4 4" className="demo-marker" />
      {/* Amber polyline (drift) */}
      <polyline
        points="420,90 450,70 480,55 510,45 540,38 570,35 600,32"
        fill="none"
        stroke="#f59e0b"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="demo-line-amber"
      />
      {/* Drift label */}
      <text x="430" y="25" fontSize="11" fill="#92400e" className="demo-label">
        drift detected
      </text>
    </svg>
  );
}
