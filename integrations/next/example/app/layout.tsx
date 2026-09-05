import type { Metadata } from "next";
import Link from "next/link";
import { SightkickTools } from "@sightmap/next";
import ir from "../public/.well-known/sightkick.json";
import "./globals.css";

export const metadata: Metadata = {
  title: "Tasks — Sightmap + WebMCP on Next.js",
  description:
    "A Next.js app whose .sightmap/ corpus compiles into WebMCP tools.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body>
        <nav data-component="SiteNav" aria-label="Site">
          <Link data-component="NavLink" href="/">
            Board
          </Link>
          <Link data-component="NavLink" href="/about">
            About
          </Link>
        </nav>
        {children}
        {/* Registers the compiled tool layer on document.modelContext (WebMCP)
            on every page. The IR is inlined at build time from
            public/.well-known/sightkick.json, which is also served as-is for
            external readers; pass a URL string instead to fetch it lazily. */}
        <SightkickTools ir={ir} />
      </body>
    </html>
  );
}
