export function restorableRoute(value: string, origin: string): string | undefined {
  try {
    const url = new URL(value, origin);
    if (url.origin !== origin || url.username || url.password) return;
    if (!/^\/console\/(?:sessions(?:\/[^/]+)?)?$/.test(url.pathname)) return;
    const workspace = url.searchParams.get("workspace");
    return url.pathname + (workspace ? `?${new URLSearchParams({ workspace })}` : "");
  } catch {
    return;
  }
}
