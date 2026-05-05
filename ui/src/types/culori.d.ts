declare module "culori" {
  export function formatHex(color: unknown): string | undefined
  export function oklch(color: unknown): { l: number; c: number; h: number; alpha?: number } | undefined
  export function parse(color: string): unknown
}
