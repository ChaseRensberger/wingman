import { useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import {
  House,
  HardDrives,
  ChatTeardropText,
  Robot,
  Gear,
  type Icon,
} from '@phosphor-icons/react'
import clsx from 'clsx'
import { Link } from '../primitives/link'
import wingmanLogo from '../../assets/wingman-blue.png'
import { SettingsDialog } from '../settings/settings-dialog'

const navigation: { name: string; href: string; icon: Icon }[] = [
  { name: 'Dashboard', href: '/', icon: House },
  { name: 'Providers', href: '/providers', icon: HardDrives },
  { name: 'Agents', href: '/agents', icon: Robot },
  { name: 'Sessions', href: '/sessions', icon: ChatTeardropText },
]

export function AppSidebar() {
  const router = useRouterState()
  const pathname = router.location.pathname
  const [settingsOpen, setSettingsOpen] = useState(false)

  return (
    <div className="flex grow flex-col items-center overflow-y-auto border-r border-border bg-background pb-4 dark:border-border dark:bg-card">
      {/* Logo — h-16 + border-b matches the top header so both underlines align */}
      {/* TODO: swap PNG for an SVG to shrink bundle size and get crisp scaling */}
      <div className="flex h-16 w-full shrink-0 items-center justify-center border-b border-border dark:border-border">
        <img src={wingmanLogo} alt="Wingman" className="size-8" />
      </div>

      <nav className="mt-5 flex flex-1 flex-col">
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
                      ? 'bg-muted text-foreground dark:bg-background/10 dark:text-foreground'
                      : 'text-muted-foreground hover:bg-accent hover:text-foreground dark:text-muted-foreground dark:hover:bg-background/5 dark:hover:text-foreground',
                    'group flex size-10 items-center justify-center rounded-md'
                  )}
                >
                  <span className="sr-only">{item.name}</span>
                  <item.icon
                    aria-hidden="true"
                    className="size-5 shrink-0"
                    weight={current ? 'fill' : 'regular'}
                  />
                </Link>
              </li>
            )
          })}

          <li className="mt-auto">
            <button
              type="button"
              onClick={() => setSettingsOpen(true)}
              title="Settings"
              className="group flex size-10 cursor-pointer items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground dark:text-muted-foreground dark:hover:bg-background/5 dark:hover:text-foreground"
            >
              <span className="sr-only">Settings</span>
              <Gear aria-hidden="true" className="size-5 shrink-0" />
            </button>
          </li>
        </ul>
      </nav>

      <SettingsDialog open={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </div>
  )
}
