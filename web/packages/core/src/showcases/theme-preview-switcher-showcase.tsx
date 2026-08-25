import { ThemePreviewSwitcher } from "#components/theme-preview-switcher";

export function ThemePreviewSwitcherShowcase() {
  return (
    <section className="space-y-4 py-4">
      <div>
        <h2 className="text-2xl font-semibold">Theme Preview Switcher</h2>
      </div>
      <ThemePreviewSwitcher />
    </section>
  );
}
