import { Outlet } from "@tanstack/react-router";
import { TanStackRouterDevtools } from "@tanstack/react-router-devtools";
import { ThemeToggle } from "@/components/theme-toggle";
import { Link } from "@tanstack/react-router";

export default function App() {
  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between border-b px-4 py-3">
        <Link to="/sessions" className="text-sm font-semibold hover:underline">
          WingBase
        </Link>
        <ThemeToggle />
      </header>
      <main className="flex-1">
        <Outlet />
      </main>
      {import.meta.env.DEV && <TanStackRouterDevtools />}
    </div>
  );
}
