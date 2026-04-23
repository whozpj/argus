import Link from "next/link";
import { Zap, CheckCircle, Shield, Activity, Bell, Server, Sparkles, ArrowRight } from "lucide-react";
import DashboardDemo from "@/components/DashboardDemo";

export default function LandingPage() {
  return (
    <div className="min-h-screen bg-white text-[#202124]">
      <TopNav />
      <Hero />
      <Stats />
      <DemoSection />
      <Features />
      <CTA />
      <Footer />
    </div>
  );
}

// ─── Top nav ──────────────────────────────────────────────────────────────────

function TopNav() {
  return (
    <nav className="sticky top-0 z-20 bg-white border-b border-[#e0e0e0] h-14 flex items-center px-6">
      <Link href="/" className="flex items-center gap-2">
        <div className="flex items-center justify-center w-7 h-7 rounded-md bg-[#1a73e8] text-white">
          <Zap className="h-4 w-4" />
        </div>
        <span className="font-semibold text-[15px]">Argus</span>
      </Link>
      <div className="ml-8 flex items-center gap-6 text-sm text-[#5f6368]">
        <Link href="/docs/quickstart" className="hover:text-[#202124]">Docs</Link>
        <a href="#pricing" className="hover:text-[#202124]">Pricing</a>
        <a
          href="https://github.com/whozpj/argus"
          target="_blank"
          rel="noreferrer noopener"
          className="hover:text-[#202124]"
        >
          GitHub
        </a>
      </div>
      <div className="ml-auto flex items-center gap-3">
        <Link href="/login" className="text-sm text-[#5f6368] hover:text-[#202124]">Sign in</Link>
        <Link
          href="/login"
          className="inline-flex items-center h-8 px-4 rounded-md bg-[#1a73e8] text-white text-sm font-medium hover:bg-[#1765cc]"
        >
          Get started free
        </Link>
      </div>
    </nav>
  );
}

// ─── Hero ─────────────────────────────────────────────────────────────────────

function Hero() {
  return (
    <section className="max-w-6xl mx-auto px-6 py-20 grid lg:grid-cols-2 gap-12 items-center">
      <div className="space-y-6">
        <span className="inline-flex items-center gap-1.5 text-xs font-medium text-[#1a73e8] bg-[#e8f0fe] rounded-full px-3 py-1">
          <Sparkles className="h-3 w-3" />
          Now in public beta
        </span>
        <h1 className="text-5xl font-semibold tracking-tight leading-[1.1]">
          Know when your LLM <span className="text-[#1a73e8]">behavior changes</span> before users do
        </h1>
        <p className="text-lg text-[#5f6368] max-w-xl">
          Argus is a drift detector for production LLM apps. One line to instrument, zero prompt data stored, alerts in Slack the moment a model starts behaving differently.
        </p>
        <div className="flex items-center gap-3 pt-2">
          <Link
            href="/login"
            className="inline-flex items-center gap-2 h-10 px-5 rounded-md bg-[#1a73e8] text-white text-sm font-medium hover:bg-[#1765cc]"
          >
            Start for free <ArrowRight className="h-4 w-4" />
          </Link>
          <Link
            href="/docs/quickstart"
            className="inline-flex items-center h-10 px-5 rounded-md border border-[#dadce0] text-sm font-medium hover:bg-[#f1f3f4]"
          >
            Read the docs
          </Link>
        </div>
      </div>

      {/* Terminal mock */}
      <div className="bg-[#202124] rounded-lg shadow-xl overflow-hidden border border-[#3c4043]">
        <div className="h-8 bg-[#3c4043] flex items-center gap-1.5 px-3">
          <span className="w-2.5 h-2.5 rounded-full bg-[#ff5f57]" />
          <span className="w-2.5 h-2.5 rounded-full bg-[#febc2e]" />
          <span className="w-2.5 h-2.5 rounded-full bg-[#28c840]" />
        </div>
        <pre className="p-5 text-[13px] leading-6 font-mono text-[#e8eaed]">
          <span className="text-[#5f6368]">$</span> pip install argus-sdk{"\n"}
          <span className="text-[#34a853]">✓ installed argus-sdk 0.4.0</span>{"\n\n"}
          <span className="text-[#5f6368]">&gt;&gt;&gt;</span> <span className="text-[#8ab4f8]">from</span> argus_sdk <span className="text-[#8ab4f8]">import</span> patch{"\n"}
          <span className="text-[#5f6368]">&gt;&gt;&gt;</span> patch(api_key=<span className="text-[#fdd663]">&quot;argus_sk_...&quot;</span>){"\n"}
          <span className="text-[#5f6368]"># ... 4 hours later ...</span>{"\n"}
          <span className="text-[#f59e0b]">⚠ DRIFT — output_tokens +42%</span>
        </pre>
      </div>
    </section>
  );
}

// ─── Stats bar ────────────────────────────────────────────────────────────────

function Stats() {
  const stats: [string, string][] = [
    ["1 line", "to instrument"],
    ["60s", "drift check interval"],
    ["98%", "detection rate at +20% shift"],
    ["0%", "prompt data stored"],
  ];
  return (
    <section className="border-y border-[#e0e0e0] bg-white">
      <div className="max-w-6xl mx-auto px-6 py-10 grid grid-cols-2 md:grid-cols-4 gap-6">
        {stats.map(([v, l]) => (
          <div key={l} className="text-center">
            <div className="text-3xl font-semibold text-[#202124]">{v}</div>
            <div className="text-xs text-[#5f6368] mt-1 uppercase tracking-wider">{l}</div>
          </div>
        ))}
      </div>
    </section>
  );
}

// ─── Demo section ─────────────────────────────────────────────────────────────

function DemoSection() {
  return (
    <section className="bg-[#f8f9fa] py-20">
      <div className="max-w-6xl mx-auto px-6">
        <div className="text-center mb-10">
          <span className="text-xs font-medium text-[#5f6368] uppercase tracking-wider">
            See it in action — live dashboard
          </span>
          <h2 className="text-3xl font-semibold mt-2">Watch drift get caught in real time</h2>
        </div>
        <DashboardDemo />
      </div>
    </section>
  );
}

// ─── Features ─────────────────────────────────────────────────────────────────

function Features() {
  const features: { icon: React.ReactNode; title: string; body: string }[] = [
    {
      icon: <Activity className="h-5 w-5" />,
      title: "Statistical drift detection",
      body: "Mann-Whitney U + Bonferroni correction. No thresholds to tune.",
    },
    {
      icon: <Shield className="h-5 w-5" />,
      title: "Zero prompt exposure",
      body: "Only derived signals (token counts, latency, finish reason). Never prompts or completions.",
    },
    {
      icon: <CheckCircle className="h-5 w-5" />,
      title: "Non-blocking instrumentation",
      body: "Background thread. Adds nothing to your request latency.",
    },
    {
      icon: <Bell className="h-5 w-5" />,
      title: "Slack alerts",
      body: "Hysteresis keeps alerts quiet. Fires once, clears cleanly.",
    },
    {
      icon: <Sparkles className="h-5 w-5" />,
      title: "Multi-model support",
      body: "Anthropic, OpenAI, any compatible provider. One SDK.",
    },
    {
      icon: <Server className="h-5 w-5" />,
      title: "Self-host or cloud",
      body: "Run a Docker container or use Argus Cloud. Same code, same SDK.",
    },
  ];
  return (
    <section className="max-w-6xl mx-auto px-6 py-20">
      <h2 className="text-3xl font-semibold text-center">Everything you need, nothing you don&apos;t</h2>
      <div className="grid sm:grid-cols-2 lg:grid-cols-3 gap-5 mt-12">
        {features.map((f) => (
          <div key={f.title} className="rounded-lg border border-[#e0e0e0] bg-white p-6">
            <div className="w-10 h-10 rounded-md bg-[#e8f0fe] text-[#1a73e8] flex items-center justify-center mb-4">
              {f.icon}
            </div>
            <h3 className="font-semibold">{f.title}</h3>
            <p className="text-sm text-[#5f6368] mt-1">{f.body}</p>
          </div>
        ))}
      </div>
    </section>
  );
}

// ─── CTA ──────────────────────────────────────────────────────────────────────

function CTA() {
  return (
    <section id="pricing" className="bg-[#e8f0fe] py-20">
      <div className="max-w-3xl mx-auto px-6 text-center">
        <h2 className="text-3xl font-semibold">Start monitoring your LLMs today</h2>
        <p className="text-[#5f6368] mt-3">Free to start. No credit card required.</p>
        <Link
          href="/login"
          className="inline-flex items-center gap-2 mt-6 h-11 px-6 rounded-md bg-[#1a73e8] text-white text-sm font-medium hover:bg-[#1765cc]"
        >
          Create free account <ArrowRight className="h-4 w-4" />
        </Link>
      </div>
    </section>
  );
}

// ─── Footer ───────────────────────────────────────────────────────────────────

function Footer() {
  return (
    <footer className="border-t border-[#e0e0e0] py-8">
      <div className="max-w-6xl mx-auto px-6 flex items-center justify-between text-xs text-[#5f6368]">
        <div className="flex items-center gap-2">
          <div className="flex items-center justify-center w-5 h-5 rounded-sm bg-[#1a73e8] text-white">
            <Zap className="h-3 w-3" />
          </div>
          <span>Argus</span>
        </div>
        <div className="flex items-center gap-5">
          <Link href="/docs/quickstart" className="hover:text-[#202124]">Docs</Link>
          <a href="https://github.com/whozpj/argus" target="_blank" rel="noreferrer noopener" className="hover:text-[#202124]">GitHub</a>
          <span>Privacy</span>
        </div>
      </div>
    </footer>
  );
}
