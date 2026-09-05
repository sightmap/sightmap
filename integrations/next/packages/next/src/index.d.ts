import type { ReactElement } from "react";

export interface SightkickToolsProps {
  /** The compiled IR: a URL to fetch (default "/.well-known/sightkick.json") or the parsed JSON to inline. */
  ir?: string | object;
  /** URL of the Sightkick runtime bundle (default "/sightkick-runtime.js"). */
  runtime?: string;
  /** Render nothing when false (e.g. gate on an environment variable). */
  enabled?: boolean;
  /** next/script strategy; "afterInteractive" by default. */
  strategy?: "afterInteractive" | "lazyOnload" | "beforeInteractive";
}

export function SightkickTools(
  props?: SightkickToolsProps,
): ReactElement | null;
export default SightkickTools;
