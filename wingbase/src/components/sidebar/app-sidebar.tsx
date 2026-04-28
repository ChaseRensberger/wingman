import { useRouterState } from '@tanstack/react-router'
import {
  House,
  HardDrives,
  ChatTeardropText,
  Gear,
  type Icon,
} from '@phosphor-icons/react'
import clsx from 'clsx'
import { Link } from '../primitives/link'
import wingmanLogo from '../../assets/wingman-blue.png'

const navigation: { name: string; href: string; icon: Icon }[] = [
  { name: 'Dashboard', href: '/', icon: House },
  { name: 'Providers', href: '/providers', icon: HardDrives },
  { name: 'Sessions', href: '/sessions', icon: ChatTeardropText },
]

export function AppSidebar() {
  const router = useRouterState()
  const pathname = router.location.pathname

  return (
    <div className="flex grow flex-col items-center gap-y-5 overflow-y-auto border-r border-zinc-200 bg-white pb-4 dark:border-white/10 dark:bg-zinc-900">
      {/* Logo — same h-16 as top header so it aligns with the page title */}
      <div className="flex h-16 shrink-0 items-center justify-center">
        <img src={wingmanLogo} alt="Wingman" className="size-8" />
      </div>

      <nav className="flex flex-1 flex-col">
        <ul role="list" className="flex flex-1 flex-col items-center gap-y-1">
          {navigation.map((item) => {
            const current = pathname === item.href
            return (
              <li key={item.name}>
                <Link
                  href={item.href}
                  title={item.name}
                  className={clsx(
                    current
                      ? 'bg-zinc-100 text-zinc-950 dark:bg-white/10 dark:text-white'
                      : 'text-zinc-500 hover:bg-zinc-50 hover:text-zinc-950 dark:text-zinc-400 dark:hover:bg-white/5 dark:hover:text-white',
                    'group flex size-10 items-center justify-center rounded-md'
                  )}
                >
                  <span className="sr-only">{item.name}</span>
                  <item.icon aria-hidden="true" className="size-5 shrink-0" weight={current ? 'fill' : 'regular'} />
                </Link>
              </li>
            )
          })}

          <li className="mt-auto">
            <Link
              href="/settings"
              title="Settings"
              className="group flex size-10 items-center justify-center rounded-md text-zinc-500 hover:bg-zinc-50 hover:text-zinc-950 dark:text-zinc-400 dark:hover:bg-white/5 dark:hover:text-white"
            >
              <span className="sr-only">Settings</span>
              <Gear aria-hidden="true" className="size-5 shrink-0" />
            </Link>
          </li>
        </ul>
      </nav>
    </div>
  )
}
