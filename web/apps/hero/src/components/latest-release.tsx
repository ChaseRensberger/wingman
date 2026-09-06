import { useEffect, useState } from "react";
import { fetchLatestRelease, LATEST_RELEASE_URL } from "../lib/release";

export function LatestRelease() {
  const [release, setRelease] = useState<Awaited<ReturnType<typeof fetchLatestRelease>>>(null);

  useEffect(() => {
    const controller = new AbortController();
    void fetchLatestRelease(controller.signal).then((result) => {
      if (!controller.signal.aborted) setRelease(result);
    });
    return () => controller.abort();
  }, []);

  return (
    <a
      href={release?.url ?? LATEST_RELEASE_URL}
      className="rounded-sm text-xs text-primary underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring"
    >
      Latest release{release ? `: ${release.version}` : ""} ↗
    </a>
  );
}
