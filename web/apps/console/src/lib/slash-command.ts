export function slashCommandQuery(text: string): string | undefined {
  return text.match(/^\/(\S*)$/)?.[1];
}
