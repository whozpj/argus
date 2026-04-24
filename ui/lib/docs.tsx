import { ReactNode } from "react";

export interface DocsPage {
  slug: string;
  title: string;
  Content: () => ReactNode;
}

// Shared prose wrapper classes — GCP-style docs body
const prose =
  "max-w-3xl text-sm leading-6 text-[#202124] space-y-4 " +
  "[&_h2]:text-xl [&_h2]:font-semibold [&_h2]:mt-6 [&_h2]:mb-2 " +
  "[&_h3]:text-base [&_h3]:font-semibold [&_h3]:mt-5 [&_h3]:mb-1 " +
  "[&_code]:font-mono [&_code]:text-[13px] [&_code]:bg-[#f1f3f4] [&_code]:px-1 [&_code]:py-0.5 [&_code]:rounded " +
  "[&_pre]:bg-[#202124] [&_pre]:text-white [&_pre]:p-4 [&_pre]:rounded-md [&_pre]:text-xs [&_pre]:overflow-x-auto " +
  "[&_pre_code]:bg-transparent [&_pre_code]:text-white [&_pre_code]:p-0 " +
  "[&_ul]:list-disc [&_ul]:pl-5 [&_ul]:space-y-1 " +
  "[&_a]:text-[#1a73e8] [&_a]:underline";

function Quickstart() {
  return (
    <article className={prose}>
      <h2>Quick Start</h2>
      <h3>1. Install</h3>
      <pre><code>pip install argus-sdk</code></pre>
      <h3>2. Add one line to your app</h3>
      <pre><code>{`from argus_sdk import patch
patch(endpoint="https://argus-sdk.com", api_key="argus_sk_...")`}</code></pre>
      <h3>3. Your LLM code is unchanged</h3>
      <pre><code>{`import anthropic
client = anthropic.Anthropic()
response = client.messages.create(model="claude-sonnet-4-6", messages=[...])
# Signals are sent to Argus in the background`}</code></pre>
      <h3>4. Watch the dashboard</h3>
      <p>Log in at <a href="https://argus-sdk.com">argus-sdk.com</a>. Once 100+ events are collected, drift detection activates.</p>
    </article>
  );
}

function SdkReference() {
  return (
    <article className={prose}>
      <h2>SDK Reference</h2>
      <h3>patch(endpoint, api_key, client=None)</h3>
      <p>Wraps Anthropic and OpenAI clients to emit signal events to Argus in a background thread.</p>
      <ul>
        <li><code>endpoint</code> — Argus server URL, e.g. <code>https://argus-sdk.com</code>.</li>
        <li><code>api_key</code> — project API key (<code>argus_sk_...</code>). Omit for self-hosted mode.</li>
        <li><code>client</code> — optional specific client instance to patch. Default: patches the module.</li>
      </ul>
      <h3>Anthropic example</h3>
      <pre><code>{`from argus_sdk import patch
import anthropic

patch(endpoint="https://argus-sdk.com", api_key="argus_sk_...")
client = anthropic.Anthropic()
client.messages.create(model="claude-sonnet-4-6", messages=[...])`}</code></pre>
      <h3>OpenAI example</h3>
      <pre><code>{`from argus_sdk import patch
import openai

patch(endpoint="https://argus-sdk.com", api_key="argus_sk_...")
client = openai.OpenAI()
client.chat.completions.create(model="gpt-4o", messages=[...])`}</code></pre>
      <h3>Streaming</h3>
      <p>Supported. The wrapper intercepts <code>stream_complete</code> and records the full completion&apos;s signals once the stream ends.</p>
      <h3>Signal shape</h3>
      <p>What the SDK sends (no prompt text, no completion text):</p>
      <pre><code>{`{
  "model":         "claude-sonnet-4-6",
  "provider":      "anthropic",
  "input_tokens":  312,
  "output_tokens": 87,
  "latency_ms":    843,
  "finish_reason": "stop",
  "timestamp_utc": "2026-04-07T14:22:01Z"
}`}</code></pre>
    </article>
  );
}

function CliReference() {
  return (
    <article className={prose}>
      <h2>CLI Reference</h2>
      <h3>argus login</h3>
      <p>Opens your browser, receives a one-time code, exchanges it for a JWT, and saves it to <code>~/.config/argus/credentials.json</code>.</p>
      <pre><code>argus login</code></pre>
      <h3>argus status</h3>
      <p>Prints your email and a drift summary for each project.</p>
      <pre><code>argus status</code></pre>
      <h3>argus projects</h3>
      <p>Lists your projects with a masked API key prefix.</p>
      <pre><code>argus projects</code></pre>
    </article>
  );
}

function SelfHosting() {
  return (
    <article className={prose}>
      <h2>Self-hosting</h2>
      <p>Argus ships as a single Docker image that bundles the Go API server and the Next.js dashboard.</p>
      <h3>Run</h3>
      <pre><code>{`docker run \\
  -e POSTGRES_URL=postgres://argus:argus@host:5432/argus \\
  -p 4000:4000 -p 3000:3000 argus`}</code></pre>
      <h3>Environment variables</h3>
      <ul>
        <li><code>ARGUS_ADDR</code> — listen address (default <code>:4000</code>)</li>
        <li><code>POSTGRES_URL</code> — PostgreSQL connection string</li>
        <li><code>ARGUS_SLACK_WEBHOOK</code> — Slack incoming webhook URL</li>
        <li><code>JWT_SECRET</code> — HS256 signing key</li>
        <li><code>ARGUS_BASE_URL</code> — public server URL (OAuth redirect URIs)</li>
        <li><code>ARGUS_UI_URL</code> — dashboard URL (post-OAuth browser redirect)</li>
        <li><code>GITHUB_CLIENT_ID</code>, <code>GITHUB_CLIENT_SECRET</code> — GitHub OAuth app</li>
        <li><code>GOOGLE_CLIENT_ID</code>, <code>GOOGLE_CLIENT_SECRET</code> — Google OAuth app</li>
      </ul>
      <h3>Fallback project</h3>
      <p>Unauthenticated SDK requests (no <code>api_key</code>) fall back to the <code>&quot;self-hosted&quot;</code> project. Useful for single-tenant deployments.</p>
    </article>
  );
}

function HowItWorks() {
  return (
    <article className={prose}>
      <h2>How drift detection works</h2>
      <h3>Online baseline — Welford&apos;s algorithm</h3>
      <p>Argus keeps a running mean and variance per model per project using Welford&apos;s online algorithm. No raw events are stored for baselining — only the aggregates needed to compare new samples against the baseline distribution.</p>
      <h3>Mann-Whitney U test</h3>
      <p>Every 60 seconds, the detector runs a Mann-Whitney U test on <code>output_tokens</code> and <code>latency_ms</code> over a recent window vs. the baseline. This is a distribution-free test, so we don&apos;t need to assume normality.</p>
      <h3>Bonferroni correction</h3>
      <p>Because two signals are tested per model, p-values are Bonferroni-corrected (multiplied by the number of tests) to control the false-positive rate.</p>
      <h3>Hysteresis</h3>
      <p>Alerts fire when <code>drift_score &gt; 0.7</code> for the first time. They clear only after 3 consecutive windows below <code>0.4</code>. Once clear, the alert can fire again on the next trigger.</p>
      <h3>Minimum sample size</h3>
      <p>A model needs at least 100 events before its baseline is considered ready. Before that, drift detection is skipped and the dashboard shows &quot;warming up&quot;.</p>
    </article>
  );
}

function Alerts() {
  return (
    <article className={prose}>
      <h2>Alerts &amp; Slack</h2>
      <h3>Configure Slack</h3>
      <p>Set <code>ARGUS_SLACK_WEBHOOK</code> to an incoming-webhook URL. Argus posts Block Kit messages to that channel when drift is detected.</p>
      <pre><code>ARGUS_SLACK_WEBHOOK=https://hooks.slack.com/services/...</code></pre>
      <h3>Trigger rule</h3>
      <p>An alert fires once when <code>drift_score &gt; 0.7</code>. Hysteresis prevents re-firing: Argus will not alert again for that model until it first clears.</p>
      <h3>Clear rule</h3>
      <p>An alert clears after 3 consecutive 60-second windows with <code>drift_score &lt; 0.4</code>. A new trigger can then fire the next time the threshold is crossed.</p>
    </article>
  );
}

export const DOCS_PAGES: DocsPage[] = [
  { slug: "quickstart",   title: "Quick Start",                 Content: Quickstart },
  { slug: "sdk",          title: "SDK Reference",               Content: SdkReference },
  { slug: "cli",          title: "CLI Reference",               Content: CliReference },
  { slug: "self-hosting", title: "Self-hosting",                Content: SelfHosting },
  { slug: "how-it-works", title: "How drift detection works",   Content: HowItWorks },
  { slug: "alerts",       title: "Alerts & Slack",              Content: Alerts },
];

export function getDocsPage(slug: string): DocsPage | undefined {
  return DOCS_PAGES.find((p) => p.slug === slug);
}
