import type { Metadata } from "next";
import localFont from "next/font/local";
import "./globals.css";

const geistSans = localFont({ src: "./fonts/GeistVF.woff", variable: "--font-geist-sans" });
const geistMono = localFont({ src: "./fonts/GeistMonoVF.woff", variable: "--font-geist-mono" });

const description =
  "Know when your LLM's behavior changes before your users do. Statistical drift detection for production LLM apps — one line to instrument, alerts in Slack, zero prompt data stored.";

export const metadata: Metadata = {
  metadataBase: new URL("https://argus-sdk.com"),
  title: "Argus — LLM Drift Monitor",
  description,
  openGraph: {
    title: "Argus — LLM Drift Monitor",
    description,
    url: "https://argus-sdk.com",
    siteName: "Argus",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: "Argus — LLM Drift Monitor",
    description,
  },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={`${geistSans.variable} ${geistMono.variable} antialiased bg-background text-foreground`}>
        {children}
      </body>
    </html>
  );
}
