export const LATEST_RELEASE_URL = "https://github.com/chaserensberger/wingman/releases/latest";

export async function fetchLatestRelease(signal: AbortSignal) {
  try {
    const response = await fetch(
      "https://api.github.com/repos/chaserensberger/wingman/releases/latest",
      { signal },
    );
    if (!response.ok) return null;

    const release = await response.json();
    if (typeof release?.tag_name !== "string" || !release.tag_name.trim()) return null;

    return {
      version: release.tag_name,
      url: `https://github.com/chaserensberger/wingman/releases/tag/${encodeURIComponent(release.tag_name)}`,
    };
  } catch {
    return null;
  }
}
