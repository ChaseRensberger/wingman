import { InputGroup, InputGroupAddon } from "@/components/core/input-group"
import { Input } from "@/components/core/input"
import { MagnifyingGlassIcon, GlobeIcon } from "@phosphor-icons/react"

export function InputGroupShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Input Group</h2>
      <div className="space-y-4 max-w-sm">
        <InputGroup>
          <InputGroupAddon>$</InputGroupAddon>
          <Input className="rounded-l-none" placeholder="0.00" />
        </InputGroup>
        <InputGroup>
          <InputGroupAddon>
            <MagnifyingGlassIcon className="size-4" />
          </InputGroupAddon>
          <Input className="rounded-l-none" placeholder="Search..." />
        </InputGroup>
        <InputGroup>
          <InputGroupAddon>
            <GlobeIcon className="size-4" />
          </InputGroupAddon>
          <Input className="rounded-none" placeholder="yoursite" />
          <InputGroupAddon>.com</InputGroupAddon>
        </InputGroup>
      </div>
    </section>
  )
}
