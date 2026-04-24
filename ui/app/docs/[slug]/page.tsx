import { Suspense } from "react";
import Link from "next/link";
import Shell from "@/components/Shell";
import { DOCS_PAGES, getDocsPage } from "@/lib/docs";

export function generateStaticParams() {
  return DOCS_PAGES.map((p) => ({ slug: p.slug }));
}

export default function DocsPage({ params }: { params: { slug: string } }) {
  const { slug } = params;
  const page = getDocsPage(slug);

  return (
    <Suspense>
      <Shell>
        <div className="max-w-3xl" data-testid="docs-page">
          {page ? (
            <>
              <h1 className="text-[22px] font-medium text-[#202124] mb-4" data-testid="docs-title">
                {page.title}
              </h1>
              <page.Content />
            </>
          ) : (
            <div className="space-y-3">
              <h1 className="text-[22px] font-medium text-[#202124]">Not found</h1>
              <p className="text-sm text-[#5f6368]">
                No docs page for slug{" "}
                <code className="bg-[#f1f3f4] px-1 py-0.5 rounded text-xs">{slug}</code>.
              </p>
              <Link href="/docs/quickstart" className="text-sm text-[#1a73e8] underline">
                Back to Quick Start
              </Link>
            </div>
          )}
        </div>
      </Shell>
    </Suspense>
  );
}
